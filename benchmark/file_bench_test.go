// Package benchmark_test contains performance benchmarks for nekomimi
// handler implementations.
package benchmark_test

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/fiathux/nekomimi"
	"github.com/fiathux/nekomimi/handlers/filerotate"
)

// benchDir returns a temporary directory for benchmarks and registers
// cleanup.  Setup time is excluded from benchmark results via b.ResetTimer.
func benchDir(b *testing.B) string {
	dir, err := os.MkdirTemp("", "nekomimi-bench-*")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// baseline message sizes
const (
	tinyMsg  = "hello"
	smallMsg = "this is a typical log message with some context"
)

var largeMsg = strings.Repeat(
	"a much larger message that simulates logging structured "+
		"data or stack traces in production environments. ", 30)

// --- Sequential RegularLog (no rotation) ---

func BenchmarkFile_RegularLog_NoRotation(b *testing.B) {
	dir := benchDir(b)
	ctx := context.Background()

	h, err := filerotate.New(ctx, filerotate.Config{
		Path:       dir,
		FilePrefix: "bench",
	})
	if err != nil {
		b.Fatal(err)
	}

	header := "2026-06-27 10:00:00.000 [INFO], bench - "
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		h.RegularLog(nekomimi.INFO, header, tinyMsg)
	}
}

// --- Sequential RegularLog with different message sizes ---

func BenchmarkFile_RegularLog_Size(b *testing.B) {
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
			dir := benchDir(b)
			ctx := context.Background()

			h, err := filerotate.New(ctx, filerotate.Config{
				Path:       dir,
				FilePrefix: "bench",
			})
			if err != nil {
				b.Fatal(err)
			}

			header := "2026-06-27 10:00:00.000 [INFO], bench - "
			b.ResetTimer()
			b.SetBytes(int64(len(sz.msg)))

			for i := 0; i < b.N; i++ {
				h.RegularLog(nekomimi.INFO, header, sz.msg)
			}
		})
	}
}

// --- Concurrent RegularLog ---

func BenchmarkFile_RegularLog_Parallel(b *testing.B) {
	dir := benchDir(b)
	ctx := context.Background()

	h, err := filerotate.New(ctx, filerotate.Config{
		Path:       dir,
		FilePrefix: "bench",
	})
	if err != nil {
		b.Fatal(err)
	}

	header := "2026-06-27 10:00:00.000 [INFO], bench - "
	msg := smallMsg
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			h.RegularLog(nekomimi.INFO, header, msg)
		}
	})
}

// --- RegularWriter (raw writer path) ---

func BenchmarkFile_RegularWriter(b *testing.B) {
	dir := benchDir(b)
	ctx := context.Background()

	h, err := filerotate.New(ctx, filerotate.Config{
		Path:       dir,
		FilePrefix: "bench",
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

// --- With rotation enabled (covers rotation check overhead) ---

func BenchmarkFile_RegularLog_WithRotationCheck(b *testing.B) {
	dir := benchDir(b)
	ctx := context.Background()

	h, err := filerotate.New(ctx, filerotate.Config{
		Path:         dir,
		FilePrefix:   "bench",
		MaxFileItems: 100000, // large enough not to trigger actual rotate
		MaxFileSize:  1048576, // 1 GB, large enough not to trigger
	})
	if err != nil {
		b.Fatal(err)
	}

	header := "2026-06-27 10:00:00.000 [INFO], bench - "
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		h.RegularLog(nekomimi.INFO, header, smallMsg)
	}
}

// --- With compression enabled ---

func BenchmarkFile_RegularLog_WithCompress(b *testing.B) {
	dir := benchDir(b)
	ctx := context.Background()

	h, err := filerotate.New(ctx, filerotate.Config{
		Path:        dir,
		FilePrefix:  "bench",
		MaxFileSize: 1, // trigger rotation immediately
		Compress:    true,
	})
	if err != nil {
		b.Fatal(err)
	}

	header := "2026-06-27 10:00:00.000 [INFO], bench - "
	// large enough to trigger rotation on first write
	bigMsg := strings.Repeat("x", 2048)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		h.RegularLog(nekomimi.INFO, header, bigMsg)
	}
}
