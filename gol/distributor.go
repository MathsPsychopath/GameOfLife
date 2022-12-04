package gol

import (
	"fmt"
	"net"
	"net/rpc"
	"strconv"
	"time"

	"uk.ac.bris.cs/gameoflife/stubs"
)

const (
	aliveCellsTickerFunc = iota
	kpListenerFunc
	distributorFunc
	pushState
	receiver
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
}

type Controller struct {
	acknowledgedCells stubs.CellsContainer
	eventsSender      Sender
	exitChannels      map[int]chan bool
	d                 distributorChannels
}

var contr Controller

// execute RPC calls to poll the number of alive cells every 2 seconds
func (c *Controller) aliveCellsTicker(client *rpc.Client) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	select {
	case <-ticker.C:
		aliveCellsCount, turn := c.acknowledgedCells.GetAliveCount()
		c.eventsSender.SendAliveCellsList(turn, aliveCellsCount)
	case <-c.exitChannels[aliveCellsTickerFunc]:
		break
	}
}

func (c *Controller) kpListener(kp <-chan rune, client *rpc.Client, p Params) {
	for {
		key := <-kp
		switch key {
		case 's':
			//output a pgm image
			world, turn := c.acknowledgedCells.Get()
			c.eventsSender.SendOutputPGM(world, turn)
		case 'q':
			//close the local controller
			_, turn := c.acknowledgedCells.Get()
			client.Call(stubs.ControllerQuit, stubs.NilRequest{}, new(stubs.NilResponse))
			c.eventsSender.SendStateChange(turn, Quitting)
			// make sure every goroutine dependent on exit is shutdown
			c.sendExitSignals()

		case 'k':
			//kill the distributed system
			fmt.Println("sent kill call")
			err := client.Call(stubs.ServerQuit, stubs.NilRequest{}, new(stubs.NilResponse))
			if err != nil {
				fmt.Println(err)
			}
			world, turn := c.acknowledgedCells.Get()
			c.eventsSender.SendOutputPGM(world, turn)
			c.eventsSender.SendStateChange(turn, Quitting)
			// make sure every goroutine dependent on exit is shutdown
			c.sendExitSignals()

		case 'p':
			//pause/unpause the processing
			res := new(stubs.PauseResponse)
			err := client.Call(stubs.PauseState, stubs.NilRequest{}, res)
			if err != nil {
				fmt.Println(err)
			}
			_, turn := c.acknowledgedCells.Get()
			if res.Status == stubs.Paused {
				c.eventsSender.SendStateChange(turn, Paused)
			} else {
				c.eventsSender.SendStateChange(turn, Executing)
			}
		}
	}
}

func newController() Controller {
	exitChannels := make(map[int]chan bool)
	return Controller{
		acknowledgedCells: *stubs.NewCellsContainer(),
		exitChannels:      exitChannels,
	}
}

func (c *Controller) sendExitSignals() {
	for _, channel := range c.exitChannels {
		channel <- true
	}
}

// distributor distributes the work to the broker via rpc calls
func distributor(p Params, d distributorChannels, kp <-chan rune) {
	contr = newController()
	rpc.Register(&contr)

	// provide global info for rpc call handlers to use
	contr.eventsSender = Sender{C: d, P: p}

	// load the initial world
	cells := contr.eventsSender.GetInitialAliveCells()
	contr.eventsSender.SendFlippedCellList(0, cells...)

	if p.Turns == 0 {
		contr.eventsSender.SendOutputPGM(stubs.ConstructWorld(cells, p.ImageHeight, p.ImageWidth), 0)
		contr.eventsSender.SendFinalTurn(0, cells)
		close(d.events)
		return
	}
	contr.eventsSender.SendTurnComplete(0)

	// store the initial world in memory
	contr.acknowledgedCells.UpdateWorld(
		stubs.ConstructWorld(cells, p.ImageHeight, p.ImageWidth),
	)

	// initialise exit

	// start listening for broker requests
	isListening := make(chan bool)
	go contr.receiver(isListening, p)
	<-isListening

	// dial the Broker.
	client, err := rpc.Dial("tcp", p.BrokerAddr)
	defer client.Close()
	if err != nil {
		fmt.Println("error when dialing broker")
		fmt.Printf("error message: %s\n", err)
		fmt.Printf("broker address: %s\n", p.BrokerAddr)
		return
	}
	go contr.aliveCellsTicker(client)

	// connect to the broker
	listenSocket := p.ListenIP + ":" + strconv.Itoa(p.ListenPort)
	connReq := stubs.ConnectRequest{IP: listenSocket}
	connErr := client.Call(stubs.ControllerConnect, connReq, new(stubs.NilResponse))
	if connErr != nil {
		fmt.Println(connErr)
		return
	}

	fmt.Println("Successfully connected to broker")

	// start keypress listener
	go contr.kpListener(kp, client, p)

	// execute rpc
	stubParams := stubs.StubParams{Turns: p.Turns, Threads: p.Threads, ImageWidth: p.ImageWidth, ImageHeight: p.ImageHeight}
	request := stubs.StartGOLRequest{InitialAliveCells: cells, P: stubParams}
	done := make(chan *rpc.Call, 1)
	client.Go(stubs.StartGOL, request, new(stubs.NilResponse), done)

	// either the call finishes executing GOL, or exit closes first
	// if exit closes first, then terminate main
	// if call finishes executing, then do the image outputting
	select {
	case <-done:
		fmt.Println("received done")
	case <-contr.exitChannels[distributorFunc]:
		close(d.events)
		return
	}

	close(done)
	// exit the broker
	client.Call(stubs.ControllerQuit, stubs.NilRequest{}, new(stubs.NilResponse))
	client.Close()
	// Get the final state of the world
	world, turn := contr.acknowledgedCells.Get()

	// Output the final image
	contr.eventsSender.SendOutputPGM(world, turn)

	// TODO: Report the final state using FinalTurnCompleteEvent.
	contr.eventsSender.SendFinalTurn(turn+1, stubs.GetAliveCells(world))

	// Make sure that the Io has finished any output before exiting.
	d.ioCommand <- ioCheckIdle
	<-d.ioIdle

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(d.events)
	contr.sendExitSignals()
}

// This method will be called if the Broker has a calculated new state
// for the user to view in SDL window
func (c *Controller) PushState(req stubs.PushStateRequest, res *stubs.NilResponse) (err error) {
	select {
	case <-c.exitChannels[pushState]:
		return
	default:

	}
	// util.VisualiseSquare(c.acknowledgedCells.CurrentWorld, len(c.acknowledgedCells.CurrentWorld), len(c.acknowledgedCells.CurrentWorld))
	c.acknowledgedCells.UpdateWorldAndTurn(req.FlippedCells, req.Turn)
	// util.VisualiseSquare(c.acknowledgedCells.CurrentWorld, len(c.acknowledgedCells.CurrentWorld), len(c.acknowledgedCells.CurrentWorld))
	// c.eventsSender.SendOutputPGM(c.acknowledgedCells.CurrentWorld, req.Turn) //DEBUG
	c.eventsSender.SendFlippedCellList(req.Turn, req.FlippedCells...)
	c.eventsSender.SendTurnComplete(req.Turn)
	return
}

func (c *Controller) receiver(listening chan<- bool, p Params) {
	fmt.Println("starting controller listening")
	listener, err := net.Listen("tcp", ":"+strconv.Itoa(p.ListenPort))
	if err != nil {
		fmt.Println("could not listen on port " + strconv.Itoa(p.ListenPort))
		return
	}
	defer listener.Close()
	go rpc.Accept(listener)
	listening <- true
	select {
	case <-c.exitChannels[receiver]:
	}
}
