# Commands
## Benchmark:
go run server.go
then
In separate terminal:
go test -run=^$ -bench ^BenchmarkLocal$ -benchtime=1x > benchmark/benchmark.txt
cd benchmark
python3 plot.py