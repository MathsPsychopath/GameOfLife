package main

import (
	"fmt"
	"testing"

	"uk.ac.bris.cs/gameoflife/gol"
)

func BenchmarkLocal(b *testing.B) {
	turnConfs := []int{1}
	threadConfs := []int{1, 2, 3, 4, 5} //, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	imageConfs := []int{16, 64, 128, 256, 512}
	for _, threads := range threadConfs {
		for _, imageSize := range imageConfs {
			for _, turns := range turnConfs {
				p := gol.Params{
					Turns:       turns,
					Threads:     threads,
					ImageWidth:  imageSize,
					ImageHeight: imageSize,
				}
				name := fmt.Sprintf("%dx%d_%d-threads_%d-turns", imageSize, imageSize, threads, turns)
				b.Run(name, func(b *testing.B) {
					benchmark(b, p)
				})
			}
		}
	}

}

func benchmark(b *testing.B, p gol.Params) {
	for i := 0; i < b.N; i++ {
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
