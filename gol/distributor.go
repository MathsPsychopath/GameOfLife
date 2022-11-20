package gol

import (
	"net/rpc"
	"strconv"
	"time"

	"uk.ac.bris.cs/gameoflife/stubs"
	util "uk.ac.bris.cs/gameoflife/util"
)

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

// execute RPC calls to poll the number of alive cells every 2 seconds
func aliveCellsTicker(client *rpc.Client, c distributorChannels, exit <-chan struct {}) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	request := stubs.AliveCellsRequest{}
	response := new(stubs.AliveCellsResponse)
	for {
		select {
		case <- exit:
			return
		case <- ticker.C:
			client.Call(stubs.GetAliveCells, request, response)
			c.events <- AliveCellsCount{CellsCount: response.Count, CompletedTurns: response.Turn}			
		}
	}
}

// sends the correct events + data in channels for pgm output
func outputPgm(c distributorChannels, p Params, world [][]byte, turn int) {
	c.ioCommand <- ioOutput
	c.ioFilename <- strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(turn)

	for _, row := range world {
		for _, cell := range row {
			c.ioOutput <- cell
		}
	}
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
			if world[i][j] == 0xFF {
				c.events <- CellFlipped{0, util.Cell{X: j, Y: i}}
			}
		}
	}
	// open RPC call to the AWS node. may need to hardcode IP address
	client, _ := rpc.Dial("tcp", "127.0.0.1:8030")
	defer client.Close()

	stubParams := stubs.StubParams{Turns: p.Turns, Threads: p.Threads, ImageWidth: p.ImageWidth, ImageHeight: p.ImageHeight }
	request := stubs.Request{World: world, P: stubParams}
	response := new(stubs.Response)

	// initialise ticker
	exit := make(chan struct {})
	defer close(exit)
	go aliveCellsTicker(client, c, exit)

	// execute rpc
	err := client.Call(stubs.Evolve, request, response)
	if err != nil {
		panic("an error happened during rpc call")
	}

	world = response.World
	// Get a slice of the alive cells
	aliveCells := getAliveCells(world)

	outputPgm(c, p, world, p.Turns)

	// TODO: Report the final state using FinalTurnCompleteEvent.
	c.events <- FinalTurnComplete{CompletedTurns: p.Turns, Alive: aliveCells}

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}
