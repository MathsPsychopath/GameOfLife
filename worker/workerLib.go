package main

import (
	"uk.ac.bris.cs/gameoflife/util"
)

// always make sure to call on the updated world, with old row, col, neighbours
func (w *Worker) useGOLRules(row, column int, neighbours byte) byte {
	if neighbours < 2 || neighbours > 3 {
		return 0x00
	} else if w.container.CurrentWorld[row][column] == 0x00 && neighbours == 3 {
		return 0xFF
	} else {
		return w.container.CurrentWorld[row][column]
	}
}

// returns a 2d slice of size rows x columns
func createNewSlice(rows, columns int) [][]byte {
	world := make([][]byte, rows)
	for i := range world {
		world[i] = make([]byte, columns)
	}
	return world
}

// count the number of neighbours that a particular cell has in the world.
// Single workers don't have halos so we need to do modular arithmetic on height+width
func (w *Worker) singleWorkerNeighbourCount(row, column int) byte {
	var alive byte = 0
	// positions of neighbouring cells relative to current cell
	offsets := []util.Cell{
		{X: -1, Y: -1},
		{X: -1, Y: 0},
		{X: -1, Y: 1},
		{X: 0, Y: -1},
		{X: 0, Y: 1},
		{X: 1, Y: -1},
		{X: 1, Y: 0},
		{X: 1, Y: 1},
	}
	for _, offset := range offsets {
		actualRow := (row + offset.X) & w.heightBitmask
		actualCol := (column + offset.Y) & w.widthBitmask
		// if actualRow < 0 {
		// 	actualRow = w.height - 1
		// }
		// if actualRow == w.height {
		// 	actualRow = 0
		// }
		// if actualCol == w.width {
		// 	actualCol = 0 
		// }
		// if actualCol < 0 {
		// 	actualCol = w.width - 1
		// }
		alive += (w.container.CurrentWorld[actualRow][actualCol] & 0xFF >> 7)
	// 	if w.container.CurrentWorld[actualRow][actualCol] == 0xFF {
	// 		alive++
	// 	}
	}
	return alive
}

// IMPORTANT: Make sure to mutex lock in calling scope
func (w *Worker) singleWorkerGOL(newWorld [][]byte) []util.Cell {
	flippedCells := []util.Cell{}
	for i, row := range(w.container.CurrentWorld) {
		for j, cell := range(row) {
			neighbours := w.singleWorkerNeighbourCount(i, j)
			newCell := w.useGOLRules(i, j, neighbours)
			newWorld[i][j] = newCell
			if cell ^ newCell == 0xff{
				flippedCells = append(flippedCells, util.Cell{X:j, Y: i})
			}
		}
	}
	return flippedCells
}

// duplicate to avoid having branching instruction in counting
func (w *Worker) multiWorkerNeighbourCount(row, column int, worldWithHalos [][]byte) byte {
	var alive byte = 0
	// positions of neighbouring cells relative to current cell
	offsets := []util.Cell{
		{X: -1, Y: -1},
		{X: -1, Y: 0},
		{X: -1, Y: 1},
		{X: 0, Y: -1},
		{X: 0, Y: 1},
		{X: 1, Y: -1},
		{X: 1, Y: 0},
		{X: 1, Y: 1},
	}
	for _, offset := range offsets {
		actualRow := (row + offset.Y)
		actualCol := (column + offset.X) & w.widthBitmask
		alive += (worldWithHalos[actualRow][actualCol] & 0xFF >> 7)
	}
	return alive
}

func (w *Worker) multiWorkerGOL(newWorld [][]byte, topHalo, bottomHalo []byte) []util.Cell {
	flippedCells := []util.Cell{}
	worldWithHalos := [][]byte{topHalo}
	worldWithHalos = append(worldWithHalos, w.container.CurrentWorld...)
	worldWithHalos = append(worldWithHalos, bottomHalo)
	for i := 1; i < len(worldWithHalos) - 1; i++ {
		for j, cell := range worldWithHalos[i] {
			
			neighbours := w.multiWorkerNeighbourCount(i, j, worldWithHalos)
			newCell := w.useGOLRules(i - 1, j, neighbours)
			newWorld[i - 1][j] = newCell
			if cell ^ newCell == 0xFF {
				flippedCells = append(flippedCells, util.Cell{X: j, Y: i-1 + w.offset})
			}
		}
	}
	return flippedCells
}