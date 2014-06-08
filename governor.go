package main

import (
	"errors"
	"sync"
)

type req struct{}

// The governor is used for rate liiting requests, and for locking
// the repository from new requests when an apt-ftparchive run is
// occuring
type Governor struct {
	Max    int
	reqs   chan req
	rwLock sync.RWMutex
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

// These are almost certainly wrong and need
// to deal witht he two locks seperatley

func (g *Governor) ReadLock() {
	if g.Max != 0 {
		_ = <-g.reqs
	}
	g.rwLock.RLock()
}

func (g *Governor) ReadUnLock() {
	g.rwLock.RUnlock()
	if g.Max != 0 {
		g.reqs <- req{}
	}
}

func (g *Governor) WriteLock() {
	if g.Max != 0 {
		for i := 0; i < g.Max; i++ {
			_ = <-g.reqs
		}
	}
	g.rwLock.Lock()
}

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
