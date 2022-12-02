package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"uk.ac.bris.cs/gameoflife/util"
)

func writePgmImage(world [][]byte, filename string, imageSize int) {
	_ = os.Mkdir("out", os.ModePerm)

	// Request a filename from the distributor.

	file, ioError := os.Create(filename)
	util.Check(ioError)
	defer file.Close()

	_, _ = file.WriteString("P5\n")
	//_, _ = file.WriteString("# PGM file writer by pnmmodules (https://github.com/owainkenwayucl/pnmmodules).\n")
	_, _ = file.WriteString(strconv.Itoa(imageSize))
	_, _ = file.WriteString(" ")
	_, _ = file.WriteString(strconv.Itoa(imageSize))
	_, _ = file.WriteString("\n")
	_, _ = file.WriteString(strconv.Itoa(255))
	_, _ = file.WriteString("\n")

	for y := 0; y < imageSize; y++ {
		for x := 0; x < imageSize; x++ {
			_, ioError = file.Write([]byte{world[y][x]})
			util.Check(ioError)
		}
	}

	ioError = file.Sync()
	util.Check(ioError)

	fmt.Println("File", filename, "output done!")
}

func getDiff(image1, image2 []byte) []byte {
	out := []byte{}
	for i := 0; i < len(image1); i++ {
		var cell byte
		if image1[i] != image2[i] {
			cell = 0xFF
		} else {
			cell = 0x00
		}
		out = append(out, cell)
	}
	return out
}

func getMatrix(cells []byte, imageSize int) [][]byte {
	out := make([][]byte, imageSize)
	for i := 0; i < imageSize; i++ {
		out[i] = make([]byte, imageSize)
		for j := 0; j < imageSize; j++ {
			out[i][j] = cells[i*imageSize+j]
		}
	}
	return out
}

func readPgmImage(filename string, imageSize int) []byte {

	// Request a filename from the distributor.

	data, ioError := ioutil.ReadFile(filename)
	util.Check(ioError)

	fields := strings.Fields(string(data))

	if fields[0] != "P5" {
		panic("Not a pgm file")
	}

	width, _ := strconv.Atoi(fields[1])
	if width != imageSize {
		panic("Incorrect width")
	}

	height, _ := strconv.Atoi(fields[2])
	if height != imageSize {
		panic("Incorrect height")
	}

	maxval, _ := strconv.Atoi(fields[3])
	if maxval != 255 {
		panic("Incorrect maxval/bit depth")
	}

	image := []byte(fields[4])
	return image
}

func main() {
	imageSize := 512
	filename := "../out/512x512x1.pgm"
	image1 := readPgmImage(filename, imageSize)
	filename = "../check/images/512x512x1.pgm"
	image2 := readPgmImage(filename, imageSize)
	diff := getDiff(image1, image2)
	matrix := getMatrix(diff, imageSize)
	filename = "../out/512x512x1_diff.pgm"
	writePgmImage(matrix, filename, imageSize)
	fmt.Printf("done\n")
}
