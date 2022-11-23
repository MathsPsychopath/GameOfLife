package main

import (
	"flag"
	"fmt"
	"net"
	"net/rpc"

	"uk.ac.bris.cs/gameoflife/stubs"
)

var exit = make(chan bool)
var id int

type Worker struct {
	internalState [][]byte
}

// creates a 2d slice with dimensions (work slice) + halo
func (w *Worker) initialiseState(req stubs.Work) {
	worker := &Worker{}
	// take into account the halos
	worker.internalState = make([][]byte, req.State.P.ImageHeight + 2)
	for i := range(worker.internalState) {
		worker.internalState[i] = make([]byte, req.State.P.ImageWidth + 2)
	}
	return
}

var thisWorker *Worker

func (w *Worker) EvolveSlice(req stubs.Work, res stubs.Work) (err error) {
	
	res.StartRow, res.EndRow = req.StartRow, req.EndRow
	for i := req.StartRow; i < req.EndRow; i++ {
		for j := 0; j < req.State.P.ImageWidth; j++ {
			w.internalState[i - req.StartRow]
		}
	}
	state := stubs.PushStateBody{Turn: req.State.Turn + 1, P: req.State.P, Id: id}
	// TODO: evolve the slice
	// TODO: update internal state
	return
}

func (w *Worker) Shutdown(req stubs.NilRequest, res stubs.StatusResponse) (err error) {
	res.Status = stubs.Terminated
	exit <- true
	return
}

func main() {
	bAddr := flag.String("brokerIP", "127.0.0.1:9000", "IP address of broker")
	pAddr := flag.String("port", "9000", "Port to listen on")
    flag.Parse()
	// connect to client
	client, _ := rpc.Dial("tcp", *bAddr)
	res := new(stubs.ConnectResponse)
	client.Call(stubs.Connect, stubs.ConnectRequest{IP: stubs.IPAddress(*pAddr)}, res)
	id = res.Id

	// listen for work
    listener, err := net.Listen("tcp", ":" + *pAddr)
	rpc.Register(&Worker{})
	if err != nil {
		fmt.Println(err)
	}
    defer listener.Close()
    go rpc.Accept(listener)
	<-exit
}