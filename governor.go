package main

import (
	"errors"
	"sync"
)

type req struct{}

// The governor is used for rate liiting requests, and for locking
// the repository from new requests when an regeneration is
// occuring
type Governor struct {
	Max    int          // Maximum number of concurrent requests
	reqs   chan req     // Channel for tracking in-flight requests
	rwLock sync.RWMutex // A RW mutex for locking out reads during  an update
}

func NewGovernor(max int) (*Governor, error) {
	var g Governor
	g.Max = max

	if g.Max != 0 {
		g.reqs = make(chan req, g.Max)
		for i := 0; i < g.Max; i++ {
			g.reqs <- req{}
		}
	}

	return &g, nil
}

// Take a read lock on this governor
func (g *Governor) ReadLock() {
	if g.Max != 0 {
		_ = <-g.reqs
	}
	g.rwLock.RLock()
}

// Release a read lock
func (g *Governor) ReadUnLock() {
	g.rwLock.RUnlock()
	if g.Max != 0 {
		g.reqs <- req{}
	}
}

// Take a write lock. THis shoudl block  until all readers
// are complete
func (g *Governor) WriteLock() {
	if g.Max != 0 {
		for i := 0; i < g.Max; i++ {
			_ = <-g.reqs
		}
	}
	g.rwLock.Lock()
}

// Release the write lock
func (g *Governor) WriteUnLock() (err error) {
	g.rwLock.Unlock()
	if g.Max != 0 {
		if len(g.reqs) != 0 {
			return errors.New("Tried to unlock when lock not exclusively held")
		} else {
			for i := 0; i < g.Max; i++ {
				g.reqs <- req{}
			}
		}
	}
	return nil
}
