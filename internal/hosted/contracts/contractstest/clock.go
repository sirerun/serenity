package contractstest

import (
	"sync"
	"time"
)

// Clock is a manually advanced clock so lease expiry is deterministic.
type Clock struct {
	mu sync.Mutex
	t  time.Time
}

// NewClock starts at a fixed instant; tests never depend on wall time.
func NewClock() *Clock { return &Clock{t: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)} }

func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}
