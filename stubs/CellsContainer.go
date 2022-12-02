package stubs

import (
	"sync"

	"uk.ac.bris.cs/gameoflife/util"
)

type CellsContainer struct {
	Mu           *sync.Mutex
	CurrentWorld [][]byte
	Turn         int
}

// initialise the container with a mutex lock
func NewCellsContainer() *CellsContainer {
	container := &CellsContainer{}
	container.Mu = new(sync.Mutex)
	return container
}

// set the container to point to some initial state
func (c *CellsContainer) UpdateWorld(world [][]byte) {
	c.Mu.Lock()
	c.CurrentWorld = world
	c.Mu.Unlock()
}

// update the current world with the flipped cells
func (c *CellsContainer) UpdateWorldAndTurn(flippedCells []util.Cell, turn int) {
	c.Mu.Lock()
	c.Turn = turn
	for _, cell := range flippedCells {
		// inversion faster without branching
s		c.CurrentWorld[cell.Y][cell.X] ^= 0xFF
	}
	c.Mu.Unlock()
}
func (c *CellsContainer) GetAliveCount() (int, int) {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	return len(GetAliveCells(c.CurrentWorld)), c.Turn
}

func (c *CellsContainer) Get() ([][]byte, int) {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	return c.CurrentWorld, c.Turn
}
