package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	_ "net/http/pprof"
	"os"
	"reflect"

	Settings "github.com/MidSmer/goTorrent/settings"
	Storage "github.com/MidSmer/goTorrent/storage"
	"github.com/MidSmer/goTorrent/torrent"
	"github.com/asdine/storm"
	"github.com/dgrijalva/jwt-go"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

var (
	//Logger does logging for the entire project
	Logger = logrus.New()
	//Authenticated stores the value of the result of the client that connects to the server
	Authenticated = false
	APP_ID        = os.Getenv("APP_ID")
	sendJSON      = make(chan interface{}) //channel for JSON messages
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	s1, _ := template.ParseFiles("templates/home.tmpl")
	s1.ExecuteTemplate(w, "base", map[string]string{"APP_ID": APP_ID})
}

//HandleMessages creates a queue of JSON messages from the client and executes them in order
func handleMessages(conn *websocket.Conn) {
	for {
		msgJSON := <-sendJSON
		conn.WriteJSON(msgJSON)
	}
}

func handleAuthentication(conn *websocket.Conn, db *storm.DB) {
	msg := torrent.Message{}
	err := conn.ReadJSON(&msg)
	payloadData, ok := msg.Payload.(map[string]interface{})
	clientAuthToken, tokenOk := payloadData["ClientAuthString"].(string)
	fmt.Println("ClientAuthToken:", clientAuthToken, "TokenOkay", tokenOk, "PayloadData", payloadData, "PayloadData Okay?", ok)
	if ok == false || tokenOk == false {
		authFail := torrent.AuthResponse{MessageType: "authResponse", Payload: "Message Payload in AuthRequest was malformed, closing connection"}
		conn.WriteJSON(authFail)
		conn.Close()
		return
	}
	if err != nil {
		Logger.WithFields(logrus.Fields{"error": err, "SuppliedToken": clientAuthToken}).Error("Unable to read authentication message")
	}
	fmt.Println("Authstring", clientAuthToken)
	//clientAuthToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJjbGllbnROYW1lIjoiZ29Ub3JyZW50V2ViVUkiLCJpc3MiOiJnb1RvcnJlbnRTZXJ2ZXIifQ.Lfqp9tm06CY4XfrqnNDeVLkq9c7rsbibDrUdPko8ffQ"
	signingKeyStruct := Storage.FetchJWTTokens(db)
	singingKey := signingKeyStruct.SigningKey
	token, err := jwt.Parse(clientAuthToken, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}
		return singingKey, nil
	})
	if err != nil {
		authFail := torrent.AuthResponse{MessageType: "authResponse", Payload: "Parsing of Token failed, ensure you have the correct token! Closing Connection"}
		conn.WriteJSON(authFail)
		Logger.WithFields(logrus.Fields{"error": err, "SuppliedToken": token}).Error("Unable to parse token!")
		fmt.Println("ENTIRE SUPPLIED TOKEN:", token, "CLIENTAUTHTOKEN", clientAuthToken)
		conn.Close()
		return
	}
	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		authTrue := torrent.AuthResponse{MessageType: "authResponse", Payload: "Authentication Verified, proceed with commands."}
		conn.WriteJSON(authTrue)
		fmt.Println("Claims", claims["ClientName"], claims["Issuer"])
		Authenticated = true
	} else {
		Logger.WithFields(logrus.Fields{"error": err}).Error("Authentication Error occurred, cannot complete!")
	}
}

func main() {
	torrent.Logger = Logger //Injecting the logger into all the packages
	Storage.Logger = Logger
	Settings.Logger = Logger

	Config := Settings.FullClientSettingsNew() //grabbing from settings.go
	if Config.LoggingOutput == "file" {
		_, err := os.Stat("logs")
		if os.IsNotExist(err) {
			err := os.Mkdir("logs", 0755)
			if err != nil {
				fmt.Println("Unable to create 'log' folder for logging.... please check permissions.. forcing output to stdout", err)
				Logger.Out = os.Stdout
			}
		} else {
			os.Remove("logs/server.log")                                               //cleanup the old log on every restart
			file, err := os.OpenFile("logs/server.log", os.O_CREATE|os.O_WRONLY, 0755) //creating the log file
			//defer file.Close()                                                         //TODO.. since we write to this constantly how does close work?
			if err != nil {
				fmt.Println("Unable to create file for logging.... please check permissions.. forcing output to stdout")
				Logger.Out = os.Stdout
			}
			fmt.Println("Logging to file logs/server.log")
			Logger.Out = file
		}
	} else {
		Logger.Out = os.Stdout
	}
	Logger.SetLevel(Config.LoggingLevel)

	httpAddr := Config.HTTPAddr
	_ = os.MkdirAll(Config.DownloadDir, 0755)  //creating a directory to store torrent files
	Logger.WithFields(logrus.Fields{"Config": Config}).Info("Torrent Client Config has been generated...")

	db, err := Storage.NewStorage(Config.DownloadDir) //initializing the boltDB store that contains all the added torrents
	if err != nil {
		Logger.WithFields(logrus.Fields{"error": err}).Fatal("Error opening/creating storage.db")
	} else {
		Logger.WithFields(logrus.Fields{"error": err}).Info("Opening or creating storage.db...")
	}
	defer db.Close() //defering closing the database until the program closes

	Config.TorrentConfig.DefaultStorage = db
	tclient, err := torrent.NewClient(Config.TorrentConfig) //pulling out the torrent specific config to use
	if err != nil {
		Logger.WithFields(logrus.Fields{"error": err}).Fatalf("Error creating torrent client: %s")
	}

	tokens := Storage.IssuedTokensList{} //if first run setting up the authentication tokens
	var signingKey []byte
	err = db.One("ID", 3, &tokens)
	if err != nil {
		Logger.WithFields(logrus.Fields{"RSSFeedStore": tokens, "error": err}).Info("No Tokens database found, assuming first run, generating token...")
		tokens.ID = 3 //creating the initial store
		claims := Settings.GoTorrentClaims{
			"goTorrentWebUI",
			jwt.StandardClaims{
				Issuer: "goTorrentServer",
			},
		}
		signingKey = Settings.GenerateSigningKey() //Running this will invalidate any certs you already issued!!
		authString := Settings.GenerateToken(claims, signingKey)
		tokens.SigningKey = signingKey
		tokens.FirstToken = authString
		tokens.TokenNames = append(tokens.TokenNames, Storage.SingleToken{"firstClient"})
		err := ioutil.WriteFile("clientAuth.txt", []byte(authString), 0755)
		if err != nil {
			Logger.WithFields(logrus.Fields{"error": err}).Warn("Unable to write client auth to file..")
		}
		db.Save(&tokens) //Writing all of that to the database
	} else { //Already have a signing key so pulling that signing key out of the database to sign any key requests
		tokens := Storage.FetchJWTTokens(db)
		signingKey = tokens.SigningKey
	}

	oldConfig, err := Storage.FetchConfig(db)
	if err != nil {
		Logger.WithFields(logrus.Fields{"error": err}).Info("Assuming first run as no config found in database, client config being generated")
		Settings.GenerateClientConfigFile(Config, tokens.FirstToken) //if first run generate the client config file
	} else {
		if reflect.DeepEqual(oldConfig.ClientConnectSettings, Config.ClientConnectSettings) {
			Logger.WithFields(logrus.Fields{"error": err}).Info("Configs are the same, not regenerating client config")
		} else {
			Logger.WithFields(logrus.Fields{"error": err}).Info("Config has changed, re-writting config")
			Settings.GenerateClientConfigFile(Config, tokens.FirstToken)
		}
	}
	Storage.SaveConfig(db, Config) //Save the config to the database

	Logger.Debug("Cron Engine Initialized...")

	var RunningTorrentArray = []torrent.ClientDB{} //this stores ALL of the torrents that are running, used for client update pushes combines Local Storage and Running tclient info
	var PreviousTorrentArray = []torrent.ClientDB{}

	TorrentLocalArray := Storage.FetchAllStoredTorrents(db) //pulling in all the already added torrents - this is an array of ALL of the local storage torrents, they will be added back in via hash

	router := mux.NewRouter()         //setting up the handler for the web backend
	router.HandleFunc("/", serveHome) //Serving the main page for our SPA
	router.PathPrefix("/static/").Handler(http.FileServer(http.Dir("public")))
	http.Handle("/", router)
	router.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) { //TODO, remove this
		RunningTorrentArray = torrent.CreateRunningTorrentArray(tclient) //Updates the RunningTorrentArray with the current client data as well
		var torrentlistArray = new(torrent.TorrentList)
		torrentlistArray.MessageType = "torrentList"          //setting the type of message
		torrentlistArray.ClientDBstruct = RunningTorrentArray //the full JSON that includes the number of torrents as the root
		torrentlistArray.Totaltorrents = len(RunningTorrentArray)
		torrentlistArrayJSON, _ := json.Marshal(torrentlistArray)
		w.Header().Set("Content-Type", "application/json")
		w.Write(torrentlistArrayJSON)
	})

	router.HandleFunc("/websocket", func(w http.ResponseWriter, r *http.Request) { //websocket is the main data pipe to the frontend
		conn, err := upgrader.Upgrade(w, r, nil)
		fmt.Println("Websocket connection established, awaiting authentication")
		connResponse := torrent.ServerPushMessage{MessageType: "connectResponse", MessageLevel: "Message", Payload: "Websocket Connection Established, awaiting Authentication"}
		conn.WriteJSON(&connResponse)
		defer conn.Close() //defer closing the websocket until done.
		if err != nil {
			Logger.WithFields(logrus.Fields{"error": err}).Fatal("Unable to create websocket!")
			return
		}
		if Authenticated != true {
			handleAuthentication(conn, db)
		} else { //If we are authenticated inject the connection into the other packages
			connResponse := torrent.ServerPushMessage{MessageType: "authResponse", MessageLevel: "Message", Payload: "Already Authenticated... Awaiting Commands"}
			conn.WriteJSON(&connResponse)
			Logger.Info("Authenticated, websocket connection available!")
		}
		torrent.Conn = conn
		Storage.Conn = conn

		go handleMessages(conn) //Starting the message channel to handle all the JSON requests from the client

	MessageLoop: //Tagging this so we can continue out of it with any errors we encounter that are failing
		for {
			msg := torrent.Message{}
			err := conn.ReadJSON(&msg)
			if err != nil {
				Logger.WithFields(logrus.Fields{"error": err, "message": msg}).Error("Unable to read JSON client message")
				torrent.CreateServerPushMessage(torrent.ServerPushMessage{MessageType: "serverPushMessage", MessageLevel: "info", Payload: "Malformed JSON request made to server.. ignoring"}, conn)
				break MessageLoop
			}
			var payloadData map[string]interface{}
			if msg.Payload != nil && msg.Payload != "" {
				payloadData = msg.Payload.(map[string]interface{})
			}
			Logger.WithFields(logrus.Fields{"message": msg}).Debug("Message From Client")
			switch msg.MessageType { //first handling data requests
			case "authRequest":
				if Authenticated {
					Logger.WithFields(logrus.Fields{"message": msg}).Debug("Client already authenticated... skipping authentication method")
				} else {
					handleAuthentication(conn, db)
				}

			case "newAuthToken":
				claims := Settings.GoTorrentClaims{
					payloadData["ClientName"].(string),
					jwt.StandardClaims{
						Issuer: "goTorrentServer",
					},
				}
				Logger.WithFields(logrus.Fields{"clientName": payloadData["ClientName"].(string)}).Info("New Auth Token creation request")
				fmt.Println("Signing Key", signingKey)
				token := Settings.GenerateToken(claims, signingKey)
				tokenReturn := Settings.TokenReturn{MessageType: "TokenReturn", TokenReturn: token}
				tokensDB := Storage.FetchJWTTokens(db)
				tokensDB.TokenNames = append(tokens.TokenNames, Storage.SingleToken{payloadData["ClientName"].(string)})
				db.Update(&tokensDB) //adding the new token client name to the database
				sendJSON <- tokenReturn

			case "torrentListRequest": //This will run automatically if a webUI is open
				Logger.WithFields(logrus.Fields{"message": msg}).Debug("Client Requested TorrentList Update")
				go func() { //running updates in separate thread so can still accept commands
					TorrentLocalArray = Storage.FetchAllStoredTorrents(db)           //Required to re-read the database since we write to the DB and this will pull the changes from it
					RunningTorrentArray = torrent.CreateRunningTorrentArray(tclient) //Updates the RunningTorrentArray with the current client data as well
					PreviousTorrentArray = RunningTorrentArray
					torrentlistArray := torrent.TorrentList{MessageType: "torrentList", ClientDBstruct: RunningTorrentArray, Totaltorrents: len(RunningTorrentArray)}
					Logger.WithFields(logrus.Fields{"torrentList": torrentlistArray, "previousTorrentList": PreviousTorrentArray}).Debug("Previous and Current Torrent Lists for sending to client")
					sendJSON <- torrentlistArray
				}()

			case "torrentFileListRequest": //client requested a filelist update
				//Logger.WithFields(logrus.Fields{"message": msg}).Info("Client Requested FileList Update")
				//fileListArrayRequest := payloadData["FileListHash"].(string)
				//FileListArray := torrent.CreateFileListArray(tclient, fileListArrayRequest, db, Config)
				//sendJSON <- FileListArray

			case "torrentPeerListRequest":
				Logger.WithFields(logrus.Fields{"message": msg}).Info("Client Requested PeerList Update")
				peerListArrayRequest := payloadData["PeerListHash"].(string)
				torrentPeerList := torrent.CreatePeerListArray(tclient, peerListArrayRequest)
				sendJSON <- torrentPeerList

			case "settingsFileRequest":
				//Logger.WithFields(logrus.Fields{"message": msg}).Info("Client Requested Settings File")
				//clientSettingsFile := torrent.SettingsFile{MessageType: "settingsFile", Config: Config}
				//sendJSON <- clientSettingsFile

			case "magnetLinkSubmit": //if we detect a magnet link we will be adding a magnet torrent
				labelValue, ok := payloadData["Label"].(string)
				if labelValue == "" || ok == false {
					labelValue = "None"
				}
				magnetLinks := payloadData["MagnetLinks"].([]interface{})
				for _, magnetLink := range magnetLinks {
					clientTorrent, err := tclient.AddMagnet(magnetLink.(string)) //reading the payload into the torrent client
					if err != nil {
						Logger.WithFields(logrus.Fields{"err": err, "MagnetLink": magnetLink}).Error("Unable to add magnetlink to client!")
						torrent.CreateServerPushMessage(torrent.ServerPushMessage{MessageType: "serverPushMessage", MessageLevel: "error", Payload: "Unable to add magnetlink to client!"}, conn)
						continue MessageLoop //continue out of the loop entirely for this message since we hit an error
					}

					tclient.StartDownload(clientTorrent)

					Logger.WithFields(logrus.Fields{"clientTorrent": clientTorrent, "magnetLink": magnetLink}).Info("Adding torrent to client!")
					torrent.CreateServerPushMessage(torrent.ServerPushMessage{MessageType: "serverPushMessage", MessageLevel: "info", Payload: "Received MagnetLink"}, conn)
				}

			case "stopTorrents":
				//torrentHashes := payloadData["TorrentHashes"].([]interface{})
				//torrent.CreateServerPushMessage(torrent.ServerPushMessage{MessageType: "serverPushMessage", MessageLevel: "info", Payload: "Received Stop Request"}, conn)
				//for _, singleTorrent := range tclient.Torrents() {
				//	for _, singleSelection := range torrentHashes {
				//
				//			torrent.StopTorrent(singleTorrent, )
				//
				//	}
				//}

			case "deleteTorrents":
				//torrentHashes := payloadData["TorrentHashes"].([]interface{})
				//withData := payloadData["WithData"].(bool) //Checking if torrents should be deleted with data
				//torrent.CreateServerPushMessage(torrent.ServerPushMessage{MessageType: "serverPushMessage", MessageLevel: "info", Payload: "Received Delete Request"}, conn)
				//Logger.WithFields(logrus.Fields{"deleteTorrentsPayload": msg.Payload, "torrentlist": msg.Payload, "deleteWithData?": withData}).Info("message for deleting torrents")
				//for _, singleTorrent := range runningTorrents {
				//	for _, singleSelection := range torrentHashes {
				//		if singleTorrent.InfoHash().String() == singleSelection {
				//			oldTorrentInfo := Storage.FetchTorrentFromStorage(db, singleTorrent.InfoHash().String())
				//			torrentQueues = Storage.FetchQueues(db)
				//
				//			Logger.WithFields(logrus.Fields{"selection": singleSelection}).Info("Matched for deleting torrents")
				//			if withData {
				//				oldTorrentInfo.TorrentStatus = "DroppedData" //Will be cleaned up the next engine loop since deleting a torrent mid loop can cause issues
				//			} else {
				//				oldTorrentInfo.TorrentStatus = "Dropped"
				//			}
				//			Storage.UpdateStorageTick(db, oldTorrentInfo)
				//			Storage.UpdateQueues(db, torrentQueues)
				//		}
				//	}
				//}

			case "startTorrents":
				//torrentHashes := payloadData["TorrentHashes"].([]interface{})
				//Logger.WithFields(logrus.Fields{"selection": msg.Payload}).Info("Matched for starting torrents")
				//torrent.CreateServerPushMessage(torrent.ServerPushMessage{MessageType: "serverPushMessage", MessageLevel: "info", Payload: "Received Start Request"}, conn)
				//for _, singleTorrent := range runningTorrents {
				//	for _, singleSelection := range torrentHashes {
				//		if singleTorrent.InfoHash().String() == singleSelection {
				//			Logger.WithFields(logrus.Fields{"infoHash": singleTorrent.InfoHash().String()}).Info("Found matching torrent to start")
				//			oldTorrentInfo := Storage.FetchTorrentFromStorage(db, singleTorrent.InfoHash().String())
				//			Logger.WithFields(logrus.Fields{"Torrent": oldTorrentInfo.TorrentName}).Info("Changing database to torrent running with 80 max connections")
				//			oldTorrentInfo.TorrentStatus = "ForceStart"
				//			oldTorrentInfo.MaxConnections = 80
				//			Storage.UpdateStorageTick(db, oldTorrentInfo) //Updating the torrent status
				//			torrent.AddTorrentToForceStart(&oldTorrentInfo, singleTorrent, db)
				//
				//		}
				//		torrentQueues = Storage.FetchQueues(db)
				//		if len(torrentQueues.ActiveTorrents) > Config.MaxActiveTorrents { //Since we are starting a new torrent stop the last torrent in the que if running is full
				//			//removeTorrent := torrentQueues.ActiveTorrents[len(torrentQueues.ActiveTorrents)-1]
				//			removeTorrent := torrentQueues.ActiveTorrents[len(torrentQueues.ActiveTorrents)-1]
				//			for _, singleTorrent := range runningTorrents {
				//				if singleTorrent.InfoHash().String() == removeTorrent {
				//					oldTorrentInfo := Storage.FetchTorrentFromStorage(db, singleTorrent.InfoHash().String())
				//					torrent.RemoveTorrentFromActive(&oldTorrentInfo, singleTorrent, db)
				//					Storage.UpdateStorageTick(db, oldTorrentInfo)
				//				}
				//			}
				//		}
				//	}
				//}

			case "forceUploadTorrents": //TODO allow force to override total limit of queued torrents?
				//torrentHashes := payloadData["TorrentHashes"].([]interface{})
				//Logger.WithFields(logrus.Fields{"selection": msg.Payload}).Info("Matched for force Uploading Torrents")
				//torrent.CreateServerPushMessage(torrent.ServerPushMessage{MessageType: "serverPushMessage", MessageLevel: "info", Payload: "Received Force Start Request"}, conn)
				//for _, singleTorrent := range runningTorrents {
				//	for _, singleSelection := range torrentHashes {
				//		if singleTorrent.InfoHash().String() == singleSelection {
				//			Logger.WithFields(logrus.Fields{"infoHash": singleTorrent.InfoHash().String()}).Debug("Found matching torrent to force start")
				//			oldTorrentInfo := Storage.FetchTorrentFromStorage(db, singleTorrent.InfoHash().String())
				//			oldTorrentInfo.TorrentUploadLimit = false // no upload limit for this torrent
				//			oldTorrentInfo.TorrentStatus = "Running"
				//			oldTorrentInfo.MaxConnections = 80
				//			Logger.WithFields(logrus.Fields{"NewMax": oldTorrentInfo.MaxConnections, "Torrent": oldTorrentInfo.TorrentName}).Info("Setting max connection from zero to 80")
				//			Storage.UpdateStorageTick(db, oldTorrentInfo) //Updating the torrent status
				//		}
				//	}
				//}

			default:
				Logger.WithFields(logrus.Fields{"message": msg}).Info("Unrecognized Message from client... ignoring")
				return
			}
		}

	})
	if Config.UseReverseProxy {
		err := http.ListenAndServe(httpAddr, handlers.ProxyHeaders(router))
		if err != nil {
			Logger.WithFields(logrus.Fields{"error": err}).Fatal("Unable to listen on the http Server!")
		}
	} else {
		err := http.ListenAndServe(httpAddr, nil) //Can't send proxy headers if not used since that can be a security issue
		if err != nil {
			Logger.WithFields(logrus.Fields{"error": err}).Fatal("Unable to listen on the http Server! (Maybe wrong IP in config, port already in use?) (Config: Not using proxy, see error for more details)")
		}
	}
}
