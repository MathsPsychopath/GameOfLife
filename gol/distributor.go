package gol

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	util "uk.ac.bris.cs/gameoflife/util"
)

// type to reduce # of individual channels
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

// apply GOL rules and return the result for given cell
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
func evolveSlice(slice HorSlice, p Params, c distributorChannels, turn int) [][]byte {
	// create empty slice
	newSlice := createNewSlice(slice.endRow-slice.startRow, p.ImageWidth)
	// iterate through cells of slice of the oldGrid
	for i := slice.startRow; i < slice.endRow; i++ {
		for j := 0; j < p.ImageWidth; j++ {
			neighbourCount := getNeighbourCount(slice.grid, i, j, p)
			// get new value for cell and append to newSlice
			updatedCell := getNextCell(slice, i, j, neighbourCount)
			newSlice[i-slice.startRow][j] = updatedCell
			// cellFlipped event
			if updatedCell != slice.grid[i][j] {
				c.events <- CellFlipped{turn, util.Cell{j, i}}
			}
		}
	}
	return newSlice
}

// create a worker assigned to a segment of the image
func worker(slice HorSlice, p Params, output chan HorSlice, c distributorChannels, turn int) {
	newSlice := evolveSlice(slice, p, c, turn)
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

// generate PGM file using ioCommand
func generatePGM(p Params, c distributorChannels, world [][]byte, turns int) {
	c.ioCommand <- ioOutput
	c.ioFilename <- (strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(turns))

	for _, row := range world {
		for _, cell := range row {
			c.ioOutput <- cell
		}
	}
}

// start worker threads to work on specific sections given params
func initialiseWorker(world [][]byte, outputChannel chan HorSlice, p Params, i int, c distributorChannels, turn int) {
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
	go worker(slice, p, outputChannel, c, turn)
}

// send the AliveCellsCount event
func checkTicker(ticker *time.Ticker, world [][]byte, turn int, c distributorChannels) {
	select {
	case <-ticker.C:
		alive := getAliveCellsCount(world)
		c.events <- AliveCellsCount{CellsCount: alive, CompletedTurns: turn}
	default:
	}

}

// parse keypresses and execute the different actions
func keypressParser(p Params, c distributorChannels, kp <-chan rune, turn <-chan int, worldState <-chan HorSlice, wg *sync.WaitGroup) {
	paused := false
	for {
		key := <-kp
		switch key {
		case 's':
			// generate PGM image of current state
			generatePGM(p, c, (<-worldState).grid, <-turn)
		case 'q':
			// generate PGM image and terminate
			generatePGM(p, c, (<-worldState).grid, <-turn)
			c.ioCommand <- ioCheckIdle
			<-c.ioIdle
			c.events <- StateChange{CompletedTurns: <-turn, NewState: Quitting}
			close(c.events)
		case 'p':
			// pause execution. If already paused, continue
			if paused {
				paused = false
				wg.Done()
				fmt.Println("Continuing")
				c.events <- StateChange{CompletedTurns: <-turn, NewState: Executing}
			} else {
				paused = true
				wg.Add(1)
				c.events <- StateChange{CompletedTurns: <-turn, NewState: Paused}
			}
		}
	}
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels, kp <-chan rune) {

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
			if world[i][j] == 0xFF {
				c.events <- CellFlipped{0, util.Cell{X: j, Y: i}}
			}
		}
	}

	// start ticker to indicate alive cells
	ticker := time.NewTicker(2 * time.Second)

	// start keypress parser
	turnSender := make(chan int)
	kpStateUpdates := make(chan HorSlice)
	var golLoop sync.WaitGroup
	go keypressParser(p, c, kp, turnSender, kpStateUpdates, &golLoop)

	// TODO: Execute all turns of the Game of Life.
	turn := 0
	// creating channels
	workerOutputChannel := make(chan HorSlice, p.Threads)
	var waitgroup sync.WaitGroup
	for ; turn < p.Turns; turn++ {
		golLoop.Wait()
		newWorld := createNewSlice(p.ImageHeight, p.ImageWidth)

		// Initialise the worker threads
		for tr := 0; tr < p.Threads; tr++ {
			initialiseWorker(world, workerOutputChannel, p, tr, c, turn)
		}
		for tr := 0; tr < p.Threads; tr++ {
			newSlice := <-workerOutputChannel
			waitgroup.Add(1)
			go func() {
				for i := newSlice.startRow; i < newSlice.endRow; i++ {
					for j := 0; j < p.ImageHeight; j++ {
						newWorld[i][j] = newSlice.grid[i-newSlice.startRow][j]
					}
				}
				waitgroup.Done()
			}()
		}
		waitgroup.Wait()
		// updates world
		world = newWorld
		c.events <- TurnComplete{turn}

		// checking if ticker has ticked
		checkTicker(ticker, world, turn+1, c)
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
	ticker.Stop()
}
