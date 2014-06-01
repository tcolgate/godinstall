package main

import "errors"

type Governor struct {
	Max   int
	locks chan int
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
	f()
	defer func() { g.locks <- lock }()
}

func (g *Governor) RunExclusive(f func()) {
	holding := 0
	defer func() {
		for i := 0; i < holding; i++ {
			g.locks <- 1
		}
	}()

	for i := 0; i < g.Max; i++ {
		<-g.locks
		holding++
	}

	f()
}
