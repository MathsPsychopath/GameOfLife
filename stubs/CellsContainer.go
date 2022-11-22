package stubs

import (
	"sync"

	"uk.ac.bris.cs/gameoflife/util"
)

type CellsContainer struct {
	Mu 		*sync.Mutex
	Cells   []util.Cell
	Turn    int
}

func (c *CellsContainer) Get() ([]util.Cell, int) {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	return c.Cells, c.Turn
}

func (c *CellsContainer) Update(k []util.Cell, turn int) {
	c.Mu.Lock()
	c.Cells = k
	c.Turn = turn
	c.Mu.Unlock()
}
