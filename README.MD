# What is this repo?
This repo is my coursework from the Computer Systems A unit, Year 2 of study at the University of Bristol. The project brief and the tasks needed to be completed are found [here](https://github.com/MathsPsychopath/GameOfLife/blob/main/project_brief.md). Written in Go, this is the first half of the coursework in which Game of Life must be written to run with parallel workers on a single machine.

# What techniques, skills did I learn from this?
  - [X] Programming in Golang
  - [X] Multithreading and parallel computing
  - [X] Mutex locks, condition variables (race condition-preventing techniques)
  - [X] Using channels and communicating between different processes
  - [X] Efficient-computation techniques (modular arithmetic vs. branching statements vs. bitwise assignment)
  
# Project Structure
If you want to look around, this is the structure:
 - root - tests provided, Go package management
 - [root/benchmark](https://github.com/MathsPsychopath/GameOfLife/tree/main/benchmark) - benchmarks for the code
 - [root/channels](https://github.com/MathsPsychopath/GameOfLife/tree/main/channels) - self-made channel implementation (was a task)
 - [root/gol](https://github.com/MathsPsychopath/GameOfLife/tree/main/gol) - the bulk of program:
   - [distributor](https://github.com/MathsPsychopath/GameOfLife/blob/main/gol/distributor.go) - the part where turns of Game of Life is processed, work distributed to threads
   - [gol](https://github.com/MathsPsychopath/GameOfLife/blob/main/gol/gol.go) - the executor of the distributor (run by tests and main)
 - others - coursework-provided files
 
# Running this for yourself
You can try this by doing:
1. `git clone git@github.com:MathsPsychopath/GameOfLife.git`
2. `cd GameOfLife`
3. `go mod download` (assuming you have Golang installed)
4. `go run .` (requires an SDL enabled window. Ideally from Linux)
