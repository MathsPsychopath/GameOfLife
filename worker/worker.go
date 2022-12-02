package main

import (
	"flag"
	"fmt"
	"net"
	"net/rpc"
	"os"
	"os/signal"
	"syscall"

	"uk.ac.bris.cs/gameoflife/stubs"
)

var c chan os.Signal

type Worker struct {
	container stubs.CellsContainer //this will contain only this worker's slice, not the whole world
	rowOffset int
	id        int
	width     int
	height    int
}

func (w *Worker) EvolveSlice(req stubs.WorkRequest, res *stubs.WorkResponse) (err error) {
	// 1    worker GOL reprimed = no halos + body
	// multiworker GOL reprimed = halos    + body
	// 1    worker GOL noprime  = no halos + no body
	// multiworker GOL noprime  = halos    + no body
	if req.FlippedCells != nil {
		// the worker has been reprimed, so its internal state is empty
		fmt.Println("flippedCells: ", req.FlippedCells)
		w.container.UpdateWorldAndTurn(req.FlippedCells, 0) // TODO: change so that worker keeps track of own turn
	}

	var evolvedSlice [][]byte = createNewSlice(w.height, w.width)
	// perform iteration
	flipped := w.evolve(evolvedSlice, req.TopHalo, req.BottomHalo)
	// updated world in worker api
	w.container.UpdateWorld(evolvedSlice)
	// add flipped cells to response (sent to broker)
	res.FlippedCells = flipped
	return

}

func (w *Worker) InitialiseWorker(req stubs.InitWorkerRequest, res *stubs.NilResponse) (err error) {
	// if using bit masking, then set it to height - 1, width - 1
	w.width = req.Width
	w.height = req.Height
	fmt.Printf("Worker primed with height: %d & width: %d\n", w.height, w.width)
	w.container.UpdateWorld(createNewSlice(w.height, w.width))
	w.rowOffset = req.RowOffset
	return
}

// sent by broker to sleep the distributed system
func (w *Worker) Shutdown(req stubs.NilRequest, res *stubs.NilResponse) (err error) {
	// programmatic Ctrl-C
	defer func() { c <- syscall.SIGINT }()
	return
}

func main() {
	bAddr := flag.String("brokerIP", "127.0.0.1:9000", "IP address of broker")
	pAddr := flag.String("port", "9010", "Port to listen on")
	flag.Parse()

	// listen for work
	listener, err := net.Listen("tcp", ":"+*pAddr)
	worker := Worker{container: *stubs.NewCellsContainer()}
	rpc.Register(&worker)
	if err != nil {
		fmt.Println(err)
	}
	defer listener.Close()
	go rpc.Accept(listener)

	// connect to broker
	client, _ := rpc.Dial("tcp", *bAddr)
	res := new(stubs.ConnectResponse)
	req := stubs.ConnectRequest{
		IP: stubs.IPAddress("127.0.0.1:" + *pAddr),
	}
	client.Call(stubs.WorkerConnect, req, res)
	worker.id = res.Id

	// detect Ctrl-C
	c = make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	client.Call(stubs.WorkerDisconnect, stubs.RemoveRequest{Id: worker.id}, new(stubs.NilResponse))
}
