package gol

import (
	"fmt"
	"net"
	"net/rpc"
	"strconv"
	"sync"
	"time"

	"uk.ac.bris.cs/gameoflife/stubs"
)

const (
	aliveCellsTickerFunc = iota
	receiver
	distributorFunc
)

const enumSize = 3

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
}

type Controller struct {
	m                 sync.Mutex
	acknowledgedCells stubs.CellsContainer
	eventsSender      Sender
	exitChannels      map[int]chan bool
}

var controller Controller

// execute RPC calls to poll the number of alive cells every 2 seconds
func (c *Controller) aliveCellsTicker(client *rpc.Client) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			aliveCellsCount, turn := c.acknowledgedCells.GetAliveCount()
			c.eventsSender.SendAliveCellsList(turn, aliveCellsCount)
		case <-c.exitChannels[aliveCellsTickerFunc]:
			break
		}
	}

}

func (c *Controller) kpListener(kp <-chan rune, client *rpc.Client) {
	for key := range kp {
		switch key {
		case 's':
			//output a pgm image
			world, turn := c.acknowledgedCells.Get()
			c.eventsSender.SendOutputPGM(world, turn)
		case 'q':
			//close the local controller
			// _, turn := c.acknowledgedCells.Get()
			// client.Call(stubs.ControllerQuit, stubs.NilRequest{}, new(stubs.NilResponse))
			// c.eventsSender.SendStateChange(turn, Quitting)
			// make sure every goroutine dependent on exit is shutdown
			c.exitChannels[distributorFunc] <- true

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
			c.exitChannels[distributorFunc] <- true

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
	for i := 0; i < enumSize; i++ {
		exitChannels[i] = make(chan bool)
	}
	return Controller{
		acknowledgedCells: *stubs.NewCellsContainer(),
		exitChannels:      exitChannels,
		m:                 *new(sync.Mutex),
	}
}

func (c *Controller) sendExitSignals() {
	c.m.Lock()
	go func() {
		c.exitChannels[aliveCellsTickerFunc] <- true
	}()
	c.exitChannels[receiver] <- true

	c.m.Unlock()
}

// distributor distributes the work to the broker via rpc calls
func distributor(p Params, d distributorChannels, kp <-chan rune) {
	controller = newController()
	rpc.Register(&controller)

	// provide global info for rpc call handlers to use
	controller.eventsSender = Sender{C: d, P: p}

	// load the initial world
	cells := controller.eventsSender.GetInitialAliveCells()
	controller.eventsSender.SendFlippedCellList(0, cells...)

	controller.eventsSender.SendTurnComplete(0)

	// store the initial world in memory
	controller.acknowledgedCells.UpdateWorld(
		stubs.ConstructWorld(cells, p.ImageHeight, p.ImageWidth),
	)
	if p.Turns == 0 {
		controller.shutDownSequence()
		return
	}

	// initialise exit

	// start listening for broker requests
	isListening := make(chan bool)
	go controller.receiver(isListening, p)
	<-isListening

	// dial the Broker.
	client, err := rpc.Dial("tcp", p.BrokerAddr)
	if err != nil {
		fmt.Println("error when dialing broker")
		fmt.Printf("error message: %s\n", err)
		fmt.Printf("broker address: %s\n", p.BrokerAddr)
		controller.shutDownSequence()
		return
	}
	go controller.aliveCellsTicker(client)

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
	go controller.kpListener(kp, client)

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
	case <-controller.exitChannels[distributorFunc]:

	}
	// exit the broker
	fmt.Println("0")
	client.Call(stubs.ControllerQuit, stubs.NilRequest{}, new(stubs.NilResponse))
	fmt.Println("1")
	client.Close()

	fmt.Println("2")
	close(done)

	fmt.Println("3")
	controller.sendExitSignals()
	fmt.Println("4")
	controller.shutDownSequence()
	fmt.Printf("after\n")
}

func (c *Controller) shutDownSequence() {
	// Get the final state of the world
	fmt.Println("5")
	world, turn := controller.acknowledgedCells.Get()

	// Output the final image
	fmt.Println("6")
	controller.eventsSender.SendOutputPGM(world, turn)

	// TODO: Report the final state using FinalTurnCompleteEvent.
	fmt.Println("7")
	controller.eventsSender.SendFinalTurn(turn, stubs.GetAliveCells(world)) //turn+1? DEBUG
	// Make sure that the Io has finished any output before exiting.
	fmt.Println("8")
	c.eventsSender.C.ioCommand <- ioCheckIdle
	fmt.Println("9")
	<-c.eventsSender.C.ioIdle
	// fmt.Printf("before\n")
	c.eventsSender.C.events <- StateChange{turn, Quitting}
	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	fmt.Println("10")
	close(c.eventsSender.C.events)
	fmt.Println("11")
}

// This method will be called if the Broker has a calculated new state
// for the user to view in SDL window
func (c *Controller) PushState(req stubs.PushStateRequest, res *stubs.NilResponse) (err error) {
	c.m.Lock()
	fmt.Printf("turn: %d\n", req.Turn)
	// util.VisualiseSquare(c.acknowledgedCells.CurrentWorld, len(c.acknowledgedCells.CurrentWorld), len(c.acknowledgedCells.CurrentWorld))
	c.acknowledgedCells.UpdateWorldAndTurn(req.FlippedCells, req.Turn)
	// util.VisualiseSquare(c.acknowledgedCells.CurrentWorld, len(c.acknowledgedCells.CurrentWorld), len(c.acknowledgedCells.CurrentWorld))
	// c.eventsSender.SendOutputPGM(c.acknowledgedCells.CurrentWorld, req.Turn) //DEBUG
	c.eventsSender.SendFlippedCellList(req.Turn, req.FlippedCells...)
	c.eventsSender.SendTurnComplete(req.Turn)
	c.m.Unlock()
	return
}

func (c *Controller) receiver(listening chan<- bool, p Params) {
	fmt.Println("starting controller listening")
	listener, err := net.Listen("tcp", ":"+strconv.Itoa(p.ListenPort))
	if err != nil {
		fmt.Println("could not listen on port " + strconv.Itoa(p.ListenPort))
		fmt.Printf("listenErr: %s\n", err.Error())
		return
	}
	defer listener.Close()
	go rpc.Accept(listener)
	listening <- true
	select {
	case <-c.exitChannels[receiver]:
	}
	return
}
