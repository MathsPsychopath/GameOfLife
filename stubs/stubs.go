package stubs

import (
	"uk.ac.bris.cs/gameoflife/util"
)

// Params provides the details of how to run the Game of Life and which image to load.
type StubParams struct {
	Turns       int
	Threads     int
	ImageWidth  int
	ImageHeight int
}

// # Controller -> Broker methods

// we use this to connect a controller to the Broker
var ControllerConnect = "Broker.ControllerConnect"

// req ConnectRequest,  sends NilResponse  x
// - client must send Params and its own IP address it is listening for updates on

// we use this to start thegame of life processing
var StartGOL = "Broker.StartGOL"

// req StartGOLRequest, sends NilResponse  x
// - client must send the initial alive cells and params

// we use this to do a clean shutdown of the distributed system
var ServerQuit = "Broker.ServerQuit"

// req NilRequest, 		sends NilResponse   x

// we use this to indicate the controller wants to quit
var ControllerQuit = "Broker.ControllerQuit"

// req NilRequest, 		sends NilResponse  x

// we use this to indicate the controller wants to pause processing
var PauseState = "Broker.PauseState"

// req NilRequest, 		sends PauseResponse   x
// - server must send the turn currently being processed

// # Broker -> Controller methods

// this method is used to update the controller what the new state is
var PushState = "Controller.PushState"

// req PushStateRequest,   sends NilResponse  x
// - the client must send the flipped cells from the previous and turn

// # Broker -> Worker methods

// this method is used to prime the worker to start processing, or
// to reset the worker in case another quits or is added
var InitialiseWorker = "Worker.InitialiseWorker"

// req InitWorkerRequest, res NilResponse
// - client must send the Height, Width of slice; if its the only worker;

// this method is used to send the work for processing
var EvolveSlice = "Worker.EvolveSlice"

// req WorkRequest,      sends WorkResponse
// - client must send the Halos if any, FlippedCells to work on
// - server must send NewSlice,

// this method is used to cleanly shutdown the worker
var Shutdown = "Worker.Shutdown"

// req NilRequest,      sends NilResponse  x

// # Worker -> Broker methods

// this method is used by worker to connect to broker
var WorkerConnect = "Broker.WorkerConnect"

// req ConnectRequest,  sends ConnectResponse x
// client must send the IP address it is listening on
// server must send id of the worker

// this method is used by workers to cleanly disconnect
var WorkerDisconnect = "Broker.WorkerDisconnect"

// req RemoveRequest, sends NilResponse
// client must send the id of itself

var PushHalo = "Worker.PushHalo"

var BrokerPushState = "Broker.PushState"

// No information needed request
type NilRequest struct{}

// No information needed response
type NilResponse struct{}

// This request gives the broker the IP address a worker is listening on
type ConnectRequest struct {
	IP string
}

// This response is given when a worker successfully connects
type ConnectResponse struct {
	Id int
}

// This request is sent by workers upon SIGINT
type RemoveRequest struct {
	Id int
}

type StateValue uint8

const (
	Paused StateValue = iota
	Running
)

// This response is given when 'p' is pressed
type PauseResponse struct {
	Status StateValue
}

// This request sends the data to the broker to process
type StartGOLRequest struct {
	InitialAliveCells []util.Cell
	P                 StubParams
}

// This request sends current state of the world at turn
type PushStateRequest struct {
	FlippedCells []util.Cell
	Turn         int
}

// This request sends the preliminary information to initialise the worker
type InitWorkerRequest struct {
	RowOffset   int //number of rows from row 0 (top row)
	Height      int
	Width       int
	TopWorkerIP string
	BotWorkerIP string
	FirstTime   bool
}

// This request sends the halos and slice of world to work on
type WorkRequest struct {
	FlippedCells   []util.Cell
	Turns          int
	StartTurn      int
	IsSingleWorker bool
}

// This response is sent by workers when work done
type WorkResponse struct {
	FlippedCells []util.Cell
}

type PushHaloRequest struct {
	Halo  []byte
	IsTop bool
}

type BrokerPushStateRequest struct {
	FlippedCells []util.Cell
	Turn         int
	WorkerId     int
}
