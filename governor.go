package main

import (
	"errors"
	"sync"
)

// The governor is used for rate liiting requests, and for locking
// the repository from new requests when an apt-ftparchive run is
// occuring
type Governor struct {
	Max    int
	locks  chan int
	rwLock sync.RWMutex
}

func NewGovernor(max int) (*Governor, error) {
	var g Governor
	g.Max = max

	if max <= 0 {
		return nil, errors.New("Must secify a limit for the governor")
	}

	g.locks = make(chan int, g.Max)
	for i := 0; i < g.Max; i++ {
		g.locks <- 1
	}

	return &g, nil
}

func (g *Governor) Run(f func()) {
	lock := <-g.locks
	g.rwLock.RLock()
	defer func() {
		g.locks <- lock
		g.rwLock.RUnlock()
	}()

	f()
}

func (g *Governor) RunExclusive(f func()) {
	g.rwLock.Lock()
	defer g.rwLock.Unlock()

	f()
}

func (g *Governor) ReadLock() {
	g.rwLock.Lock()
}

func (g *Governor) ReadUnLock() {
	g.rwLock.Unlock()
}

func (g *Governor) WriteLock() {
	g.rwLock.Lock()
}

func (g *Governor) WriteUnLock() {
	g.rwLock.Unlock()
}
