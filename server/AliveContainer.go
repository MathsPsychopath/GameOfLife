package main

import "sync"

// Mutex locked data to avoid race conditions
type AliveContainer struct {
	mu 		*sync.Mutex
	turn    int
	count   int
}

// update the turn and alive
func (a *AliveContainer) update(newTurn, newAlive int) {
	a.mu.Lock()
	a.turn = newTurn
	a.count = newAlive
	a.mu.Unlock()
}
