package gol

import "uk.ac.bris.cs/gameoflife/util"

type Sender struct {
	Events     chan<- Event
	P 		   Params
}

func (s *Sender) SendStateChange(turn int, state State) {
	s.Events <- StateChange{CompletedTurns: turn, NewState: state}
	return
}

func (s *Sender) SendAliveCellsList(turn int, cells []util.Cell) {
	s.Events <- AliveCellsCount{CompletedTurns: turn, CellsCount: len(cells)}
}

func (s *Sender) SendFlippedCellList(turn int, cells ...util.Cell) {
	for _, cell := range(cells) {
		s.Events <- CellFlipped{CompletedTurns: turn, Cell: cell}
	}
}

func (s *Sender) SendFinalTurn(turn int, cells []util.Cell) {
	s.Events <- FinalTurnComplete{CompletedTurns: turn, Alive: cells}
}

func (s *Sender) SendTurnComplete(turn int) {
	s.Events <- TurnComplete{CompletedTurns: turn}
}