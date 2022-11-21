package server

import "uk.ac.bris.cs/gameoflife/stubs"


type ioChannels struct {
	command <-chan ioCommand
	idle    chan<- bool

	filename <-chan string
	output   <-chan uint8
	input    chan<- uint8
}

// ioState is the internal ioState of the io goroutine.
type ioState struct {
	params   stubs.Params
	channels ioChannels
}

// ioCommand allows requesting behaviour from the io (pgm) goroutine.
type ioCommand uint8

// This is a way of creating enums in Go.
// It will evaluate to:
//		ioOutput 	= 0
//		ioInput 	= 1
//		ioCheckIdle = 2
const (
	ioOutput ioCommand = iota
	ioInput
	ioCheckIdle
)