#!/bin/sh
# Run nekomimi benchmarks with CPU and memory profiling.
# Output files are written to the current directory.
#
# Usage: ./pprof.sh [benchmark_filter]
#
# Examples:
#   ./pprof.sh                          # all benchmarks
#   ./pprof.sh Write_Rotate100          # single benchmark
#   ./pprof.sh "File.*Rotate"           # regex filter
#
# After running, inspect with:
#   go tool pprof cpu.out
#   go tool pprof mem.out
#   go tool pprof -http=:8080 cpu.out

set -eu

FILTER="${1:-.}"
BENCHTIME="${BENCHTIME:-1s}"
DIR="$(dirname "$0")"

cd "$DIR/.."

go test -bench="$FILTER" \
    -benchmem \
    -benchtime="$BENCHTIME" \
    -cpuprofile=cpu.out \
    -memprofile=mem.out \
    -blockprofile=block.out \
    -count=1 \
    -timeout 300s \
    ./benchmark/

echo ""
echo "Profiles written: cpu.out mem.out block.out"
echo "View with: go tool pprof cpu.out"
