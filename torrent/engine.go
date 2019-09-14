package torrent //main file for all the calculations and data gathering needed for creating the running torrent arrays

import (
	"fmt"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/gorilla/websocket"
	"sort"
	"strconv"
	"strings"
	"time"
)

//Conn is the injected variable for the websocket connection
var Conn *websocket.Conn

//CreateServerPushMessage Pushes a message from the server to the client
func CreateServerPushMessage(message ServerPushMessage, conn *websocket.Conn) {
	conn.WriteJSON(message)
}

type ClientDBArray []ClientDB

func (a ClientDBArray) Len() int {
	return len(a)
}
func (a ClientDBArray) Swap(i, j int){
	a[i], a[j] = a[j], a[i]
}
func (a ClientDBArray) Less(i, j int) bool {
	t1, err := time.ParseInLocation("2006-01-02 15:04:05", a[i].DateAdded, time.Local)
	t2, err := time.ParseInLocation("2006-01-02 15:04:05", a[j].DateAdded, time.Local)
	if err == nil && t1.Before(t2) {
		return false
	}
	return true
}

//CreateRunningTorrentArray creates the entire torrent list to pass to client
func CreateRunningTorrentArray(tclient *Client) (RunningTorrentArray []ClientDB) {
	for _, singleTorrent := range tclient.Torrents() {
		fullClientDB := new(ClientDB)
		//Handling deleted torrents here
		var TempHash metainfo.Hash
		TempHash = singleTorrent.to.InfoHash()

		activePeersString := strconv.Itoa(singleTorrent.to.Stats().ActivePeers) //converting to strings
		totalPeersString := fmt.Sprintf("%v", singleTorrent.to.Stats().TotalPeers)
		fullClientDB.StoragePath = ""

		downloadedSizeHumanized := HumanizeBytes(float32(singleTorrent.to.BytesCompleted())) //convert size to GB if needed

		totalSizeHumanized := ""
		PercentDone := ""
		if singleTorrent.to.Info() != nil {
			totalSizeHumanized = HumanizeBytes(float32(singleTorrent.to.Length()))
			PercentDone = fmt.Sprintf("%.2f", float32(singleTorrent.to.BytesCompleted())/float32(singleTorrent.to.Length()))
		}

		fullClientDB.DownloadedSize = downloadedSizeHumanized
		fullClientDB.Size = totalSizeHumanized
		fullClientDB.TorrentHash = TempHash
		fullClientDB.PercentDone = PercentDone
		fullClientDB.DataBytesRead = 0    //used for calculations not passed to client calculating up/down speed
		fullClientDB.DataBytesWritten = 0 //used for calculations not passed to client calculating up/down speed
		fullClientDB.ActivePeers = activePeersString + " / (" + totalPeersString + ")"
		fullClientDB.TorrentHashString = TempHash.String()
		fullClientDB.TorrentName = singleTorrent.to.Name()
		fullClientDB.DateAdded = singleTorrent.dateAdded
		fullClientDB.TorrentLabel = TempHash.String()
		fullClientDB.BytesCompleted = singleTorrent.to.BytesCompleted()

		fullClientDB.Status = singleTorrent.CalculateTorrentStatus()

		fullClientDB.TotalUploadedSize = ""
		fullClientDB.UploadRatio = ""

		RunningTorrentArray = append(RunningTorrentArray, *fullClientDB)
	}
	sort.Sort(ClientDBArray(RunningTorrentArray))
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
