// Example: advanced file rotation log handler
//
// Demonstrates the filerotate handler with automatic rotation by file size,
// entry count, and TTL; gzip compression; archive management; and how it
// composes with the native stdout handler for dual output.
package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fiathux/nekomimi"
	"github.com/fiathux/nekomimi/handlers/filerotate"
)

func main() {
	// Create a context that cancels on SIGINT/SIGTERM for graceful shutdown.
	ctx, cancel := signal.NotifyContext(
		context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Create the file rotation handler.
	// Rotation triggers:
	//   MaxFileSize   — 1 KB  (demo: rotate after ~1 KB of data)
	//   MaxFileItems  — 10    (demo: rotate after 10 entries)
	//   MaxFileTTL    — 1 min (demo: rotate if file is older than 1 min)
	fileHandler, err := filerotate.New(ctx, filerotate.Config{
		Path:         "/tmp/nekomimi-filerotate",
		FilePrefix:   "app",
		MaxFileSize:  32 * 1024, // 1 KB
		MaxFileItems: 10000,     // 10 entries per file
		MaxFileTTL:   0,         // 1 minute
		MaxArchives:  5,         // keep latest 5 archives
		Compress:     true,      // gzip archived files
		RotatePanic:  false,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create file handler: %v\n", err)
		os.Exit(1)
	}

	startTs := time.Now()

	// Wrap with native handler for console output.
	logger := nekomimi.New("FilerotateDemo", nekomimi.LogConfig{
		Handler: fileHandler,
	})

	// Write small messages (not triggering size rotation).
	logger.Inf("file rotation demo started")
	for i := 1; i <= 3; i++ {
		logger.Dbgf("processing item %d", i)
	}

	// Write a large message to trigger size-based rotation (~2 KB).
	large := make([]byte, 2048)
	for i := range large {
		large[i] = 'A' + byte(i%26)
	}
	logger.Inff("large payload: %s", string(large))

	// Trigger item-count rotation by writing more entries.
	for i := 1; i <= 15; i++ {
		logger.Inff("batch-rotated entry %d", i)
		time.Sleep(10 * time.Millisecond) // slow down for readability
	}

	// Demonstrate Panic logging with a recover wrapper.
	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Inff("recovered from panic: %v", r)
			}
		}()
		logger.Panic("intentional panic for demonstration")
	}()

	var randbyte [100]byte
	rand.Read(randbyte[:])
	randstr := fmt.Sprintf("%x", randbyte[:])
	for i := 0; i < 200000; i++ {
		logger.Inff("flood entry %d, %s", i, string(randstr))
	}

	logger.Inf("file rotation demo finished")
	fmt.Println("\nCheck /tmp/nekomimi-filerotate for rotated log files.")
	spend := time.Since(startTs)
	fmt.Printf("Total time spent: %v\n", spend)

	time.Sleep(10 * time.Second)

	// Context cancellation triggers final flush and shutdown.
	cancel()
	time.Sleep(100 * time.Millisecond)
}
