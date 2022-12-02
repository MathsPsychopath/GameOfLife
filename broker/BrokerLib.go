package main

import (
	"errors"
	"fmt"
	"net/rpc"
	"sync"

	"uk.ac.bris.cs/gameoflife/stubs"
	"uk.ac.bris.cs/gameoflife/util"
)

type Broker struct {
	Mu               *sync.Mutex
	Workers          map[int]*rpc.Client //for HALO exchange: we need to know which workers are next to each other. Solution: make Workers a map[(int, int)]*rpc.Client.
	Controller       *rpc.Client
	NextID           int
	Pause            sync.WaitGroup
	isPaused         bool
	Params           stubs.StubParams
	CurrentWorld     [][]byte
	workAllocation   map[int]int
	workerRowOffsets map[int]int
	workerIds        []int
}

// initialises Broker struct
func NewBroker() *Broker {
	return &Broker{NextID: 0, Mu: new(sync.Mutex), Controller: nil, Workers: map[int]*rpc.Client{}, workerIds: []int{}, workerRowOffsets: map[int]int{}}
}

// removes all worker ids from b.Workers map
func (b *Broker) removeWorkersFromRegister(byWorker bool, ids ...int) {
	b.Mu.Lock()
	for _, id := range ids {
		if byWorker {
			b.Workers[id].Call(stubs.Shutdown, stubs.NilRequest{}, new(stubs.NilResponse))
		}
		delete(b.Workers, id)
		b.workerIds = stubs.RemoveSliceElement(b.workerIds, id)
	}
	b.Mu.Unlock()
}

// sets a new worker on b.Workers
func (b *Broker) addWorker(client *rpc.Client) {
	b.Mu.Lock()
	b.Workers[b.NextID] = client
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
func divideEvenly(rowCount int, workerIds []int) map[int]int {
	workerCount := len(workerIds)
	workAllocationMap := make(map[int]int)
	workSize := rowCount / workerCount
	remainder := rowCount - workSize*workerCount
	for _, id := range workerIds {
		workAllocationMap[id] = workSize
	}
	for _, id := range workerIds {
		if remainder <= 0 {
			continue
		}
		workAllocationMap[id] += 1
		remainder--
	}
	return workAllocationMap
}

// prime workers should set the slice size that workers use for processing
// IMPORTANT: must mutex lock in calling scope
func (b *Broker) primeWorkers() {
	b.Mu.Lock()
	defer b.Mu.Unlock()
	workAllocationMap := divideEvenly(b.Params.ImageHeight, b.workerIds)
	fmt.Println("finished dividing work. Allocation: ", workAllocationMap)
	currRowOffset := 0
	for id, worker := range b.Workers {
		b.workerRowOffsets[id] = currRowOffset
		workSize := workAllocationMap[id]
		// fmt.Printf("worksize: %d, imageWidth: %d\n", workSize, b.Params.ImageWidth)
		initRequest := stubs.InitWorkerRequest{
			Height:      workSize,
			Width:       b.Params.ImageWidth,
			WorkerIndex: id,
			RowOffset:   currRowOffset,
		}
		worker.Call(stubs.InitialiseWorker, initRequest, new(stubs.NilResponse))
		id++
		currRowOffset += workSize //keeping track of currentRowOffset
	}
	fmt.Println("Finished priming")
	b.workAllocation = workAllocationMap
}

// sends shutdown request to all connected workers
func (b *Broker) killWorkers() {
	b.Mu.Lock()
	for _, worker := range b.Workers {
		worker.Call(stubs.Shutdown, stubs.NilRequest{}, new(stubs.NilResponse))
		worker.Close()
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

// executes 1 iteration of single worker GOL
func (b *Broker) singleWorkerGOL(hasReprimed bool) (flippedCells []util.Cell, success bool, faultyWorkerIds []int) {
	var workReq stubs.WorkRequest
	if hasReprimed {
		// repriming will make the worker empty
		workReq = stubs.WorkRequest{
			FlippedCells: stubs.GetAliveCells(b.CurrentWorld),
		}
	} else {
		// the worker will already have their own state
		workReq = stubs.WorkRequest{FlippedCells: nil, TopHalo: nil, BottomHalo: nil}
	}
	workRes := new(stubs.WorkResponse)
	err := b.Workers[b.workerIds[0]].Call(stubs.EvolveSlice, workReq, workRes)
	success = err == nil
	if !success {
		faultyWorkerIds = append(faultyWorkerIds, b.workerIds[0])
		return
	}
	flippedCells = workRes.FlippedCells
	return
}

// gets the slice for a worker
func (b *Broker) getSectionSlice(workerId int) [][]byte {
	rowOffset := b.workerRowOffsets[workerId]
	workSize := b.workAllocation[workerId]
	fmt.Printf("worker: %d, startRow: %d, endRow: %d\n", workerId, rowOffset, rowOffset+workSize)
	return b.CurrentWorld[rowOffset : rowOffset+workSize]
}

// executes 1 iteration of Multi-worker GOL
func (b *Broker) multiWorkerGOL(hasReprimed bool) (flippedCells []util.Cell, success bool, faultyWorkerIds []int) {
	// multiple workers => halos
	var waitgroup sync.WaitGroup
	success = true
	flippedCells = []util.Cell{}
	flippedCellChannel := make(chan util.Cell, 10000)
	for id := range b.workerIds {
		// assign halos to work request
		workReq := stubs.WorkRequest{}
		workReq.BottomHalo, workReq.TopHalo = b.getHalos(id)

		if hasReprimed {
			// reprimed workers will have blank states, so set state
			workReq.FlippedCells = stubs.GetAliveCells( //alive cells == flipped cells because repriming
				b.getSectionSlice(id),
			)
		} else {
			workReq.FlippedCells = nil
		}
		// async send and receive to allow other slice requests
		waitgroup.Add(1)
		done := make(chan *rpc.Call, 1)
		workRes := new(stubs.WorkResponse)
		b.Workers[id].Go(stubs.EvolveSlice, workReq, workRes, done)
		go func(id int) {
			res := <-done
			localSuccess := res.Error == nil
			if !localSuccess {
				faultyWorkerIds = append(faultyWorkerIds, id)
			} else {
				// we need to merge the flipped cells in different goroutines
				// so send into serialising channel
				for _, cell := range workRes.FlippedCells {
					flippedCellChannel <- cell
				}
			}
			// if one fails, then discard the whole batch
			success = success && localSuccess
			waitgroup.Done()
		}(id)
	}
	// when all goroutines have finished, this will unblock the "for range chan"
	go func() {
		waitgroup.Wait()
		close(flippedCellChannel)
	}()
	// this receives the cells from the goroutines
	for cell := range flippedCellChannel {
		flippedCells = append(flippedCells, cell)
	}
	return
}

// returns the top and bottom halos for a given worker
func (b *Broker) getHalos(workerId int) (topHalo, bottomHalo []byte) {

	workSize := b.workAllocation[workerId]
	if b.workerRowOffsets[workerId] == 0 { // if first slice
		topHalo = b.CurrentWorld[len(b.CurrentWorld)-1]
		bottomHalo = b.CurrentWorld[workSize]
		return
	}
	if b.workerRowOffsets[workerId]+b.workAllocation[workerId] == b.Params.ImageHeight { // if last slice
		topHalo = b.CurrentWorld[b.workerRowOffsets[workerId]-1]
		bottomHalo = b.CurrentWorld[0]
		return
	}
	// if any slice in between
	topHalo = b.CurrentWorld[b.workerRowOffsets[workerId]-1]
	bottomHalo = b.CurrentWorld[b.workerRowOffsets[workerId]+workSize]
	return
}

// applies the flipped cell changes to the broker's current world
func (b *Broker) applyChanges(flippedCells []util.Cell) {
	for _, cell := range flippedCells {
		b.CurrentWorld[cell.Y][cell.X] ^= 0xFF
	}
}
