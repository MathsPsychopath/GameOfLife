package main

import (
	"flag"
	"fmt"
	"net"
	"net/rpc"
	"sync"

	"uk.ac.bris.cs/gameoflife/stubs"
	"uk.ac.bris.cs/gameoflife/util"
)

// Mutex locked data to avoid race conditions
type AliveContainer struct {
	mu 		sync.Mutex
	turn    int
	count   int
}

type WorldContainer struct {
	mu 		sync.Mutex
	world   [][]byte
}

var acknowledgedAlive = AliveContainer{turn: 0, count: 0}
var acknowledgedWorld = WorldContainer{world: nil}

// update the turn and alive
func (a *AliveContainer) update(newTurn, newAlive int) {
	a.mu.Lock()
	a.turn = newTurn
	a.count = newAlive
	a.mu.Unlock()
}

// may need an implementation that changes based on cell flipped events
func (w *WorldContainer) update(world [][]byte) {
	w.mu.Lock()
	w.world = world
	w.mu.Unlock()
}

// create a blank 2D slice of size p.ImageHeight x p.ImageWidth
func initialiseNewWorld(p stubs.StubParams) [][]byte {
	world := make([][]byte, p.ImageHeight)
	for i := range(world){
		world[i] = make([]byte, p.ImageWidth)
	}
	return world
}

// count the number of neighbours that a particular cell has in the world
func getNeighbourCount(world [][]byte, row, column int, p stubs.StubParams) int {
	alive := 0
	offsets := []util.Cell{
		{X:-1,Y: -1},
		{X:-1,Y: 0},
		{X:-1,Y: 1},
		{X:0, Y:-1},
		{X:0, Y:1},
		{X:1, Y:-1},
		{X:1, Y:0},
		{X:1, Y:1},
	}
	for _,offset := range offsets {
		actualRow := (row + offset.X) % p.ImageHeight
		if actualRow < 0 {
			actualRow = p.ImageHeight - 1
		}
		actualCol := (column + offset.Y) % p.ImageWidth
		if actualCol < 0 {
			actualCol = p.ImageWidth - 1
		}
		if world[actualRow][actualCol] == 0xFF {
			alive++
		}
	}
	return alive
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

// complete 1 iteration of the world following Game of Life rules
func evolve(world [][]byte, p stubs.StubParams) [][]byte {
	newWorld := initialiseNewWorld(p);
	for i, row := range world {
		for j := range row {
			neighbours := getNeighbourCount(world, i, j, p)
			if neighbours < 2 || neighbours > 3{
				newWorld[i][j] = 0x00
			} else {
				if world[i][j] == 0x00 && neighbours == 3{
					newWorld[i][j] = 0xFF
					continue
				}
				newWorld[i][j] = world[i][j]
			}
		}
	}
	return newWorld
}

// This implements the interface method
func EvolveWorld(world [][]byte, p stubs.StubParams) [][]byte {
    turn := 0
	// TODO: Execute all turns of the Game of Life.
	for ;turn < p.Turns; turn++ {
		world = evolve(world, p);
		count := getAliveCellsCount(world)
		go func() {
			acknowledgedAlive.update(turn, count)
		}()
	}
    return world
}

type GameOfLife struct {}

// expose an interface method
func (g *GameOfLife) Evolve(req stubs.EvolveRequest, res *stubs.Response) (err error) {
    res.World = EvolveWorld(req.World, req.P)
    return
}

func (g *GameOfLife) GetAliveCells(req stubs.GetRequest, res *stubs.Response) (err error) {
	fmt.Println("received GAC call")
	acknowledgedAlive.mu.Lock()
	res.Count = acknowledgedAlive.count
	res.Turn = acknowledgedAlive.turn
	acknowledgedAlive.mu.Unlock()
	return
}

type InputOutput struct {}

func (i *InputOutput) SaveState(req stubs.GetRequest, res *stubs.Response) (err error) {
	acknowledgedWorld.mu.Lock()
	res.World = acknowledgedWorld.world
	acknowledgedWorld.mu.Unlock()
	acknowledgedAlive.mu.Lock()
	res.Turn = acknowledgedAlive.turn
	acknowledgedAlive.mu.Unlock()
	return
}

// func (i *InputOutput) ControllerStop(req stubs.GetRequest, res *stubs.ResponseStatus) (err error) {
// 	// 
// }

func main() {
    pAddr := flag.String("port", "8030", "Port to listen on")
    flag.Parse()
    rpc.Register(&GameOfLife{})

    listener, _ := net.Listen("tcp", ":" + *pAddr)
    defer listener.Close()
    rpc.Accept(listener)

}