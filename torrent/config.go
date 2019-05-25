package torrent

import "github.com/anacrolix/torrent"

type ClientConfig struct {
	Cc *torrent.ClientConfig

	MaxActiveNum int
}
