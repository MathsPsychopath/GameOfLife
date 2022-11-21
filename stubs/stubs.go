package stubs

// Params provides the details of how to run the Game of Life and which image to load.
type StubParams struct {
	Turns       int
	Threads     int
	ImageWidth  int
	ImageHeight int
}

var Evolve = "GameOfLife.Evolve"

type Response struct {
	World [][]byte
}

type Request struct {
	World [][]byte
	P StubParams
}