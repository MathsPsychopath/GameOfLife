package main

import (
	"errors"
	"fmt"
	"net/rpc"
	"sync"

	"uk.ac.bris.cs/gameoflife/stubs"
	"uk.ac.bris.cs/gameoflife/util"
)

type WorkerInfo struct {
	client    *rpc.Client
	workSize  int
	rowOffset int
	ipAddress string
}
type Broker struct {
	Mu                *sync.Mutex
	Workers           map[int]*WorkerInfo // Id : worker
	Controller        *rpc.Client
	NextID            int
	Pause             sync.WaitGroup
	isPaused          bool
	Params            stubs.StubParams
	CurrentWorld      [][]byte
	lastCompletedTurn int                  //if new worker added, worker deleted ... any change
	flippedCells      map[int][]util.Cell  //turn : flippedCells
	workersResponded  map[int]map[int]bool //turn: (id : bool)
	workerIds         []int
	exit              bool
	processCellsReq   chan bool
	errorChan         chan int
	processing        bool
}

// initialises Broker struct
func NewBroker() *Broker {
	return &Broker{errorChan: make(chan int), processCellsReq: make(chan bool), NextID: 0, Mu: new(sync.Mutex), Controller: nil, Workers: map[int]*WorkerInfo{}, workerIds: []int{}, workersResponded: make(map[int]map[int]bool), flippedCells: make(map[int][]util.Cell)}
}

func (b *Broker) resetBroker(id int) {
	//resetting variables
	b.Mu.Lock()
	defer b.Mu.Unlock()
	b.flippedCells = make(map[int][]util.Cell)
	b.workersResponded = make(map[int]map[int]bool)
	b.lastCompletedTurn = 0
	// flush channels
	for len(b.processCellsReq) != 0 {
		<-b.processCellsReq
	}
	//remove bad worker
	if id != -1 {
		b.Workers[id].client.Go(stubs.Shutdown, new(stubs.NilRequest), new(stubs.NilResponse), nil)
		delete(b.Workers, id)
		b.workerIds = stubs.RemoveSliceElement(b.workerIds, id)
	}

}

// removes all worker ids from b.Workers map
func (b *Broker) removeWorkersFromRegister(byWorker bool, ids ...int) {
	b.Mu.Lock()
	for _, id := range ids {
		if byWorker {
			b.Workers[id].client.Call(stubs.Shutdown, stubs.NilRequest{}, new(stubs.NilResponse))
		}
		delete(b.Workers, id)
		b.workerIds = stubs.RemoveSliceElement(b.workerIds, id)
	}
	b.Mu.Unlock()
}

// sets a new worker on b.Workers
func (b *Broker) addWorker(client *rpc.Client, ip string) {
	b.Mu.Lock()
	newWorker := WorkerInfo{
		client:    client,
		ipAddress: ip,
	}
	b.Workers[b.NextID] = &newWorker
	b.workerIds = append(b.workerIds, b.NextID)
	b.NextID++
	b.Mu.Unlock()
}

// sets the controller client if one doesn't already exist
func (b *Broker) setController(client *rpc.Client) (err error) {
	b.Mu.Lock()
	defer b.Mu.Unlock()
	if b.Controller != nil {
		return errors.New("couldn't connect: there's already a controller")
	}
	b.Controller = client
	return nil
}

// removes controller client
func (b *Broker) removeController() {
	b.Mu.Lock()
	b.Controller = nil
	b.Mu.Unlock()
}

// distributes the allocation as evenly as possible, minimising (max - min)
func divideEvenly(b *Broker) {
	workerCount := len(b.workerIds)
	workSize := b.Params.ImageHeight / workerCount
	remainder := b.Params.ImageHeight - workSize*workerCount
	for _, id := range b.workerIds {
		b.Workers[id].workSize = workSize
	}
	for _, id := range b.workerIds {
		if remainder <= 0 {
			continue
		}
		b.Workers[id].workSize += 1
		remainder--
	}
}

func (b *Broker) getHaloIPs(i int) (topWorkerIp, botWorkerIp string) {

	//topworker is also the previous worker because we are going top to bottom during section allocation
	topWorkerIndex := i - 1
	if topWorkerIndex == -1 {
		topWorkerIndex = len(b.workerIds) - 1
	}
	botWorkerIndex := i + 1
	if botWorkerIndex == len(b.workerIds) {
		botWorkerIndex = 0
	}
	topWorkerId := b.workerIds[topWorkerIndex]
	botWorkerId := b.workerIds[botWorkerIndex]
	topWorkerIp = b.Workers[topWorkerId].ipAddress
	botWorkerIp = b.Workers[botWorkerId].ipAddress
	fmt.Printf("topIp: %s, botIp %s\n", topWorkerIp, botWorkerIp)
	return
}

func (b *Broker) getWorkersNotResponded() []int {
	b.Mu.Lock()
	defer b.Mu.Unlock()
	out := b.workerIds
	for workerId := range b.workersResponded {
		out = stubs.RemoveSliceElement(b.workerIds, workerId)
	}
	return out
}

// prime workers should set the slice size that workers use for processing
// IMPORTANT: must mutex lock in calling scope
func (b *Broker) primeWorkers(firstTime bool) {
	b.Mu.Lock()
	defer b.Mu.Unlock()
	divideEvenly(b)
	currRowOffset := 0
	//this is assuming that the order of the map does not change within this function
	for i, id := range b.workerIds {
		b.Workers[id].rowOffset = currRowOffset
		workSize := b.Workers[id].workSize

		topWorkerIp, botWorkerIp := "", ""
		if len(b.workerIds) > 1 {
			topWorkerIp, botWorkerIp = b.getHaloIPs(i)
		}
		initRequest := stubs.InitWorkerRequest{
			Height:      workSize,
			Width:       b.Params.ImageWidth,
			RowOffset:   currRowOffset,
			TopWorkerIP: topWorkerIp,
			BotWorkerIP: botWorkerIp,
			FirstTime:   firstTime,
		}
		worker := b.Workers[id].client
		err := worker.Call(stubs.InitialiseWorker, initRequest, new(stubs.NilResponse))
		if err != nil {
			fmt.Printf("err: %s\n", err)
		}
		currRowOffset += workSize //keeping track of currentRowOffset
	}
	fmt.Println("Finished priming")
}

// sends shutdown request to all connected workers
func (b *Broker) killWorkers() {
	b.Mu.Lock()
	for _, worker := range b.Workers {
		worker.client.Call(stubs.Shutdown, stubs.NilRequest{}, new(stubs.NilResponse))
		worker.client.Close()
	}
	b.Mu.Unlock()
}

// sets the Params for the current controller
func (b *Broker) setParams(p stubs.StubParams) {
	b.Mu.Lock()
	b.Params = p
	b.Mu.Unlock()
}

// sets the initial world for the current controller
func (b *Broker) initialiseWorld(initialAliveCells []util.Cell) {
	b.Mu.Lock()
	b.CurrentWorld = stubs.ConstructWorld(initialAliveCells, b.Params.ImageHeight, b.Params.ImageWidth)
	b.lastCompletedTurn = 0
	b.Mu.Unlock()
}

// tests if the workerIds in given are the same as actually connected
func (b *Broker) isSameWorkers(ids []int) bool {
	b.Mu.Lock()
	defer b.Mu.Unlock()
	dummy := true
	idMap := make(map[int]*bool, len(ids))
	for _, id := range ids {
		idMap[id] = &dummy
	}
	//checks if workerId is in map
	for id := range b.Workers {
		if idMap[id] == nil {
			return false
		}
	}
	return true
}

// gets the slice for a worker
func (b *Broker) getSectionSlice(workerId int) [][]byte {
	rowOffset := b.Workers[workerId].rowOffset
	workSize := b.Workers[workerId].workSize
	fmt.Printf("worker: %d, startRow: %d, endRow: %d\n", workerId, rowOffset, rowOffset+workSize)
	return b.CurrentWorld[rowOffset : rowOffset+workSize]
}

// applies the flipped cell changes to the broker's current world
func (b *Broker) applyChanges(flippedCells []util.Cell) {
	for _, cell := range flippedCells {
		b.CurrentWorld[cell.Y][cell.X] ^= 0xFF
	}
}
