package gol

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"uk.ac.bris.cs/gameoflife/channels"
	util "uk.ac.bris.cs/gameoflife/util"
)

// type to reduce # of individual channels
type HorSlice struct {
	grid     [][]byte
	startRow int
	endRow   int
}

// declare channel for HSlice on only used in this package

type HSliceChannel struct {
	value    *HorSlice
	cond     *sync.Cond
}

func NewHSliceChannel() *HSliceChannel {
	m := new(sync.Mutex)
	return &HSliceChannel{value: nil, cond: sync.NewCond(m)}
}

func (c *HSliceChannel) Send(value HorSlice, block bool) {
	c.cond.L.Lock()
	for c.value != nil {
		if !block {
			c.cond.L.Unlock()
			return
		}
		c.cond.Wait()
	}
	c.value = &value
	c.cond.Broadcast()
	c.cond.L.Unlock()
}

func (c *HSliceChannel) Receive() (v HorSlice) {
	c.cond.L.Lock()
	for c.value == nil {
		c.cond.Wait()
	}
	v, c.value = *c.value, nil
	c.cond.Broadcast()
	c.cond.L.Unlock()
	return v
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
		actualRow := (row + offset.X) & (p.ImageHeight - 1)
		actualCol := (column + offset.Y) & (p.ImageWidth - 1)
		alive += (world[actualRow][actualCol] >> 7)
	}
	return int(alive)
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
				c.events <- CellFlipped{turn, util.Cell{X: j, Y: i}}
			}
		}
	}
	return newSlice
}

// create a worker assigned to a segment of the image
func worker(slice HorSlice, p Params, output *HSliceChannel, c distributorChannels, turn int) {
	newSlice := evolveSlice(slice, p, c, turn)
	output.Send(HorSlice{newSlice, slice.startRow, slice.endRow}, true)
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
func keypressParser(p Params, c distributorChannels, kp <-chan rune, turnChan *channels.IntChannel, worldChan *HSliceChannel, wg *sync.WaitGroup, quit *channels.BoolChannel) {
	paused := false
	for {
		key := <-kp
		switch key {
		case 's':
			if paused {
				continue
			}
			// generate PGM image of current state
			turn, _ := turnChan.Receive(true)
			worldState := worldChan.Receive()
			generatePGM(p, c, worldState.grid, turn)
		case 'q':
			// generate PGM image and terminate
			if paused {
				continue
			}
			turn, _ := turnChan.Receive(true)
			worldState := worldChan.Receive()
			generatePGM(p, c, worldState.grid, turn)
			c.ioCommand <- ioCheckIdle
			<-c.ioIdle
			turn, _ = turnChan.Receive(true)
			c.events <- StateChange{CompletedTurns: turn, NewState: Quitting}
			quit.Send(true, true)
			return
		case 'p':
			// pause execution. If already paused, continue
			if paused {
				paused = false
				wg.Done()
				fmt.Println("Continuing")
				turn, _ := turnChan.Receive(true)
				c.events <- StateChange{CompletedTurns: turn, NewState: Executing}
			} else {
				paused = true
				wg.Add(1)
				turn, _ := turnChan.Receive(true)
				c.events <- StateChange{CompletedTurns: turn, NewState: Paused}
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
	turnSender := channels.NewIntChannel()
	kpStateUpdates := NewHSliceChannel()
	quit := channels.NewBoolChannel()
	var golLoop sync.WaitGroup
	go keypressParser(p, c, kp, turnSender, kpStateUpdates, &golLoop, quit)

	// TODO: Execute all turns of the Game of Life.
	turn := 0
	// creating channels
	workerOutputChannel := NewHSliceChannel()
	var waitgroup sync.WaitGroup
	run := true

	// pre-calculate work distribution
	segment := p.ImageHeight / p.Threads
	remainder := p.ImageHeight - (segment * p.Threads)
	workSizes := []HorSlice{}
	workerStartRow := 0
	for i := 0; i < p.Threads; i++ {
		work := HorSlice{grid: nil, startRow: workerStartRow, endRow: workerStartRow + segment}
		if remainder > 0 {
			work.endRow += 1
			remainder--
		}
		workSizes = append(workSizes, work)
		workerStartRow = work.endRow
	}

	for ; turn < p.Turns && run; turn++ {
		turnSender.Send(turn, false)
		kpStateUpdates.Send(HorSlice{grid: world, startRow: 0, endRow: 0}, false)
		_, success := quit.Receive(false)
		if success {
			run = false
		}
		golLoop.Wait()
		newWorld := createNewSlice(p.ImageHeight, p.ImageWidth)

		// Initialise the worker threads
		for tr := 0; tr < p.Threads; tr++ {
			slice := workSizes[tr]
			slice.grid = world
			go worker(slice, p, workerOutputChannel, c, turn)
		}
		for tr := 0; tr < p.Threads; tr++ {
			newSlice := workerOutputChannel.Receive()
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
