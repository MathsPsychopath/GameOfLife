package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"net/rpc"
	"sync"

	"uk.ac.bris.cs/gameoflife/stubs"
	"uk.ac.bris.cs/gameoflife/util"
)

type Worker struct {
	client   *rpc.Client
	id        int
}

type Broker struct {
	mu      	 *sync.Mutex
	workers     []Worker
	controller 	 *rpc.Client
}

func (b *Broker) removeWorker(id int) {
	filteredWorkers := make([]Worker, 2)
	for _,worker := range(b.workers) {
		if (worker.id != id) {
			filteredWorkers = append(filteredWorkers, worker)
		}
	}
	b.workers = filteredWorkers
}

type Pause struct {
	pauseGOL sync.WaitGroup
	isPaused bool
}

var id = 0
var state = stubs.New(nil, 0)
var exit = make(chan bool)
var pause = Pause{isPaused: false}
var workerConnected = make(chan int)

// Sends work to all clients registered in Broker.workers
func sendWork(b *Broker, req stubs.StartGOLRequest) bool {
	var waitgroup sync.WaitGroup
	success := false
	var faultyId int = -1
	newCells := make([]util.Cell, 10000)

	b.mu.Lock()
	for i, worker := range(b.workers) {
		waitgroup.Add(1)
		// define work
		cells, turn := state.Get()
		reqState := stubs.PushStateBody{
			Id: worker.id, Cells: cells, Turn: turn, P: req.P,
		}
		req := stubs.Work{
			StartRow: i, EndRow: i+1, State: reqState,
		}
		res := new(stubs.Work)

		// try doing work
		go func (worker Worker) {
			err := worker.client.Call(stubs.EvolveSlice, req, res)
			newCells = append(newCells, res.State.Cells...)
			waitgroup.Done()

			if err != nil {
				// something went wrong mid-turn, so report failure
				faultyId = worker.id
				success = false
			}
		} (worker)
	}
	waitgroup.Wait()
	// delete faulty worker
	if faultyId != -1 {
		b.removeWorker(faultyId)
	}
	b.mu.Unlock()
	return success
}

// This sends the world's state
func (b *Broker) SaveState(req stubs.NilRequest, res *stubs.PushStateBody) (err error) {
	if state.Cells == nil {
		return errors.New("could not find world to save")
	}
	res.Cells, res.Turn = state.Get()
	return
}

// This will start the GOL process and send processed turns back to client
func (b *Broker) StartGOL(req stubs.StartGOLRequest, res *stubs.StatusResponse) (err error) {
	for turn := 0; turn < req.P.Turns; turn++ {
		pause.pauseGOL.Wait()
		if b.controller == nil {
			fmt.Println("could not find controller")
			return
		}
		if len(b.workers) == 0 {
			fmt.Println("Waiting for workers to connect")
			<-workerConnected
			fmt.Println("Worker has connected!")
		}
		success := false
		// repeat if not successful
		for !success {
			success = sendWork(b, req)
		} 
		cells, _ := state.Get()
		req := stubs.PushStateBody{Turn: turn, Cells: cells, P: req.P}
		res := new(stubs.StatusResponse)
		b.mu.Lock()
		b.controller.Call(stubs.PushState, req, res)
		b.mu.Unlock()
	}
	return
}

// This will initialise the server -> client communication
func (b *Broker) ConConnect(req stubs.ConnectRequest, res *stubs.ConnectResponse) (err error) {
	b.mu.Lock()
	fmt.Println("received controller connect request")
	b.controller, _ = rpc.Dial("tcp", string(req.IP))
	b.mu.Unlock()
	return
}

// This will return the alive cells
func (b *Broker) GetAlive(req stubs.NilRequest, res *stubs.PushStateBody) (err error) {
	res.Cells, res.Turn = state.Get()
	return
}

// This will close the workers, broker and send the latest state back
func (b *Broker) SerQuit(req stubs.NilRequest, res *stubs.PushStateBody) (err error) {
	b.mu.Lock()
	for _, worker := range(b.workers) {
		res := new(stubs.StatusResponse)
		worker.client.Call(stubs.Shutdown, stubs.NilRequest{}, res)
		worker.client.Close()
	}
	b.controller.Close()
	defer func(){ exit <- true }()
	b.mu.Unlock()
	res.Cells, res.Turn = state.Get()
	return
}

func (b *Broker) PauseState(req stubs.NilRequest, res *stubs.StatusResponse) (err error) {
	if pause.isPaused {
		pause.pauseGOL.Done()
		res.Status = stubs.Running
		_, res.Turn = state.Get()
	} else {
		pause.pauseGOL.Add(1)
		res.Status = stubs.Running
		_, res.Turn = state.Get()
	}
	return
}

func (b *Broker) ConQuit(req stubs.NilRequest, res *stubs.StatusResponse) (err error) {
	b.mu.Lock()
	b.controller.Close()
	b.controller = nil
	b.mu.Unlock()
	res.Status = stubs.Terminated
	return
}

func (b *Broker) Connect(req stubs.ConnectRequest, res *stubs.ConnectResponse) (err error) {
	b.mu.Lock()
	client, err := rpc.Dial("tcp", string(req.IP))
	if err != nil {
		fmt.Println(err)
		return
	}
	b.workers = append(b.workers, Worker{id: id, client: client})
	id++
	select {
	case workerConnected <- id:
	default:
	}
	return 
}

func main() {
	pAddr := flag.String("port", "9000", "Port to listen on")
    flag.Parse()
	broker := &Broker{}
	broker.mu = new(sync.Mutex)
	rpc.Register(broker)

    listener, err := net.Listen("tcp", ":" + *pAddr)
	if err != nil {
		fmt.Println(err)
	}
    defer listener.Close()
    go rpc.Accept(listener)
	<-exit
}