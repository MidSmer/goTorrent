package torrent

import (
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"sync"
)

type Client struct {
	cl *torrent.Client

	_mu sync.RWMutex

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
	torrents := []*Torrent{}

	for _, torrent := range cl.Torrents() {
		switch state {
		case Downloading:
			if torrent.isActive && !torrent.isGetMatainfo && torrent.to.Stats().ActivePeers > 0 && torrent.to.BytesMissing() > 0 {
				torrents = append(torrents, torrent)
			}
		case Seeding:
			if torrent.isActive && torrent.to.Seeding() && torrent.to.Stats().ActivePeers > 0 && torrent.to.BytesMissing() == 0 {
				torrents = append(torrents, torrent)
			}
		case Completed:
			if torrent.to.Stats().ActivePeers == 0 && torrent.to.BytesMissing() == 0 {
				torrents = append(torrents, torrent)
			}
		case Paused:
			if torrent.isPaused {
				torrents = append(torrents, torrent)
			}
		case Active:
			if torrent.isActive {
				torrents = append(torrents, torrent)
			}
		case Inactive:
			if torrent.isActive && torrent.to.Stats().ActivePeers == 0 && torrent.to.BytesMissing() > 0 {
				torrents = append(torrents, torrent)
			}
		case Errored:
			if torrent.isErrored {
				torrents = append(torrents, torrent)
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
	}()
}

func (cl *Client) SetConns(t *Torrent, connsCount int) (old int, err error) {
	t.to.NewReader()
	old = t.to.SetMaxEstablishedConns(connsCount)

	return
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
