// Package benchmark_test contains performance benchmarks for nekomimi
// handler implementations.
package benchmark_test

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fiathux/nekomimi"
	"github.com/fiathux/nekomimi/handlers/filerotate"
)

// —– helpers ————————————————————————————————————————————————

func tempDir(b *testing.B) string {
	dir, err := os.MkdirTemp("", "nekomimi-bench-*")
	if err != nil {
		b.Fatal(err)
	}
	return dir
}

// waitForGzip polls the directory until no uncompressed archive files
// remain, or a deadline is reached.  Only timestamped archive .log
// files (pending gzip) are counted — active and fallback log files
// (<prefix>.log, <prefix>_<n>.log) are ignored.
func waitForGzip(dir, prefix string, deadline time.Duration) error {
	dl := time.Now().Add(deadline)
	for {
		if pendingGzipCount(dir, prefix) == 0 {
			return nil
		}
		if time.Now().After(dl) {
			return fmt.Errorf(
				"archive gzip did not finish within %v", deadline)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func pendingGzipCount(dir, prefix string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return -1
	}
	count := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix+"_") {
			continue
		}
		if strings.HasSuffix(name, ".gz") {
			continue
		}
		if !strings.HasSuffix(name, ".log") {
			continue
		}
		// Match <prefix>_<yymmdd>_<seconds>.log.
		rest := strings.TrimSuffix(name, ".log")
		rest = rest[len(prefix)+1:]
		parts := strings.SplitN(rest, "_", 2)
		if len(parts) == 2 && len(parts[0]) == 6 {
			count++
		}
	}
	return count
}

// —– baseline messages —————————————————————————————————————————

const (
	tinyMsg  = "hello"
	smallMsg = "this is a typical log message with some context"
)

var largeMsg = strings.Repeat(
	"a much larger message that simulates logging structured "+
		"data or stack traces in production environments. ", 30)

const benchHeader = "2026-06-27 10:00:00.000 [INFO], bench - "

// —– pure write path (no rotation) —————————————————————————————

func BenchmarkFile_Write(b *testing.B) {
	dir := tempDir(b)
	defer os.RemoveAll(dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h, err := filerotate.New(ctx, filerotate.Config{
		Path: dir, FilePrefix: "bench",
	})
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.RegularLog(nekomimi.INFO, benchHeader, tinyMsg)
	}
}

// —– message size impact ————————————————————————————————————————

func BenchmarkFile_Write_Size(b *testing.B) {
	sizes := []struct {
		name string
		msg  string
	}{
		{"16B", tinyMsg},
		{"64B", smallMsg},
		{"1KB", largeMsg},
	}
	for _, sz := range sizes {
		b.Run(sz.name, func(b *testing.B) {
			dir := tempDir(b)
			defer os.RemoveAll(dir)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			h, err := filerotate.New(ctx, filerotate.Config{
				Path: dir, FilePrefix: "bench",
			})
			if err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			b.SetBytes(int64(len(sz.msg)))
			for i := 0; i < b.N; i++ {
				h.RegularLog(nekomimi.INFO, benchHeader, sz.msg)
			}
		})
	}
}

// —– concurrent write path ——————————————————————————————————————

func BenchmarkFile_Write_Parallel(b *testing.B) {
	dir := tempDir(b)
	defer os.RemoveAll(dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h, err := filerotate.New(ctx, filerotate.Config{
		Path: dir, FilePrefix: "bench",
	})
	if err != nil {
		b.Fatal(err)
	}
	msg := smallMsg
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			h.RegularLog(nekomimi.INFO, benchHeader, msg)
		}
	})
}

// —– raw writer path ————————————————————————————————————————————

func BenchmarkFile_RegularWriter(b *testing.B) {
	dir := tempDir(b)
	defer os.RemoveAll(dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h, err := filerotate.New(ctx, filerotate.Config{
		Path: dir, FilePrefix: "bench",
	})
	if err != nil {
		b.Fatal(err)
	}
	msg := smallMsg
	b.ResetTimer()
	b.SetBytes(int64(len(msg)))
	for i := 0; i < b.N; i++ {
		h.RegularWriter(nekomimi.INFO, func(w io.StringWriter) {
			w.WriteString(msg)
		})
	}
}

// —– rotation every 100 entries, 20000 entries per b.N iteration ——

const rotateInterval = 100
const totalEntries = 20000

func BenchmarkFile_Write_Rotate100(b *testing.B) {
	dir := tempDir(b)
	defer os.RemoveAll(dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h, err := filerotate.New(ctx, filerotate.Config{
		Path: dir, FilePrefix: "bench", MaxFileItems: rotateInterval,
	})
	if err != nil {
		b.Fatal(err)
	}

	header := benchHeader
	msg := smallMsg
	// each inner loop of totalEntries writes includes 200 rotations;
	// the framework runs b.N outer iterations for stable measurement
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < totalEntries; j++ {
			h.RegularLog(nekomimi.INFO, header, msg)
		}
	}
}

// —– rotation + background archive gzip ——————————————————————————
//
// Write-path timing is separated from archive gzip via StopTimer /
// ReportMetric.  ns/op is the per-log write cost (including
// amortised rotation).  archive-gzip-ns-per-file is the background
// compression time for each rotated archive, measured after all
// writes complete.

func BenchmarkFile_Write_Rotate100_ArchiveGzip(b *testing.B) {
	dir := tempDir(b)
	defer os.RemoveAll(dir)
	ctx, cancel := context.WithCancel(context.Background())

	h, err := filerotate.New(ctx, filerotate.Config{
		Path:         dir,
		FilePrefix:   "bench",
		MaxFileItems: rotateInterval,
		Compress:     true,
	})
	if err != nil {
		b.Fatal(err)
	}

	header := benchHeader
	msg := smallMsg
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.RegularLog(nekomimi.INFO, header, msg)
	}
	b.StopTimer()

	// all writes done — shutdown triggers compressWG.Wait, which
	// blocks until every background gzip goroutine finishes.
	// Most gzip work overlaps with writes (async); this metric
	// captures only the residual time needed to finish stragglers.
	gzipStart := time.Now()
	cancel()
	if err := waitForGzip(dir, "bench", 30*time.Second); err != nil {
		b.Fatal(err)
	}
	gzipResidual := time.Since(gzipStart)
	// number of rotations = b.N / rotateInterval (floor division)
	b.ReportMetric(
		float64(gzipResidual.Nanoseconds())/
			float64(b.N/rotateInterval),
		"archive-gzip-residual-ns-per-file")
}

// —– pure gzip baseline ——————————————————————————————————————————
//
// Compresses one archive-sized file (~50 KB, typical for 100 log
// entries) to measure raw gzip cost independently.

const archiveFileSize = 50 * 1024

func BenchmarkFile_GzipOneArchive(b *testing.B) {
	dir := tempDir(b)
	defer os.RemoveAll(dir)

	src, err := os.CreateTemp(dir, "archive-*.log")
	if err != nil {
		b.Fatal(err)
	}
	data := make([]byte, archiveFileSize)
	for i := range data {
		data[i] = byte('A' + i%26)
	}
	if _, err := src.Write(data); err != nil {
		b.Fatal(err)
	}
	src.Close()
	srcPath := src.Name()

	b.ResetTimer()
	b.SetBytes(archiveFileSize)
	for i := 0; i < b.N; i++ {
		in, _ := os.Open(srcPath)
		outPath := srcPath + ".gz"
		out, _ := os.Create(outPath)
		gw := gzip.NewWriter(out)
		io.Copy(gw, in)
		gw.Close()
		out.Close()
		in.Close()
		os.Remove(outPath)
	}
}
