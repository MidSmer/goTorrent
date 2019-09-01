package torrent

import "github.com/sirupsen/logrus"

type downloadState int

const (
	Downloading downloadState = iota
	Seeding
	Completed
	Paused
	Active
	Inactive
	Errored
)

//Logger is the injected variable for global logger
var Logger *logrus.Logger
