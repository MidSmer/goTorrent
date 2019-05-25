package torrent

import (
	raw_torrent "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

type Torrent struct {
	to *raw_torrent.Torrent

	hash				metainfo.Hash

	isErrored  			bool
	isPaused	 		bool
	isActive	 		bool
	isGetMatainfo	    bool
}