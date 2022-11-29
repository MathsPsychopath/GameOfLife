package gol

import (
	"fmt"
	"net"
	"net/rpc"
	"sync"
	"time"

	"uk.ac.bris.cs/gameoflife/stubs"
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

var acknowledgedCells = stubs.NewCellsContainer()
var brokerAddr = "127.0.0.1:9000"
var listenAddr = ":8090"
var eventsSender Sender

// execute RPC calls to poll the number of alive cells every 2 seconds
func aliveCellsTicker(client *rpc.Client, c distributorChannels, exit <-chan bool) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <- exit:
			return
		case <- ticker.C:
			world, turn := acknowledgedCells.Get()
			eventsSender.SendAliveCellsList(turn, stubs.SquashSlice(world))
		}
	}
}

func kpListener(kp <-chan rune, client *rpc.Client, exit chan bool, c distributorChannels, p Params) {
	for {
		key := <-kp
		switch key {
		case 's':
			//output a pgm image
			world, turn := acknowledgedCells.Get()
			eventsSender.SendOutputPGM(world, turn)
		case 'q':
			//close the local controller
			_, turn := acknowledgedCells.Get()
			client.Call(stubs.ControllerQuit, stubs.NilRequest{}, new(stubs.NilResponse))
			eventsSender.SendStateChange(turn, Quitting)
			// make sure every goroutine dependent on exit is shutdown
			eventsSender.mu.Lock()
			for i := 0; i < 5; i++ {
				select {
				case exit <- true:
				default:
				}
			}
			eventsSender.mu.Unlock()
		case 'k':
			//kill the distributed system
			fmt.Println("sent kill call")
			err := client.Call(stubs.ServerQuit, stubs.NilRequest{}, new(stubs.NilResponse))
			if err != nil {
				fmt.Println(err)
			}
			world, turn := acknowledgedCells.Get()
			eventsSender.SendOutputPGM(world, turn)
			eventsSender.SendStateChange(turn, Quitting)
			// make sure every goroutine dependent on exit is shutdown
			for i := 0; i < 5; i++ {
				select {
				case exit <- true:
				default:
				}
			}
		case 'p':
			//pause/unpause the processing
			res := new(stubs.PauseResponse)
			err := client.Call(stubs.PauseState, stubs.NilRequest{}, res)
			if err != nil {
				fmt.Println(err)
			}
			_, turn := acknowledgedCells.Get()
			if res.Status == stubs.Paused {
				eventsSender.SendStateChange(turn, Paused)
			} else {
				eventsSender.SendStateChange(turn, Executing)
			}
		}
	}
}

// distributor distributes the work to the broker via rpc calls
func distributor(p Params, c distributorChannels,kp <-chan rune) {
	// provide global info for rpc call handlers to use
	eventsSender = Sender{C: c, P: p, mu: new(sync.Mutex)}

	// load the initial world
	cells := eventsSender.GetInitialAliveCells()
	eventsSender.SendFlippedCellList(0, cells...)
	eventsSender.SendTurnComplete(0)

	if p.Turns == 0 {
		eventsSender.SendOutputPGM(stubs.ConstructWorld(cells, p.ImageHeight, p.ImageWidth), 0)
		eventsSender.SendFinalTurn(0, cells)
		close(c.events)
		return
	}

	// store the initial world in memory
	acknowledgedCells.InitialiseWorld(
		stubs.ConstructWorld(cells, p.ImageHeight, p.ImageWidth),
	)
	
	// initialise exit
	exit := make(chan bool)
	defer func(){exit <- true}()
	
	// start listening for broker requests
	isListening := make(chan bool)
	go receiver(exit, isListening)
	<-isListening
	
	// dial the Broker. This is a hardcoded address
	client, err := rpc.Dial("tcp", brokerAddr)
	defer client.Close()
	if err != nil {
		fmt.Println("error when dialing broker")
		return
	}
	go aliveCellsTicker(client, c, exit)
	
	// connect to the broker
	connReq := stubs.ConnectRequest{IP: stubs.IPAddress("127.0.0.1" + listenAddr)}
	connErr := client.Call(stubs.ControllerConnect, connReq, new(stubs.NilResponse))
	if connErr != nil {
		fmt.Println(connErr)
	}

	fmt.Println("Successfully connected to broker")
	
	// start keypress listener
	go kpListener(kp, client, exit, c, p)
	
	// execute rpc
	stubParams := stubs.StubParams{Turns: p.Turns, Threads: p.Threads, ImageWidth: p.ImageWidth, ImageHeight: p.ImageHeight }
	request := stubs.StartGOLRequest{InitialAliveCells: cells, P: stubParams}
	done := make(chan struct{})
	go func () {
		err := client.Call(stubs.StartGOL, request, new(stubs.NilResponse))
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

	// exit the broker
	client.Call(stubs.ControllerQuit, stubs.NilRequest{}, new(stubs.NilResponse))

	// Get the final state of the world
	world, turn := acknowledgedCells.Get()
	
	// Output the final image
	eventsSender.SendOutputPGM(world, turn + 1)

	if p.ImageHeight == 16 {
		fmt.Println("Final World State after ", turn, " turns")
		util.VisualiseMatrix(world, 16, 16)
	}	

	// TODO: Report the final state using FinalTurnCompleteEvent.
	eventsSender.SendFinalTurn(turn + 1, stubs.SquashSlice(world))

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

type Controller struct {}

// This method will be called if the Broker has a calculated new state 
// for the user to view in SDL window
func (c *Controller) PushState(req stubs.PushStateRequest, res *stubs.NilResponse) (err error) {
	acknowledgedCells.UpdateWorld(req.FlippedCells, req.Turn)

	eventsSender.SendFlippedCellList(req.Turn, req.FlippedCells...)
	eventsSender.SendTurnComplete(req.Turn)
	return
}

func receiver(exit chan bool, listening chan<- bool) {
	fmt.Println("starting controller listening")
	rpc.Register(&Controller{})
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		fmt.Println("could not listen on port " + listenAddr)
		return
	}
	defer listener.Close()
	go rpc.Accept(listener)
	listening <- true
	<-exit
}
