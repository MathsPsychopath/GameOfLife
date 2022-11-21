package stubs

import "uk.ac.bris.cs/gameoflife/util"

// Params provides the details of how to run the Game of Life and which image to load.
type StubParams struct {
	Turns       int
	Threads     int
	ImageWidth  int
	ImageHeight int
}

var Evolve = "GameOfLife.Evolve"
var GetAliveCells = "GameOfLife.GetAliveCells"
var Save = "InputOutput.SaveState"
var Quit = "InputOutput.ControllerStop"
var Kill = "InputOutput.KillWorkers"
var Pause = "InputOutput.PauseWorkers"

type statusValue uint8

const (
	Ok statusValue = iota
	Failed
)

type Response struct {
	World [][]byte
	Count int
	Turn int
}

type ResponseStatus struct {
	Status statusValue
}

type EvolveRequest struct {
	World [][]byte
	P StubParams
}

type GetRequest struct {}

// server -> distributor API
var SendState = "DistributorApi.ReceiveState"

type StageComplete struct {
	Turn int
	CellsFlipped []util.Cell
}


