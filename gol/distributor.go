package gol

import (
	// "fmt"
	// "strconv"

	"strconv"

	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
}

// create a blak 2D slice of size p.ImageHeight x p.ImageWidth
func initialiseNewWorld(p Params) [][]byte {
	world := make([][]byte, p.ImageHeight)
	for i := range(world){
		world[i] = make([]byte, p.ImageWidth)
	}
	return world
}

// count the number of neighbours that a particular cell has in the world
func getNeighbourCount(world [][]byte, row, column int, p Params) int {
	alive := 0
	offsets := []util.Cell{
		{X:-1,Y: -1},
		{X:-1,Y: 0},
		{X:-1,Y: 1},
		{X:0, Y:-1},
		{X:0, Y:1},
		{X:1, Y:-1},
		{X:1, Y:0},
		{X:1, Y:1},
	}
	for _,offset := range offsets {
		actualRow := (row + offset.X) % p.ImageHeight
		if actualRow < 0 {
			actualRow = p.ImageHeight - 1
		}
		actualCol := (column + offset.Y) % p.ImageWidth
		if actualCol < 0 {
			actualCol = p.ImageWidth - 1
		}
		if world[actualRow][actualCol] == 0xFF {
			alive++
		}
	}
	return alive
}

// complete 1 iteration of the world following Game of Life rules
func evolve(world [][]byte, p Params) [][]byte {
	newWorld := initialiseNewWorld(p);
	for i, row := range world {
		for j := range row {
			neighbours := getNeighbourCount(world, i, j, p)
			if neighbours < 2 || neighbours > 3{
				newWorld[i][j] = 0x00
			} else {
				if world[i][j] == 0x00 && neighbours == 3{
					newWorld[i][j] = 0xFF
					continue
				}
				newWorld[i][j] = world[i][j]
			}
		}
	}
	return newWorld
}

// get a list of the alive cells existing in the world
func getAliveCells(world [][]byte) []util.Cell {
	var aliveCells []util.Cell

	for i, row := range(world) {
		for j, cell := range(row) {
			if cell == 0xFF {
				aliveCells = append(aliveCells, util.Cell{X:j, Y:i})
			}
		}
	}
	return aliveCells
}


// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {

	// TODO: Give the filename to the io.channels.filename channel
	c.ioCommand <- ioInput
	// e.g., 64x64, 128x128 etc.
	c.ioFilename <- (strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight))

	// TODO: Create a 2D slice to store the world and populate it
	world := initialiseNewWorld(p)

	// TODO: Populate blank 2D slice with world data from input
	for i := 0; i < p.ImageHeight; i++{
		for j := 0; j < p.ImageWidth; j++ {
			world[i][j] = <-c.ioInput
		}
	}

	turn := 0
	// TODO: Execute all turns of the Game of Life.
	for ;turn < p.Turns; turn++  {
		world = evolve(world, p);
	}

	// Get a slice of the alive cells
	aliveCells := getAliveCells(world)

	// TODO: Report the final state using FinalTurnCompleteEvent.
	c.events <- FinalTurnComplete{CompletedTurns: p.Turns, Alive: aliveCells}

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}
	
	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}
