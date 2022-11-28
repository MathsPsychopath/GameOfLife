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
	Mu   			*sync.Mutex
	Workers			map[int]*rpc.Client
	Controller  	*rpc.Client
	NextID      	int
	Pause       	sync.WaitGroup
	isPaused    	bool
	Params      	stubs.StubParams
	CurrentWorld	[][]byte
	workAllocation  []int
}

// initialises Broker struct
func NewBroker() *Broker {
	return &Broker{NextID: 0, Mu: new(sync.Mutex), Controller: nil, Workers: map[int]*rpc.Client{}}
}

// removes all worker ids from b.Workers map
func (b *Broker) removeWorkersFromRegister( byWorker bool,ids ...int) {
	b.Mu.Lock()
	for _, id := range ids {
		if byWorker {
			b.Workers[id].Call(stubs.Shutdown, stubs.NilRequest{}, new(stubs.NilResponse))
		}
		delete(b.Workers, id)
	}
	b.Mu.Unlock()
}

// sets a new worker on b.Workers
func (b *Broker) addWorker(client *rpc.Client) {
	b.Mu.Lock()
	b.Workers[b.NextID] = client
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
func divideEvenly(allocation, workersLeft int) []int {
	work := make([]int, workersLeft)
	workSize := allocation / workersLeft
	remainder := allocation - workSize * workersLeft
	for i := 0; i < workersLeft; i++ {
		work[i] = workSize
	}
	for i := 0; remainder > 0; i++ {
		work[i] += 1
		remainder--
	}
	return work
}

// prime workers should set the slice size that workers use for processing
// IMPORTANT: must mutex lock in calling scope
func (b *Broker) primeWorkers() {
	allocation := divideEvenly(b.Params.ImageHeight, len(b.Workers))
	fmt.Println("finished dividing work. Allocation: ", allocation)
	index := 0
	for i, worker := range b.Workers {
		workSize := allocation[index]
		initRequest := stubs.InitWorkerRequest{
			Height: workSize,
			Width:  b.Params.ImageWidth,
			WorkerIndex: i,
		}
		worker.Call(stubs.InitialiseWorker, initRequest, new(stubs.NilResponse))
		index++
	}
	fmt.Println("Finished priming")
	b.workAllocation = allocation
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

// converts b.Workers into a map of workerIds
func getWorkerKeys(m map[int]*rpc.Client) []int {
	keys := []int{}
	for key := range m {
		keys = append(keys, key)		
	}
	return keys
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
	for id := range b.Workers {
		if idMap[id] == nil {
			return false
		}
	}
	return true
}

// called when sequence of workers changes. Will await worker to connect if none
func (b *Broker) handleWorkerChanges() (hasReprimed bool, acknowledgedWorkers []int){
	hasReprimed = true
	b.Mu.Lock()
	b.primeWorkers()
	acknowledgedWorkers = getWorkerKeys(b.Workers)
	b.Mu.Unlock()
	return
}

// executes 1 iteration of single worker GOL
func (b *Broker) singleWorkerGOL(hasReprimed bool, acknowledgedWorkers []int) (changed []util.Cell, success bool, faultyWorkerIds []int){
	var workReq stubs.WorkRequest
	if hasReprimed {
		// repriming will make the worker empty
		workReq = stubs.WorkRequest{
			FlippedCells: stubs.SquashSlice(b.CurrentWorld),
		}
	} else {
		// the worker will already have their own state
		workReq = stubs.WorkRequest{ FlippedCells: nil, TopHalo: nil, BottomHalo: nil}
	}
	workRes := new(stubs.WorkResponse)

	err := b.Workers[acknowledgedWorkers[0]].Call(stubs.EvolveSlice, workReq, workRes)
	success = err == nil
	if !success {
		faultyWorkerIds = append(faultyWorkerIds, acknowledgedWorkers[0])
		return
	}
	changed = workRes.FlippedCells
	return 
}

// gets the slice for a worker, size = b.workAllocation
func (b *Broker) getSectionSlice(workerNo, sliceStartIndex int) [][]byte {
	fmt.Println(sliceStartIndex, sliceStartIndex + b.workAllocation[workerNo])
	return b.CurrentWorld[sliceStartIndex: sliceStartIndex + b.workAllocation[workerNo]]
}

// executes 1 iteration of Multi-worker GOL
func (b *Broker) multiWorkerGOL(hasReprimed bool, acknowledgedWorkers []int) (changed []util.Cell, success bool, faultyWorkerIds []int) {
	// multiple workers => halos
	totalWorkers := len(acknowledgedWorkers)
	var waitgroup sync.WaitGroup
	success = true
	changed = []util.Cell{}
	sliceStartIndex := 0
	serialise := make(chan util.Cell, 10000)
	for currentWorkerNo, id := range acknowledgedWorkers {
		// assign halos to work request
		workReq := stubs.WorkRequest{}
		workReq.BottomHalo, workReq.TopHalo = 
			b.getHalos(currentWorkerNo, sliceStartIndex, totalWorkers)

		if hasReprimed {
			// reprimed workers will have blank states, so set state
			workReq.FlippedCells = stubs.SquashSlice(
				b.getSectionSlice(currentWorkerNo, sliceStartIndex),
			)
		} else {
			workReq.FlippedCells = nil
		}
		sliceStartIndex += b.workAllocation[currentWorkerNo]
		// async send and receive to allow other slice requests
		waitgroup.Add(1)
		done := make(chan *rpc.Call, 1)
		workRes := new(stubs.WorkResponse)
		b.Workers[id].Go(stubs.EvolveSlice, workReq, workRes, done)
		go func(id int) {
			res := <- done
			localSuccess := res.Error == nil
			if !localSuccess {
				faultyWorkerIds = append(faultyWorkerIds, id)
			} else {
				// we need to merge the flipped cells in different goroutines
				// so send into serialising channel
				for _, cell := range workRes.FlippedCells {
					serialise <- cell
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
		close(serialise)
	}()
	// this receives the cells from the goroutines
	for cell := range serialise {
		changed = append(changed, cell)
	}
	return
}

// returns the top and bottom halos for a given worker
func (b *Broker) getHalos(currentWorker, sliceStartIndex, totalWorkers int) (topHalo, bottomHalo []util.Cell) {
	workSize := b.workAllocation[currentWorker]
	if currentWorker == 0 {
		// this is the first slice
		topHalo = stubs.SquashSlice(
			[][]byte{ b.CurrentWorld[len(b.CurrentWorld)-1] },
		)
		bottomHalo = stubs.SquashSlice(
			[][]byte{ b.CurrentWorld[workSize] },
		)
		return
	}
	if currentWorker == totalWorkers - 1 {
		// this is the last slice
		topHalo = stubs.SquashSlice(
			[][]byte{ b.CurrentWorld[ sliceStartIndex - 1]},
		)
		bottomHalo = stubs.SquashSlice(
			[][]byte{ b.CurrentWorld[0] },
		)
		return
	}
	topHalo = stubs.SquashSlice(
		[][]byte{ b.CurrentWorld[sliceStartIndex - 1] },
	)
	bottomHalo = stubs.SquashSlice(
		[][]byte{ b.CurrentWorld[sliceStartIndex + workSize] },
	)
	return
}

// applies the flipped cell changes to the broker's current world
func (b *Broker) applyChanges(flippedCells []util.Cell) {
	for _, cell := range flippedCells {
		b.CurrentWorld[cell.Y][cell.X] ^= 0xFF
	}
}