package main

import (
	"net/rpc"

	"uk.ac.bris.cs/gameoflife/stubs"
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

func (w *Worker) evolve(outputWorldSlice [][]byte, topHalo, botHalo []byte) []util.Cell {
	w.container.Mu.Lock()
	defer w.container.Mu.Unlock()
	flippedCells := []util.Cell{}
	worldSlice := [][]byte{topHalo}
	worldSlice = append(worldSlice, w.container.CurrentWorld...)
	worldSlice = append(worldSlice, botHalo)
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

func (w *Worker) pushHalos(topDone, botDone chan *rpc.Call) {
	// IMPORTANT: TOP HALO GOES TO BOT WORKER!!!!!!!!!!!!!!!
	// BOT HALO GOES TO TOP WORKER !!!!!!!!!!!!!!!!!!!!!!!!!!
	//todo make below into 2 go routines
	topHaloPushReq := stubs.PushHaloRequest{ //this is the topHalo of the adjacent worker (beneath this one)
		Halo:  w.container.CurrentWorld[w.height-1], //last row
		IsTop: true,
	}
	<-topDone //make sure that previous pushHalo was received
	w.botWorker.Go(stubs.PushHalo, topHaloPushReq, new(stubs.NilResponse), topDone)

	botHaloPushReq := stubs.PushHaloRequest{ //this is the botHalo of the adjacent worker (above this one)
		Halo:  w.container.CurrentWorld[0], //first row
		IsTop: false,
	}
	<-botDone //make sure that previous pushHalo was received by adjacent worker
	w.topWorker.Go(stubs.PushHalo, botHaloPushReq, new(stubs.NilResponse), botDone)
}
