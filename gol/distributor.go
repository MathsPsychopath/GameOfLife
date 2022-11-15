package gol

import (
	// "fmt"
	// "strconv"

	"strconv"
	"time"

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

// parameterizable 2D slice creator (rows x columns)
func createNewSlice(rows, columns int) [][]byte {
	world := make([][]byte, rows)
	for i := range(world){
		world[i] = make([]byte, columns)
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

// parameterizable evolve world
func evolveParameterizable(world [][]byte, startRow, endRow int, p Params) [][]byte {
	newPartition := createNewSlice(endRow - startRow, p.ImageWidth)
	for i, k := startRow, 0; i < endRow; i, k = i+1, k+1 {
		for j := 0; j < p.ImageWidth; j++ {
			neighbours := getNeighbourCount(world, i, j, p)
			if neighbours < 2 || neighbours > 3{
				newPartition[k][j] = 0x00 //startRow is 8, but the new slice is relative to 0
			} else {
				if world[i][j] == 0x00 && neighbours == 3{
					newPartition[k][j] = 0xFF
					continue
				}
				newPartition[k][j] = world[i][j]
			}
		}
	}
	return newPartition
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

// get the number of alive cells
func getAliveCellsCount(world [][]byte) int {
	count := 0
	for _, row := range(world) {
		for _, cell := range(row) {
			if cell == 0xFF {
				count++
			}
		}
	}
	return count
}

// create a worker assigned to a segment of the image
func worker(startY, endY int, p Params,
	input <-chan [][]byte, output chan [][]byte) {
	for i := 0; i < p.Turns; i++ {
		oldWorld := <- input
		newWorld := evolveParameterizable(oldWorld, startY, endY, p)
		output <- newWorld
	}
}

func reportCellCount(input <-chan [][]byte, quit <-chan bool, 
	events chan<- Event, completedTurns <-chan int){
	ticker:= time.NewTicker(2*time.Second)

	for {
		select {
		case <-ticker.C:
			world := <-input
			turn := <-completedTurns
			alive := getAliveCellsCount(world)
			events <- AliveCellsCount{CellsCount: alive, CompletedTurns: turn}
		case <-quit:
			ticker.Stop()
			return
		}
	}
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {

	// TODO: Give the filename to the io.channels.filename channel
	c.ioCommand <- ioInput
	// e.g., 64x64, 128x128 etc.
	c.ioFilename <- (strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight))

	// define the worker arguments
	segments := p.ImageHeight / p.Threads
	workerInputs := []chan [][]byte {}
	workerOutputs := []chan [][]byte {}
	
	// Initialise the worker threads
	for i := 0; i < p.Threads; i++ {
		input := make(chan [][]byte)
		output := make(chan [][]byte)
		workerInputs = append(workerInputs, input)
		workerOutputs = append(workerOutputs, output)
		if i == p.Threads - 1 {
			go worker(i*segments, p.ImageHeight, p, input, output)
		} else {
			go worker(i * segments, (i+1) * segments, p, input, output)
		}

	}
	
	
	// TODO: initialise the 2D slice
	world := createNewSlice(p.ImageHeight, p.ImageWidth)

	// TODO: Populate blank 2D slice with world data from input
	for i := 0; i < p.ImageHeight; i++{
		for j := 0; j < p.ImageWidth; j++ {
			world[i][j] = <-c.ioInput
		}
	}

	// start ticker to indicate alive cells
	worldChannel := make(chan [][]byte)
	turnChannel := make(chan int)
	quit := make(chan bool)
	go reportCellCount(worldChannel, quit, c.events, turnChannel)

	// TODO: Execute all turns of the Game of Life.
	turn := 0
	for ;turn < p.Turns; turn++  {
		newWorld := [][]byte {}
		// non-blocking send
		select {
		case worldChannel <- world:
			turnChannel <- turn
		default:
		}
		for i := 0; i < p.Threads; i++ {
			workerInputs[i] <- world
			newWorld = append(newWorld, <-workerOutputs[i]...)
		}
		c.events <- TurnComplete{CompletedTurns: turn}
		world = newWorld
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
