package gol

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	util "uk.ac.bris.cs/gameoflife/util"
)

type HorSlice struct {
	grid     [][]byte
	startRow int
	endRow   int
}

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
}

// create a blank 2D slice of size p.ImageHeight x p.ImageWidth
func initialiseNewWorld(p Params) [][]byte {
	world := make([][]byte, p.ImageHeight)
	for i := range world {
		world[i] = make([]byte, p.ImageWidth)
	}
	return world
}

// parameterizable 2D slice creator (rows x columns)
func createNewSlice(rows, columns int) [][]byte {
	world := make([][]byte, rows)
	for i := range world {
		world[i] = make([]byte, columns)
	}
	return world
}

// count the number of neighbours that a particular cell has in the world
func getNeighbourCount(world [][]byte, row, column int, p Params) int {
	alive := 0
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
		actualRow := (row + offset.X) % p.ImageHeight
		if actualRow < 0 {
			actualRow = p.ImageHeight + actualRow
		}
		actualCol := (column + offset.Y) % p.ImageWidth
		if actualCol < 0 {
			actualCol = p.ImageWidth + actualCol
		}
		if world[actualRow][actualCol] == 0xFF {
			alive++
		}
	}
	return alive
}

// complete 1 iteration of the world following Game of Life rules
func evolve(world [][]byte, p Params) [][]byte {
	newWorld := initialiseNewWorld(p)
	for i, row := range world {
		for j := range row {
			neighbours := getNeighbourCount(world, i, j, p)
			if neighbours < 2 || neighbours > 3 {
				newWorld[i][j] = 0x00
			} else {
				if world[i][j] == 0x00 && neighbours == 3 {
					newWorld[i][j] = 0xFF
					continue
				}
				newWorld[i][j] = world[i][j]
			}
		}
	}
	return newWorld
}

func getNextCell(slice HorSlice, i, j, neighbourCount int) uint8 {
	if neighbourCount < 2 || neighbourCount > 3 {
		return 0x00
	} else if slice.grid[i][j] == 0x00 && neighbourCount == 3 {
		return 0xFF
	} else {
		return slice.grid[i][j]
	}
}

// parameterizable evolve slice.grid
func evolveSlice(slice HorSlice, p Params) [][]byte {
	// create empty slice
	newSlice := createNewSlice(slice.endRow-slice.startRow, p.ImageWidth)
	// iterate through cells of slice of the oldGrid
	for i := slice.startRow; i < slice.endRow; i++ {
		for j := 0; j < p.ImageWidth; j++ {
			neighbourCount := getNeighbourCount(slice.grid, i, j, p)
			// get new value for cell and append to newSlice
			newSlice[i-slice.startRow][j] = getNextCell(slice, i, j, neighbourCount)
		}
	}
	return newSlice
}

// create a worker assigned to a segment of the image
func worker(slice HorSlice, p Params, output chan HorSlice) {
	newSlice := evolveSlice(slice, p)
	output <- HorSlice{newSlice, slice.startRow, slice.endRow}
}

// get a list of the alive cells existing in the world
func getAliveCells(world [][]byte) []util.Cell {
	var aliveCells []util.Cell

	for i, row := range world {
		for j, cell := range row {
			if cell == 0xFF {
				aliveCells = append(aliveCells, util.Cell{X: j, Y: i})
			}
		}
	}
	return aliveCells
}

// get the number of alive cells
func getAliveCellsCount(world [][]byte) int {
	count := 0
	for _, row := range world {
		for _, cell := range row {
			if cell == 0xFF {
				count++
			}
		}
	}
	return count
}

func reportCellCount(input <-chan [][]byte, quit <-chan bool,
	events chan<- Event, completedTurns <-chan int) {
	ticker := time.NewTicker(2 * time.Second)

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

func generatePGM(p Params, c distributorChannels, world [][]byte, turns int) {
	c.ioCommand <- ioOutput
	c.ioFilename <- (strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(turns))

	for _, row := range world {
		for _, cell := range row {
			c.ioOutput <- cell
		}
	}
}

func initialiseWorker(world [][]byte, outputChannel chan HorSlice, p Params, i int) {
	// i is current thread
	segmentSize := p.ImageHeight / p.Threads
	startRow := i * segmentSize
	var endRow int
	if i == p.Threads-1 {
		endRow = p.ImageHeight
	} else {
		endRow = (i + 1) * segmentSize
	}
	slice := HorSlice{world, startRow, endRow}
	go worker(slice, p, outputChannel)
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {

	// TODO: Give the filename to the io.channels.filename channel
	c.ioCommand <- ioInput
	// e.g., 64x64, 128x128 etc.
	c.ioFilename <- (strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight))

	// TODO: initialise the world
	world := createNewSlice(p.ImageHeight, p.ImageWidth)

	// TODO: Populate blank world with world data from input
	for i := 0; i < p.ImageHeight; i++ {
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
	// creating channels
	workerOutputChannel := make(chan HorSlice, p.Threads)
	var waitgroup sync.WaitGroup
	for ; turn < p.Turns; turn++ {
		newWorld := make([][]byte, p.ImageHeight)
		for i := 0; i < p.ImageHeight; i++ {
			newWorld[i] = make([]byte, p.ImageWidth)
		}

		// non-blocking send
		// update turn and world data for ticker
		select {
		case worldChannel <- world:
			turnChannel <- turn
		default:
		}
		// could the above be changed to an if statement?
		// Initialise the worker threads
		for tr := 0; tr < p.Threads; tr++ {
			initialiseWorker(world, workerOutputChannel, p, tr)
		}
		// goes through each thread one by one, waiting to receive a slice of the new world.
		for tr := 0; tr < p.Threads; tr++ {
			// TODO: improve by appending worker outputs in any order
			// this blocks until next thread is finished
			newSlice := <-workerOutputChannel
			// fmt.Printf("\nchan len: %d\n", len(workerOutputChannel))
			waitgroup.Add(1)
			go func() {
				defer waitgroup.Done()
				for i := newSlice.startRow; i < newSlice.endRow; i++ {
					for j := 0; j < p.ImageHeight; j++ {
						newWorld[i][j] = newSlice.grid[i-newSlice.startRow][j]
					}
				}
			}()
		}
		waitgroup.Wait()
		c.events <- TurnComplete{CompletedTurns: turn}
		// updates world
		// fmt.Print("old:\n")
		// util.VisualiseSquare(world, p.ImageWidth, p.ImageHeight)
		// fmt.Print("new:\n")
		// util.VisualiseSquare(newWorld, p.ImageWidth, p.ImageHeight)

		// time.Sleep(500 * time.Millisecond)
		world = newWorld
		assertChannelEmpty(workerOutputChannel, p, turn)
	}
	close(workerOutputChannel)
	// Generate a PGM image at turn 100
	generatePGM(p, c, world, turn)

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

//asserts
func assertChannelEmpty(channel chan HorSlice, p Params, turn int) {
	if len(channel) != 0 {
		fmt.Printf("\n\n\nchannel not empty!!!!\n\n\n")
		fmt.Printf("channel length: %d\n", len(channel))
		temp := <-channel
		fmt.Printf("\n\n\nchannel item start row %d end row %d \n\n\n", temp.startRow, temp.endRow)
		fmt.Printf("turns total: %d\n\n", p.Turns)
		fmt.Printf("current turn: %d\n\n", turn)
		os.Exit(1)
	}
}
