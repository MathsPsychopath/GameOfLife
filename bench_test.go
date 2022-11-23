package main

import (
	"fmt"
	"testing"

	"uk.ac.bris.cs/gameoflife/gol"
)

func BenchmarkLocal(b *testing.B) {
	p := gol.Params{
		Turns:       1000,
		Threads:     8,
		ImageWidth:  512,
		ImageHeight: 512,
	}
	benchmark(b, p)
}

func benchmark(b *testing.B, p gol.Params) {
	fmt.Print("working\n")

	for i := 0; i < 1; i++ {
		events := make(chan gol.Event)
		go gol.Run(p, events, nil)

		for event := range events {
			switch event.(type) {
			case gol.FinalTurnComplete:
				break
			}
		}
	}

}
