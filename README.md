# Commands
## Benchmark:
go run server.go
then
In separate terminal:
go test -run=^$ -bench ^BenchmarkLocal$ > benchmark/benchmark.txt
