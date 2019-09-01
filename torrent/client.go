package torrent

import (
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/asdine/storm"
	"github.com/sirupsen/logrus"
	"sync"
	"time"
)

type Client struct {
	cl *torrent.Client

	_mu sync.RWMutex

	storage      *storm.DB
	torrents     map[torrent.InfoHash]*Torrent
	maxActiveNum int
}

func NewClient(cfg *ClientConfig) (cl *Client, err error) {
	client, clientErr := torrent.NewClient(cfg.Cc)

	if clientErr != nil {
		panic(clientErr)
	}

	cl = &Client{
		cl: client,

		torrents:     make(map[metainfo.Hash]*Torrent),
		maxActiveNum: cfg.MaxActiveNum,
	}

	cl.storage = cfg.DefaultStorage

	cl.Recovery()

	return
}

// Returns handles to all the torrents loaded in the Client.
func (cl *Client) Torrents() []*Torrent {
	cl.lock()
	defer cl.unlock()
	return cl.torrentsAsSlice()
}

func (cl *Client) torrentsAsSlice() (ret []*Torrent) {
	for _, t := range cl.torrents {
		ret = append(ret, t)
	}
	return
}

func (cl *Client) TorrentStateFilter(state downloadState) []*Torrent {
	var torrents []*Torrent

	for _, t := range cl.Torrents() {
		switch state {
		case Downloading:
			if t.isActive && !t.isGetMatainfo && t.to.Stats().ActivePeers > 0 && t.to.BytesMissing() > 0 {
				torrents = append(torrents, t)
			}
		case Seeding:
			if t.isActive && t.to.Seeding() && t.to.Stats().ActivePeers > 0 && t.to.BytesMissing() == 0 {
				torrents = append(torrents, t)
			}
		case Completed:
			if t.to.Stats().ActivePeers == 0 && t.to.BytesMissing() == 0 {
				torrents = append(torrents, t)
			}
		case Paused:
			if t.isPaused {
				torrents = append(torrents, t)
			}
		case Active:
			if t.isActive {
				torrents = append(torrents, t)
			}
		case Inactive:
			if t.isActive && t.to.Stats().ActivePeers == 0 && t.to.BytesMissing() > 0 {
				torrents = append(torrents, t)
			}
		case Errored:
			if t.isErrored {
				torrents = append(torrents, t)
			}
		}
	}

	return torrents
}

func (cl *Client) AddTorrentSpec(spec *torrent.TorrentSpec) (t *Torrent, new bool, err error) {
	T, N, err := cl.cl.AddTorrentSpec(spec)

	if err != nil {
		return
	}

	new = N

	t = &Torrent{
		to:   T,
		hash: T.InfoHash(),

		isActive:      true,
		isPaused:      false,
		isGetMatainfo: true,
	}

	var storage TorrentStorage
	e := cl.storage.One("Hash", t.to.InfoHash().String(), &storage)
	if e != nil {
		if e != storm.ErrNotFound {
			Logger.WithFields(logrus.Fields{"error": e}).Fatal("Err read storage.db")
		}
	}

	if storage.Hash == "" {
		storage = TorrentStorage{
			Hash:          T.InfoHash().String(),
			DateAdded:     time.Now().String(),
			TorrentName:   spec.DisplayName,
			Trackers:      spec.Trackers,
			InfoHash:      T.InfoHash(),
			IsActive:      true,
			IsPaused:      false,
			IsGetMatainfo: true,
		}

		e := cl.storage.Save(&storage)
		if e != nil {
			Logger.WithFields(logrus.Fields{"error": e}).Fatal("Err write storage.db")
		}
	}

	cl.torrents[T.InfoHash()] = t
	return
}

func (cl *Client) AddMagnet(uri string) (T *Torrent, err error) {
	spec, err := torrent.TorrentSpecFromMagnetURI(uri)
	if err != nil {
		return
	}

	T, _, err = cl.AddTorrentSpec(spec)
	return
}

func (cl *Client) StartDownload(t *Torrent) {
	go func() {
		<-t.to.GotInfo()
		t.isGetMatainfo = false

		t.to.DownloadAll()

		_, err := cl.SetConns(t, 80)

		if err != nil {
			return
		}

		storage := TorrentStorage{}
		e := cl.storage.One("Hash", t.to.InfoHash().String(), &storage)
		if e != nil {
			Logger.WithFields(logrus.Fields{"error": e}).Fatal("Err read storage.db")
		}

		storage.IsGetMatainfo = false
		storage.InfoBytes = t.to.Metainfo().InfoBytes

		e = cl.storage.Update(&storage)
		if e != nil {
			Logger.WithFields(logrus.Fields{"error": e}).Fatal("Err write storage.db")
		}
	}()
}

func (cl *Client) SetConns(t *Torrent, connsCount int) (old int, err error) {
	t.to.NewReader()
	old = t.to.SetMaxEstablishedConns(connsCount)

	return
}

func (cl *Client) Recovery() {
	var storage []TorrentStorage
	e := cl.storage.All(&storage)
	if e != nil {
		Logger.WithFields(logrus.Fields{"error": e}).Fatal("Err read storage.db")
	}

	for _, item := range storage {
		spec := &torrent.TorrentSpec{
			Trackers:    item.Trackers,
			DisplayName: item.TorrentName,
			InfoHash:    item.InfoHash,
		}

		if len(item.InfoBytes) > 0 {
			spec.InfoBytes = item.InfoBytes
		}

		T, _, _ := cl.AddTorrentSpec(spec)
		cl.StartDownload(T)
	}
}

func (cl *Client) rLock() {
	cl._mu.RLock()
}

func (cl *Client) rUnlock() {
	cl._mu.RUnlock()
}

func (cl *Client) lock() {
	cl._mu.Lock()
}

func (cl *Client) unlock() {
	cl._mu.Unlock()
}
