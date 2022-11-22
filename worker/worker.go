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

func (w *Worker) EvolveSlice(req stubs.Work, res stubs.Work) (err error) {
	
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