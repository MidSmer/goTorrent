package torrent

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
