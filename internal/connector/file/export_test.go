package file

import "github.com/fsnotify/fsnotify"

// HandleEventForTest drives one fsnotify event through handleEvent so the
// external test package can exercise the subtree re-watch path directly.
func (c *Connector) HandleEventForTest(name string, op fsnotify.Op) {
	c.handleEvent(fsnotify.Event{Name: name, Op: op})
}
