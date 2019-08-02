package torrent

import "github.com/anacrolix/torrent/metainfo"

//TorrentLocal is local storage of the torrents for readd on server restart, marshalled into the database using Storm
type TorrentStorage struct {
	Hash            string `storm:"id,unique"` //Hash should be unique for every torrent... if not we are re-adding an already added torrent
	InfoBytes       []byte
	DateAdded       string
	StoragePath     string //The absolute value of the path where the torrent will be moved when completed
	TempStoragePath string //The absolute path of where the torrent is temporarily stored as it is downloaded
	TorrentName     string
	Trackers        [][]string
	InfoHash        metainfo.Hash
	Label           string //User enterable label to sort torrents by

	IsGetMatainfo bool
	IsActive      bool
	IsPaused      bool
}
