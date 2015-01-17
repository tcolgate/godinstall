package main

import (
	"errors"
	"log"
	"runtime/debug"
	"sync"
)

type req struct{}

// Governor is used for rate liiting requests, and for locking
// the repository from new requests when an regeneration is
// occuring
type Governor struct {
	Max    int          // Maximum number of concurrent requests
	reqs   chan req     // Channel for tracking in-flight requests
	rwLock sync.RWMutex // A RW mutex for locking out reads during  an update
}

// NewGovernor creates a governor that will limit users to max current
// readers at any one time
func NewGovernor(max int) *Governor {
	var g Governor
	g.Max = max

	if g.Max != 0 {
		g.reqs = make(chan req, g.Max)
		for i := 0; i < g.Max; i++ {
			g.reqs <- req{}
		}
	}

	return &g
}

// ReadLock takes a read lock on this governor
func (g *Governor) ReadLock() {
	log.Println("read loccked")
	debug.PrintStack()
	if g.Max != 0 {
		_ = <-g.reqs
	}
	g.rwLock.RLock()
}

// ReadUnLock releases a read lock
func (g *Governor) ReadUnLock() {
	log.Println("read unloccked")
	debug.PrintStack()
	g.rwLock.RUnlock()
	if g.Max != 0 {
		g.reqs <- req{}
	}
}

// WriteLock takes a write lock. THis shoudl block  until all readers
// are complete
func (g *Governor) WriteLock() {
	log.Println("write loccked")
	debug.PrintStack()

	if g.Max != 0 {
		for i := 0; i < g.Max; i++ {
			_ = <-g.reqs
		}
	}
	g.rwLock.Lock()
}

// WriteUnLock releases the write lock
func (g *Governor) WriteUnLock() (err error) {
	log.Println("write unloccked")
	debug.PrintStack()
	g.rwLock.Unlock()
	if g.Max != 0 {
		if len(g.reqs) != 0 {
			return errors.New("Tried to unlock when lock not exclusively held")
		}
		for i := 0; i < g.Max; i++ {
			g.reqs <- req{}
		}
	}
	return nil
}
