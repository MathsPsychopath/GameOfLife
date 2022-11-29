package gol

import (
	"strconv"
	"sync"

	"uk.ac.bris.cs/gameoflife/util"
)

type Sender struct {
	C 		   distributorChannels     
	P 		   Params
	mu         *sync.Mutex
}

func (s *Sender) SendStateChange(turn int, state State) {
	s.mu.Lock()
	s.C.events <- StateChange{CompletedTurns: turn, NewState: state}
	s.mu.Unlock()
}

func (s *Sender) SendAliveCellsList(turn int, cells []util.Cell) {
	s.mu.Lock()
	s.C.events <- AliveCellsCount{CompletedTurns: turn, CellsCount: len(cells)}
	s.mu.Unlock()
}

func (s *Sender) SendFlippedCellList(turn int, cells ...util.Cell) {
	s.mu.Lock()
	for _, cell := range(cells) {
		s.C.events <- CellFlipped{CompletedTurns: turn, Cell: cell}
	}
	s.mu.Unlock()
}

func (s *Sender) SendFinalTurn(turn int, cells []util.Cell) {
	s.mu.Lock()
	s.C.events <- FinalTurnComplete{CompletedTurns: turn, Alive: cells}
	s.mu.Unlock()
}

func (s *Sender) SendTurnComplete(turn int) {
	s.mu.Lock()
	s.C.events <- TurnComplete{CompletedTurns: turn}
	s.mu.Unlock()
}

func (s *Sender) GetInitialAliveCells() []util.Cell {
	s.C.ioCommand <- ioInput
	s.C.ioFilename <- (strconv.Itoa(s.P.ImageWidth) + "x" + strconv.Itoa(s.P.ImageHeight))

	cells := []util.Cell{}
	for i := 0; i < s.P.ImageHeight; i++ {
		for j := 0; j < s.P.ImageWidth; j++ {
			if <-s.C.ioInput == 0xFF {
				cells = append(cells, util.Cell{X:j, Y: i})
			}
		}
	}

	return cells
}

func (s *Sender) SendOutputPGM(world [][]byte, turn int) {
	s.C.ioCommand <- ioOutput
	s.C.ioFilename <- (strconv.Itoa(s.P.ImageWidth) + "x" + strconv.Itoa(s.P.ImageHeight) + "x" + strconv.Itoa(turn))

	for _, row := range world {
		for _, cell := range row {
			s.C.ioOutput <- cell
		}
	}
	s.C.ioCommand <- ioCheckIdle
	<-s.C.ioIdle
}