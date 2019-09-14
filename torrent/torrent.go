package torrent

import (
	raw_torrent "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

type Torrent struct {
	to *raw_torrent.Torrent

	hash				metainfo.Hash

	dateAdded       	string

	isErrored  			bool
	isPaused	 		bool
	isActive	 		bool
	isGetMatainfo	    bool
}

func (t *Torrent) InfoHash() metainfo.Hash {
	return t.hash
}

func (t *Torrent) StopTorrent() {
	t.isPaused = true
	t.to.SetMaxEstablishedConns(0)
}

func (t *Torrent) CalculateTorrentStatus() string {
	if t.isActive && t.isGetMatainfo {
		return "GetMatainfo"
	}

	if t.isPaused {
		return "Stopped"
	}

	if t.to.Info() == nil {
		return "Unknown"
	}

	if t.isActive && !t.isGetMatainfo && t.to.Stats().ActivePeers > 0 && t.to.BytesMissing() > 0 {
		return "Downloading"
	}

	if t.isActive && t.to.Seeding() && t.to.Stats().ActivePeers > 0 && t.to.BytesMissing() == 0 {
		return "Seeding"
	}

	if t.to.Stats().ActivePeers == 0 && t.to.BytesMissing() == 0 {
		return "Completed"
	}

	return "Unknown"
}