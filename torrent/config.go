package torrent

import (
	"github.com/anacrolix/torrent"
	"github.com/asdine/storm"
)

type ClientConfig struct {
	Cc *torrent.ClientConfig

	DefaultStorage *storm.DB
	MaxActiveNum int
}
