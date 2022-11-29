package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"net/rpc"

	"uk.ac.bris.cs/gameoflife/stubs"
	"uk.ac.bris.cs/gameoflife/util"
)

var exit = make(chan bool)
var workerConnected = make(chan bool)

// initialises bidirectional comms with controller
func (b *Broker) ControllerConnect(req stubs.ConnectRequest, res *stubs.NilResponse) (err error) {
	fmt.Println("Received controller connect request")
	client, dialError := rpc.Dial("tcp", string(req.IP))
	if dialError != nil {
		err = errors.New("broker > we couldn't dial your IP address of " + string(req.IP))
		return
	}
	alreadyExistsError := b.setController(client)
	if alreadyExistsError != nil {
		return alreadyExistsError
	}
	fmt.Println("Controller connected successfully!")
	return
}

// starts the Game of Life Loop
func (b *Broker) StartGOL(req stubs.StartGOLRequest, res *stubs.NilResponse) (err error) {
	// define state to evolve from
	b.setParams(req.P)
	if req.P.Turns == 0 {
		b.Mu.Lock()
		b.Controller.Call(stubs.PushState, stubs.PushStateRequest{Turn: 0, FlippedCells: req.InitialAliveCells}, new(stubs.NilResponse))
		b.Mu.Unlock()
		return
	}
	b.initialiseWorld(req.InitialAliveCells)

	// if controller connects before any workers, block
	if (len(b.Workers) == 0) {
		fmt.Println("Waiting for workers to connect")
		<-workerConnected
		fmt.Println("Worker has connected!")
	}
	b.primeWorkers() // this is called serially so no mutex
	acknowledgedWorkers := getWorkerKeys(b.Workers)
	for turn := 0; turn < req.P.Turns; turn++ {
		fmt.Println("worker map: ", b.Workers)
		fmt.Println("acknowledged workers: ", acknowledgedWorkers)
		b.Pause.Wait()
		hasReprimed := false || turn == 0
		// check for any additions or disconnects
		if len(b.Workers) == 0 {
			fmt.Println("Waiting for workers to connect")
			<-workerConnected
			fmt.Println("Worker has connected!")
		}
		if !b.isSameWorkers(acknowledgedWorkers) {
			fmt.Println("workers sequence has changed")
			hasReprimed, acknowledgedWorkers = b.handleWorkerChanges()
		}
		
		var flippedCells []util.Cell
		var success bool
		var faultyWorkerIds = []int{}
		b.Mu.Lock()
		if len(b.Workers) == 1 {
			// do single worker GOL
			flippedCells, success, faultyWorkerIds = b.singleWorkerGOL(hasReprimed, acknowledgedWorkers)
		} else {
			if len(b.Workers) != 0 {
				// slice the world and distribute it to workers
				flippedCells, success, faultyWorkerIds = b.multiWorkerGOL(hasReprimed, acknowledgedWorkers)
			}
		}
		b.Mu.Unlock()
		
		if !success {
			//repeat processing
			b.removeWorkersFromRegister(false, faultyWorkerIds...)
			turn--
			fmt.Println("Worker(s) had an error, repeating without them")
			continue
		}
		fmt.Println("successful turn. sending results")
		b.Mu.Lock()
		if b.Controller == nil {
			b.Mu.Unlock()
			return
		}
		// consolidate and apply changes
		b.applyChanges(flippedCells)
		// rpc controller with flipped cells
		req := stubs.PushStateRequest{FlippedCells: flippedCells, Turn: turn}
		b.Controller.Call(stubs.PushState, req, new(stubs.NilResponse))
		b.Mu.Unlock()
	}
	return
}

// kills every worker and halts the broker
func (b *Broker) ServerQuit(req stubs.NilRequest, res *stubs.NilResponse) (err error) {
	b.Pause.Add(1)
	b.killWorkers()
	defer func(){exit <- true}()
	return
}

// remove the controller on voluntary request
func (b *Broker) ControllerQuit(req stubs.NilRequest, res *stubs.NilResponse) (err error) {
	b.Mu.Lock()
	b.Controller.Close()
	b.Mu.Unlock()
	defer b.removeController()
	return
}

// pauses the game of life loop
func (b *Broker) PauseState(req stubs.NilRequest, res *stubs.PauseResponse) (err error) {
	if b.isPaused {
		b.Pause.Done()
		b.isPaused = false
	} else {
		b.Pause.Add(1)
		b.isPaused = true
	}
	return
}

// connects worker to broker
func (b *Broker) WorkerConnect(req stubs.ConnectRequest, res *stubs.ConnectResponse) (err error) {
	fmt.Println("Received worker connect request")
	client, dialError := rpc.Dial("tcp", string(req.IP))
	if dialError != nil {
		err = errors.New("broker > could not dial IP " + string(req.IP))
	}
	res.Id = b.NextID
	b.addWorker(client)
	b.Mu.Lock()
	b.primeWorkers()
	b.Mu.Unlock()
	select {
	case workerConnected <- true:
	default:
	}
	fmt.Println("Worker connected successfully!")
	return
}

func (b *Broker) WorkerDisconnect(req stubs.RemoveRequest, res *stubs.NilResponse) (err error) {
	b.removeWorkersFromRegister(true, req.Id)
	fmt.Println("removed worker #", req.Id)
	return
}

func main() {
	pAddr := flag.String("port", "9000", "Port to listen on")
    flag.Parse()
	rpc.Register(NewBroker())

    listener, err := net.Listen("tcp", ":" + *pAddr)
	if err != nil {
		fmt.Println("could not listen on port " + *pAddr)
	}
    defer listener.Close()
    go rpc.Accept(listener)
	<-exit
}