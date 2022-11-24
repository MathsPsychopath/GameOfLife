package gol

import (
	"fmt"
	"net"
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

var acknowledgedCells = stubs.New(nil, 0)
var exit = make(chan struct {})
var brokerAddr = "127.0.0.1:9000"
var listenAddr = ":9010"
var eventsSender Sender

// parameterizable 2D slice creator (rows x columns)
func createNewSlice(rows, columns int) [][]byte {
	world := make([][]byte, rows)
	for i := range world {
		world[i] = make([]byte, columns)
	}
	return world
}

// convert []util.Cell to a 2D slice
func populateWorld(cells []util.Cell, p Params) [][]byte {
	world := createNewSlice(p.ImageHeight, p.ImageWidth)
	for _, aliveCell := range cells {
		world[aliveCell.Y][aliveCell.X] = 0xFF
	}
	return world
}

// execute RPC calls to poll the number of alive cells every 2 seconds
func aliveCellsTicker(client *rpc.Client, c distributorChannels, exit <-chan struct {}) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	res := new(stubs.PushStateBody)
	for {
		select {
		case <- exit:
			return
		case <- ticker.C:
			client.Call(stubs.GetAlive, stubs.NilRequest{}, res)
			eventsSender.SendAliveCellsList(res.Turn, res.Cells)
		}
	}
}

// sends the correct events + data in channels for pgm output
func outputPgm(c distributorChannels, p Params, cells []util.Cell, turn int) {
	c.ioCommand <- ioOutput
	c.ioFilename <- (strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(turn))

	world := populateWorld(cells, p)	

	for _, row := range world {
		for _, cell := range row {
			c.ioOutput <- cell
		}
	}
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle
}

func kpListener(kp <-chan rune, client *rpc.Client, exit chan struct {}, c distributorChannels, p Params) {
	for {
		key := <-kp
		switch key {
		case 's':
			//output a pgm image
			res := new(stubs.PushStateBody)
			client.Call(stubs.SaveState, stubs.NilRequest{}, res)
			fmt.Println("sent Save call")
			outputPgm(c, p, res.Cells, res.Turn)
		case 'q':
			//close the local controller
			res := new(stubs.StatusResponse)
			err := client.Call(stubs.ConQuit, stubs.NilRequest{}, res)
			if err != nil {
				fmt.Println(err)
			}

			eventsSender.SendStateChange(res.Turn, Quitting)
			close(exit)
		case 'k':
			//kill the distributed system
			res := new(stubs.PushStateBody)
			fmt.Println("sent kill call")
			err := client.Call(stubs.SerQuit, stubs.NilRequest{}, res)
			if err != nil {
				fmt.Println(err)
			}
			outputPgm(c, p, res.Cells, res.Turn)
			
			eventsSender.SendStateChange(res.Turn, Quitting)
			close(exit)
		case 'p':
			//pause/unpause the processing
			res := new(stubs.StatusResponse)
			err := client.Call(stubs.PauseState, stubs.NilRequest{}, res)
			if err != nil {
				fmt.Println(err)
			}
			if res.Status == stubs.Paused {
				eventsSender.SendStateChange(res.Turn, Paused)
			} else {
				eventsSender.SendStateChange(res.Turn, Executing)
			}
		}
	}
}
// TODO: test with 1 worker
// distributor distributes the work to the broker via rpc calls
func distributor(p Params, c distributorChannels,kp <-chan rune) {
	// provide global info for rpc call handlers to use
	eventsSender = Sender{Events: c.events, P: p}

	// TODO: Give the filename to the io.channels.filename channel
	c.ioCommand <- ioInput
	// e.g., 64x64, 128x128 etc.
	c.ioFilename <- (strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight))

	cellsList := make([]util.Cell, 10000)
	// TODO: Populate blank world with world data from input
	for i := 0; i < p.ImageHeight; i++ {
		for j := 0; j < p.ImageWidth; j++ {
			if <-c.ioInput == 0xFF {
				Cell := util.Cell{X:j, Y:i}
				cellsList = append(cellsList, Cell)
			}
		}
	}
	eventsSender.SendFlippedCellList(0, cellsList...)
	eventsSender.SendTurnComplete(0)
	acknowledgedCells.Update(cellsList, 0)

	// dial the Broker. This is a hardcoded address
	client, err := rpc.Dial("tcp", brokerAddr)
	if err != nil {
		fmt.Println("error when dialing broker")
	}
	defer client.Close()

	
	// initialise ticker
	exit := make(chan struct {})
	defer close(exit)
	// go aliveCellsTicker(client, c, exit)

	// start broker receiver
	go receiver(exit)
	
	// connect to the broker
	connReq := stubs.ConnectRequest{IP: stubs.IPAddress("127.0.0.1" + listenAddr)}
	connRes := new(stubs.ConnectResponse)
	connErr := client.Call(stubs.ConConnect, connReq, connRes)
	if connErr != nil {
		fmt.Println(connErr)
	}
	
	// start keypress listener
	go kpListener(kp, client, exit, c, p)
	
	// execute rpc
	cells, _ := acknowledgedCells.Get()
	stubParams := stubs.StubParams{Turns: p.Turns, Threads: p.Threads, ImageWidth: p.ImageWidth, ImageHeight: p.ImageHeight }
	request := stubs.StartGOLRequest{Cells: cells, P: stubParams}
	response := new(stubs.StatusResponse)
	done := make(chan struct{})
	go func () {
		err := client.Call(stubs.StartGOL, request, response)
		if err != nil {
			fmt.Println(err)
		}
		close(done)
	}()
		
	// either the call finishes executing GOL, or exit closes first
	// if exit closes first, then terminate main
	// if call finishes executing, then do the image outputting
	select {
	case <-exit:
		close(c.events)
		return
	case <-done:
	}

	// Get a slice of the alive cells
	aliveCells, _ := acknowledgedCells.Get()

	outputPgm(c, p, aliveCells, p.Turns)

	// TODO: Report the final state using FinalTurnCompleteEvent.
	eventsSender.SendFinalTurn(p.Turns, aliveCells)

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

type Controller struct {}

func (c *Controller) PushState(req stubs.PushStateBody, res *stubs.StatusResponse) (err error) {
	acknowledgedCells.Update(req.Cells, req.Turn)
	eventsSender.SendFlippedCellList(req.Turn, req.Cells...)
	res.Status = stubs.Running
	return
}

func receiver(exit chan struct{}) {
	rpc.Register(&Controller{})
	listener, _ := net.Listen("tcp", listenAddr)
	defer listener.Close()
	go rpc.Accept(listener)
	<-exit
}
