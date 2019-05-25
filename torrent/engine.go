package torrent //main file for all the calculations and data gathering needed for creating the running torrent arrays

import (
	"fmt"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	"strconv"
	"strings"
)

//Logger is the injected variable for global logger
var Logger *logrus.Logger

//Conn is the injected variable for the websocket connection
var Conn *websocket.Conn

//CreateServerPushMessage Pushes a message from the server to the client
func CreateServerPushMessage(message ServerPushMessage, conn *websocket.Conn) {
	conn.WriteJSON(message)
}

//CreateRunningTorrentArray creates the entire torrent list to pass to client
func CreateRunningTorrentArray(tclient *Client) (RunningTorrentArray []ClientDB) {
	for _, singleTorrent := range tclient.TorrentStateFilter(Downloading) {
		fullClientDB := new(ClientDB)
		//Handling deleted torrents here
		var TempHash metainfo.Hash
		TempHash = singleTorrent.to.InfoHash()

		activePeersString := strconv.Itoa(singleTorrent.to.Stats().ActivePeers) //converting to strings
		totalPeersString := fmt.Sprintf("%v", singleTorrent.to.Stats().TotalPeers)
		fullClientDB.StoragePath = ""

		downloadedSizeHumanized := HumanizeBytes(float32(singleTorrent.to.BytesCompleted())) //convert size to GB if needed
		totalSizeHumanized := HumanizeBytes(float32(singleTorrent.to.Length()))

		fullClientDB.DownloadedSize = downloadedSizeHumanized
		fullClientDB.Size = totalSizeHumanized
		PercentDone := fmt.Sprintf("%.2f", float32(singleTorrent.to.BytesCompleted())/float32(singleTorrent.to.Length()))
		fullClientDB.TorrentHash = TempHash
		fullClientDB.PercentDone = PercentDone
		fullClientDB.DataBytesRead = 0    //used for calculations not passed to client calculating up/down speed
		fullClientDB.DataBytesWritten = 0 //used for calculations not passed to client calculating up/down speed
		fullClientDB.ActivePeers = activePeersString + " / (" + totalPeersString + ")"
		fullClientDB.TorrentHashString = TempHash.String()
		fullClientDB.TorrentName = singleTorrent.to.Name()
		fullClientDB.DateAdded = ""
		fullClientDB.TorrentLabel = TempHash.String()
		fullClientDB.BytesCompleted = singleTorrent.to.BytesCompleted()

		fullClientDB.TotalUploadedSize = ""
		fullClientDB.UploadRatio = ""

		RunningTorrentArray = append(RunningTorrentArray, *fullClientDB)
	}
	return RunningTorrentArray
}

//CreatePeerListArray create a list of peers for the torrent and displays them
func CreatePeerListArray(tclient *Client, selectedHash string) PeerFileList {
	runningTorrents := tclient.Torrents()
	TorrentPeerList := PeerFileList{}
	for _, singleTorrent := range runningTorrents {
		tempHash := singleTorrent.to.InfoHash().String()
		if (strings.Compare(tempHash, selectedHash)) == 0 {
			TorrentPeerList.MessageType = "torrentPeerList"
			TorrentPeerList.PeerList = singleTorrent.to.KnownSwarm()
			TorrentPeerList.TotalPeers = len(TorrentPeerList.PeerList)
			return TorrentPeerList
		}
	}
	return TorrentPeerList
}
