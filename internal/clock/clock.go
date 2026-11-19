// Package clock is the Platform's only source of time, so tests can move it.
package clock

import (
	"sync"
	"time"
)

// Clock is real time plus an offset; the offset only changes in test builds.
type Clock struct {
	mu     sync.RWMutex
	offset time.Duration
}

func New() *Clock { return &Clock{} }

func (c *Clock) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return time.Now().Add(c.offset)
}

func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.offset += d
}
