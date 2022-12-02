package main

import (
	"fmt"

	"uk.ac.bris.cs/gameoflife/util"
)

// always make sure to call on the updated world, with old row, col, neighbours
func (w *Worker) getNewCell(cellValue, neighbours byte) byte {
	if neighbours < 2 || neighbours > 3 {
		return 0x00
	} else if cellValue == 0x00 && neighbours == 3 {
		return 0xFF
	} else {
		return cellValue
	}
}

// returns a 2d slice of size rows x columns
func createNewSlice(rowCount, colCount int) [][]byte {
	world := make([][]byte, rowCount)
	for i := range world {
		world[i] = make([]byte, colCount)
	}
	return world
}

// duplicate to avoid having branching instruction in counting
func (w *Worker) getNeighbourCount(row, column int, worldWithHalos [][]byte) byte {
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
		neighbourRow := (row + offset.Y)
		widthBitMask := w.width - 1
		neighbourCol := (column + offset.X) & widthBitMask                //BEWARE! & widthBitMask ONLY WORKS IF IMAGE SIZE IS 2^N
		alive += (worldWithHalos[neighbourRow][neighbourCol] & 0xFF >> 7) //either 00000000 >>7 == 0 or 11111111 >> 7 = 00000001(because of undeflow)
	}
	return alive
}

// joins slice with Halo
func (w *Worker) constructWorld(topHalo, botHalo []byte) [][]byte {
	if topHalo == nil {
		fmt.Println("running single worker GOL")
		topHalo = w.container.CurrentWorld[len(w.container.CurrentWorld)-1] //topHalo to bottom row
		botHalo = w.container.CurrentWorld[0]                               //botHalo to top row
	} else {
		fmt.Println("running multi-worker GOL") //else statement only for debugging purposes
	}
	output := [][]byte{topHalo}
	output = append(output, w.container.CurrentWorld...)
	output = append(output, botHalo)
	return output
}

func (w *Worker) evolve(outputWorldSlice [][]byte, topHalo, botHalo []byte) []util.Cell {
	w.container.Mu.Lock()
	defer w.container.Mu.Unlock()
	flippedCells := []util.Cell{}
	worldSlice := w.constructWorld(topHalo, botHalo)

	// i starts at 1 and ends at len-1 because we are not iterating through halo-rows
	for i := 1; i < len(worldSlice)-1; i++ {
		for j, cell := range worldSlice[i] {

			neighbourCount := w.getNeighbourCount(i, j, worldSlice)
			newCell := w.getNewCell(worldSlice[i][j], neighbourCount)
			outputWorldSlice[i-1][j] = newCell
			// generate flipped cells list
			if cell^newCell == 0xFF {
				flippedCells = append(flippedCells, util.Cell{X: j, Y: i - 1 + w.rowOffset})
			}
		}
	}
	return flippedCells
}
