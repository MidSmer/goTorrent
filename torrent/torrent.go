package torrent

import (
	raw_torrent "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/sirupsen/logrus"
)

type Torrent struct {
	to *raw_torrent.Torrent
	cl *Client

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

func (t *Torrent) Magnet() string {
	mi := t.to.Metainfo()
	return mi.Magnet(t.to.Name(), t.to.InfoHash()).String()
}

func (t *Torrent) StopTorrent() {
	t.isPaused = true
	t.to.SetMaxEstablishedConns(0)
	t.save()
}

func (t *Torrent) DeleteTorrent() {
	t.isActive = false
	t.to.Drop()
	t.delete()
}

func (t *Torrent) save() {
	storage := TorrentStorage{}
	e := t.cl.storage.One("Hash", t.to.InfoHash().String(), &storage)
	if e != nil {
		Logger.WithFields(logrus.Fields{"error": e}).Fatal("Err read storage.db")
	}

	storage.IsPaused = t.isPaused
	storage.IsActive = t.isActive
	storage.IsGetMatainfo = t.isGetMatainfo

	e = t.cl.storage.Update(&storage)
	if e != nil {
		Logger.WithFields(logrus.Fields{"error": e}).Fatal("Err write storage.db")
	}
}

func (t *Torrent)delete()  {
	storage := TorrentStorage{}
	e := t.cl.storage.One("Hash", t.to.InfoHash().String(), &storage)
	if e != nil {
		Logger.WithFields(logrus.Fields{"error": e}).Fatal("Err read storage.db")
	}

	e = t.cl.storage.DeleteStruct(&storage)
	if e != nil {
		Logger.WithFields(logrus.Fields{"error": e}).Fatal("Err delete storage.db")
	}
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