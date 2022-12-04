package stubs

import (
	"uk.ac.bris.cs/gameoflife/util"
)

/*
 * This is a library intended to convert slices into a communication friendly form.
 * The library also provides a means to reconstruct the cells into a 2d world.
 */

// squash a 2d slice into its alive cells.
// There are no generics in this version of Go so convert to [][]byte
func GetAliveCells(slice [][]byte) []util.Cell {
	cells := []util.Cell{}
	for i, row := range slice {
		for j, cell := range row {
			if cell == 0xFF {
				cells = append(cells, util.Cell{X: j, Y: i})
			}
		}
	}
	return cells
}

// generate a 2d world from the cells
func ConstructWorld(cells []util.Cell, height, width int) [][]byte {
	world := make([][]byte, height)
	for i := range world {
		world[i] = make([]byte, width)
	}
	for _, cell := range cells {
		world[cell.Y][cell.X] = 0xFF
	}
	return world
}

// converts Cell array to byte array
func ConstructHalo(cells []util.Cell, width int) []byte {
	halo := make([]byte, width)
	for _, cell := range cells {
		halo[cell.X] = 0xFF
	}
	return halo
}

// squash a halo into cells
func SquashHalo(halo []byte, row int) []util.Cell {
	cells := make([]util.Cell, 10)
	for i, cell := range halo {
		if cell == 0xff {
			cells = append(cells, util.Cell{X: i, Y: row})
		}
	}
	return cells
}

func RemoveSliceElement(s []int, val int) []int {
	i := FindValue(s, val)
	s[i] = s[len(s)-1]
	return s[:len(s)-1]
}

func FindValue(arr []int, element int) int {
	for i, val := range arr {
		if val == element {
			return i
		}
	}
	return -1
}

func FlushHaloChan(c chan []byte) {
	for len(c) != 0 {
		<-c
	}
}
