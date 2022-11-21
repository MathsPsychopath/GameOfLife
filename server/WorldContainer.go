package main

import "sync"

type WorldContainer struct {
	mu 		sync.Mutex
	world   [][]byte
}

// may need an implementation that changes based on cell flipped events
func (w *WorldContainer) update(world [][]byte) {
	w.mu.Lock()
	w.world = world
	w.mu.Unlock()
}