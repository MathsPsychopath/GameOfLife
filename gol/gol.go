package gol

// Params provides the details of how to run the Game of Life and which image to load.
type Params struct {
	Turns       int
	Threads     int
	ImageWidth  int
	ImageHeight int
	BrokerAddr  string
	ListenIP    string
	ListenPort  int
}

// Run starts the processing of Game of Life. It should initialise channels and goroutines.
func Run(p Params, events chan<- Event, keyPresses <-chan rune) {

	// for testing purposes
	if p.BrokerAddr == "" { //this is because the tests don't assign these variables, and we can't change the test files.
		p.BrokerAddr = "127.0.0.1:9000"
	}

	if p.ListenIP == "" {
		p.ListenIP = "127.0.0.1"
	}
	if p.ListenPort == 0 {
		p.ListenPort = 8090
	}

	// end for testing purposes
	ioCommand := make(chan ioCommand)
	ioIdle := make(chan bool)
	filename := make(chan string)
	output := make(chan uint8)
	input := make(chan uint8)

	ioChannels := ioChannels{
		command:  ioCommand,
		idle:     ioIdle,
		filename: filename,
		output:   output,
		input:    input,
	}
	go startIo(p, ioChannels)

	distributorChannels := distributorChannels{
		events:     events,
		ioCommand:  ioCommand,
		ioIdle:     ioIdle,
		ioFilename: filename,
		ioOutput:   output,
		ioInput:    input,
	}
	distributor(p, distributorChannels, keyPresses)
}
