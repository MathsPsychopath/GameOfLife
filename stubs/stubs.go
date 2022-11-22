package stubs

import "uk.ac.bris.cs/gameoflife/util"

// Params provides the details of how to run the Game of Life and which image to load.
type StubParams struct {
	Turns       int
	Threads     int
	ImageWidth  int
	ImageHeight int
}

// Controller -> Broker methods
var StartGOL = "Broker.StartGOL"   		// req StartGOLRequest, sends StatusResponse  x
var SaveState = "Broker.SaveState" 		// req NilRequest,      sends PushStateBody   x
var SerQuit = "Broker.SerQuit" 	       	// req NilRequest, 		sends PushStateBody   x
var ConQuit = "Broker.ConQuit"			// req NilRequest, 		sends StatusResponse  x
var PauseState = "Broker.Pause" 		// req NilRequest, 		sends PushStateBody   x
var GetAlive = "Broker.GetAlive"        // req NilRequest,      sends PushStateBody   x
var ConConnect = "Broker.ConConnect"    // req ConnectRequest,  sends StatusResponse  x
 
// Broker -> Controller methods 
var PushState = "Controller.PushState"  // req PushStateBody,   sends StatusResponse  x
 
// Broker -> Worker methods 
var EvolveSlice = "Worker.EvolveSlice"  // req Work,       		sends Work 
var Shutdown = "Worker.Shutdown"        // req NilRequest,      sends StatusResponse  x
 
// Worker -> Broker methods 
var Connect = "Broker.Connect" 			// req ConnectRequest,  sends ConnectResponse x

// Worker -> Worker methods
// var PassHalo = "Worker.PassHalo"		// req PushStateBody,   sends PushStateBody

type statusValue uint8
type IPAddress   string

const (
	Running statusValue = iota
	Paused
	Terminated
)

// This request is a empty indicating a State change
type NilRequest struct {}

// This request gives the broker the IP address a worker is listening on
type ConnectRequest struct {
	IP  IPAddress
}

// This response is given when a worker successfully connects
type ConnectResponse struct {
	Id  int
}

// This response indicates if the processing is paused or not
type StatusResponse struct {
	Status statusValue
	Turn   int
}

// This request sends the data to the broker to process
type StartGOLRequest struct {
	Cells []util.Cell
	P     StubParams
}

// This request sends current state of the world at turn
type PushStateBody struct {
	Cells []util.Cell
	Turn  int
	P     StubParams
	Id    int
}

type Work struct {
	State    PushStateBody
	StartRow int
	EndRow   int
}
