package storage

import (
	"path/filepath"

	Settings "github.com/MidSmer/goTorrent/settings"
	"github.com/asdine/storm"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

//Logger is the global Logger that is used in all packages
var Logger *logrus.Logger

//Conn is the global websocket connection used to push server notification messages
var Conn *websocket.Conn

//TorrentQueues contains the active and queued torrent hashes in slices
type TorrentQueues struct {
	ID             int `storm:"id,unique"` //storm requires unique ID (will be 5)
	ActiveTorrents []string
	QueuedTorrents []string
	ForcedTorrents []string
}

//IssuedTokensList contains a slice of all the tokens issues to applications
type IssuedTokensList struct {
	ID         int `storm:"id,unique"` //storm requires unique ID (will be 3) to save although there will only be one of these
	SigningKey []byte
	TokenNames []SingleToken
	FirstToken string `storm:omitempty`
}

//SingleToken stores a single token and all of the associated information
type SingleToken struct {
	ClientName string
}

//TorrentHistoryList holds the entire history of downloaded torrents by hash TODO implement a way to read this and maybe grab the name for every torrent as well
type TorrentHistoryList struct {
	ID       int `storm:"id,unique"` //storm requires unique ID (will be 2) to save although there will only be one of these
	HashList []string
}

//RSSFeedStore stores all of our RSS feeds in a slice of gofeed.Feed
type RSSFeedStore struct {
	ID       int `storm:"id,unique"` //storm requires unique ID (will be 1) to save although there will only be one of these
	RSSFeeds []SingleRSSFeed         //slice of string containing URL's in string form for gofeed to parse
}

//SingleRSSFeed stores an RSS feed with a list of all the torrents in the feed
type SingleRSSFeed struct {
	URL      string `storm:"id,unique"` //the URL of the individual RSS feed
	Name     string
	Torrents []SingleRSSTorrent //name of the torrents
}

//SingleRSSTorrent stores a single RSS torrent with all the relevant information
type SingleRSSTorrent struct {
	Link    string `storm:"id,unique"`
	Title   string
	PubDate string
}

//TorrentFilePriority stores the priority for each file in a torrent
type TorrentFilePriority struct {
	TorrentFilePath     string
	TorrentFilePriority string
	TorrentFileSize     int64
}

//TorrentLocal is local storage of the torrents for readd on server restart, marshalled into the database using Storm
type TorrentLocal struct {
	Hash                string `storm:"id,unique"` //Hash should be unique for every torrent... if not we are re-adding an already added torrent
	InfoBytes           []byte
	DateAdded           string
	StoragePath         string //The absolute value of the path where the torrent will be moved when completed
	TempStoragePath     string //The absolute path of where the torrent is temporarily stored as it is downloaded
	TorrentMoved        bool   //If completed has the torrent been moved to the end location
	TorrentName         string
	TorrentStatus       string //"Stopped", "Running", "ForceStart"
	TorrentUploadLimit  bool   //if true this torrent will bypass the upload storage limit (effectively unlimited)
	MaxConnections      int    //Max connections that the torrent can have to it at one time
	TorrentType         string //magnet or .torrent file
	TorrentFileName     string //Should be just the name of the torrent
	TorrentFile         []byte //If torrent was from .torrent file, store the entire file for re-adding on restart
	Label               string //User enterable label to sort torrents by
	UploadedBytes       int64  //Total amount the client has uploaded on this torrent
	DownloadedBytes     int64  //Total amount the client has downloaded on this torrent
	TorrentSize         int64  //If we cancel a file change the download size since we won't be downloading that file
	UploadRatio         string
	TorrentFilePriority []TorrentFilePriority //Slice of all the files the torrent contains and the priority of each file
	IsPause             bool
	IsGetMetadata       bool
	IsDownloading       bool
}

func NewStorage(path string) (*storm.DB, error) {
	db, err := storm.Open(filepath.Join(path, "storage.db"))
	return db, err
}

//SaveConfig saves the config to the database to compare for changes to settings.toml on restart
func SaveConfig(torrentStorage *storm.DB, config Settings.FullClientSettings) {
	config.ID = 4
	err := torrentStorage.Save(&config)
	if err != nil {
		Logger.WithFields(logrus.Fields{"database": torrentStorage, "error": err}).Error("Error saving Config to database!")
	}
}

//UpdateQueues Saves the slice of hashes that contain the active Torrents
func UpdateQueues(db *storm.DB, torrentQueues TorrentQueues) {
	torrentQueues.ID = 5
	err := db.Save(&torrentQueues)
	if err != nil {
		Logger.WithFields(logrus.Fields{"database": db, "error": err}).Error("Unable to write Queues to database!")
	}
}

//FetchQueues fetches the activetorrent and queuedtorrent slices from the database
func FetchQueues(db *storm.DB) TorrentQueues {
	torrentQueues := TorrentQueues{}
	err := db.One("ID", 5, &torrentQueues)
	if err != nil {
		Logger.WithFields(logrus.Fields{"database": db, "error": err}).Error("Unable to read Database into torrentQueues!")
		return torrentQueues
	}
	return torrentQueues
}

//FetchConfig fetches the client config from the database
func FetchConfig(torrentStorage *storm.DB) (Settings.FullClientSettings, error) {
	config := Settings.FullClientSettings{}
	err := torrentStorage.One("ID", 4, &config)
	if err != nil {
		Logger.WithFields(logrus.Fields{"database": torrentStorage, "error": err}).Error("Unable to read Database into configFile!")
		return config, err
	}
	return config, err
}

//FetchAllStoredTorrents is called to read in ALL local stored torrents in the boltdb database (called on server restart)
func FetchAllStoredTorrents(torrentStorage *storm.DB) (torrentLocalArray []*TorrentLocal) {
	torrentLocalArray = []*TorrentLocal{} //creating the array of the torrentlocal struct

	err := torrentStorage.All(&torrentLocalArray) //unmarshalling the database into the []torrentlocal
	if err != nil {
		Logger.WithFields(logrus.Fields{"database": torrentStorage, "error": err}).Error("Unable to read Database into torrentLocalArray!")
	}
	return torrentLocalArray //all done, return the entire Array to add to the torrent client
}

//UpdateStorageTick updates the values in boltdb that should update on every tick (like uploadratio or uploadedbytes, not downloaded since we should have the actual file)
func UpdateStorageTick(torrentStorage *storm.DB, torrentLocal TorrentLocal) {
	err := torrentStorage.Update(&torrentLocal)
	if err != nil {
		Logger.WithFields(logrus.Fields{"UpdateContents": torrentLocal, "error": err}).Error("Error performing tick update to database!")
	} else {
		Logger.WithFields(logrus.Fields{"UpdateContents": torrentLocal, "error": err}).Debug("Performed Update to database!")
	}
}

//FetchHashHistory fetches the infohash of all torrents added into the client.  The cron job checks this so as not to add torrents from RSS that were already added before
func FetchHashHistory(db *storm.DB) TorrentHistoryList {
	torrentHistory := TorrentHistoryList{}
	err := db.One("ID", 2, &torrentHistory)
	if err != nil {
		Logger.WithFields(logrus.Fields{"TorrentHistoryList": torrentHistory, "error": err}).Error("Failure retrieving torrent history list, creating bucket for history list, expected behaviour if first run for history list")
		torrentHistory := TorrentHistoryList{}
		torrentHistory.ID = 2
		err = db.Save(&torrentHistory)
		if err != nil {
			Logger.WithFields(logrus.Fields{"RSSFeed": torrentHistory, "error": err}).Error("Error saving torrent History to database!")
		}
		return torrentHistory
	}
	return torrentHistory
}

//FetchJWTTokens fetches the stored client authentication tokens
func FetchJWTTokens(db *storm.DB) IssuedTokensList {
	tokens := IssuedTokensList{}
	err := db.One("ID", 3, &tokens)
	if err != nil {
		Logger.WithFields(logrus.Fields{"Tokens": tokens, "error": err}).Error("Unable to fetch Token database... should always be one token in database")
	}
	return tokens
}
