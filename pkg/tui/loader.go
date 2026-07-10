package tui

import "time"

var loaderFrames = []string{
	"⛵‿‿‿‿‿‿‿‿‿‿",
	"‿⛵‿‿‿‿‿‿‿‿‿",
	"‿‿⛵‿‿‿‿‿‿‿‿",
	"‿‿‿⛵‿‿‿‿‿‿‿",
	"‿‿‿‿⛵‿‿‿‿‿‿",
	"‿‿‿‿‿⛵‿‿‿‿‿",
	"‿‿‿‿‿‿⛵‿‿‿‿",
	"‿‿‿‿‿‿‿⛵‿‿‿",
	"‿‿‿‿‿‿‿‿⛵‿‿",
	"‿‿‿‿‿‿‿‿‿⛵‿",
	"‿‿‿‿‿‿‿‿‿‿⛵",
	"‿‿‿‿‿‿‿‿‿‿‿‿",
	"‿‿‿‿‿‿‿‿‿‿‿‿",
	"‿‿‿‿‿‿‿‿‿‿‿‿",
	"‿‿‿‿‿‿‿‿‿‿‿‿",
	"‿‿‿‿‿‿‿‿‿‿‿‿",
	"‿‿‿‿‿‿‿‿‿‿‿‿",
}

const loaderInterval = 170 * time.Millisecond

type Loader struct {
	started time.Time
}

func NewLoader() *Loader {
	// banstructlit:ignore
	return &Loader{started: time.Now()}
}

func (l *Loader) Render() string {
	idx := int(time.Since(l.started)/loaderInterval) % len(loaderFrames)
	return loaderFrames[idx]
}
