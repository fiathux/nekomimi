// Example: handler composition — build chains that write to stdout,
// files, and the network simultaneously.
//
// Two patterns are shown:
//   1. filerotate (advanced rotation) as the innermost wrapped handler
//   2. FileAccessorLogHandler (basic file) as the innermost handler
//
// In both cases the innermost handler has WrapOnly=true — only the
// outermost handler (NativeLogHandler) triggers panic/exit.  Inner
// handlers only transport or persist log entries.
package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/fiathux/nekomimi"
	"github.com/fiathux/nekomimi/handlers/filerotate"
	"github.com/fiathux/nekomimi/handlers/netlog"
)

func main() {
	ctx, cancel := signal.NotifyContext(
		context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// ---- pattern A: filerotate as innermost wrapped handler ----
	// WrapOnly=true ensures this handler never triggers panic/exit.
	// Crash behaviour belongs to the outermost handler.
	rotHandler, err := filerotate.New(ctx, filerotate.Config{
		Path:         "/tmp/nekomimi-compose",
		FilePrefix:   "app",
		MaxFileSize:  10240,
		MaxFileItems: 100000,
		MaxFileTTL:   1440,
		MaxArchives:  30,
		Compress:     true,
		RotatePanic:  false,
		WrapOnly:     true, // only persist, never crash
	})
	if err != nil {
		panic("filerotate: " + err.Error())
	}

	netHandler, err := netlog.New(ctx, netlog.Config{
		Connect:  "tcp://log-collector.example.com:28280",
		WrapOnly: true, // only transport, never crash
		Wrapper:  rotHandler,
	})
	if err != nil {
		netHandler = nil
	}

	// Wrapping chain (inner → outer):
	//   rotHandler  — writes to rotating log files (WrapOnly)
	//   netHandler  — sends JSON over TCP (WrapOnly, wraps rotHandler)
	//   nativeLog   — writes to stdout (outermost, handles panic/exit)
	nativeLog := nekomimi.NewNativeLogHandler(netHandler)

	logger := nekomimi.New("Compose", nekomimi.LogConfig{
		Handler: nativeLog,
	})
	logger.Inf("chain A: stdout → network → filerotate")

	// ---- pattern B: basic FileAccessorLogHandler comparison ----
	// TinyLogHandlerFunc inherently does not trigger panic/exit,
	// making it a natural fit for innermost wrapped handlers.
	FileAccessorExample(ctx)
}

// FileAccessorExample demonstrates the simpler built-in file handler
// as a wrapped leaf node.  Unlike filerotate, it has no rotation,
// compression, or archiving — just simple append writes with periodic
// flush.
func FileAccessorExample(ctx context.Context) {
	// TinyLogHandlerFunc — no crash behaviour by design.
	fileHandler, err := nekomimi.NewFileAccessorLogHandler(
		ctx, "/tmp/nekomimi-basic.log",
	)
	if err != nil {
		panic("file accessor: " + err.Error())
	}

	// Wrap with a custom format handler + native stdout.
	customHandler := &nekomimi.LogHandlerFunc{
		RegularLogFunc: func(lv nekomimi.LogLevel, pnt func(io.StringWriter)) {
			os.Stdout.WriteString("[APP] ")
			pnt(os.Stdout)
		},
		Wrapper: fileHandler,
	}

	logger := nekomimi.New("Basic", nekomimi.LogConfig{
		Handler: customHandler,
	})
	logger.Inf("chain B: custom prefix → basic file")

	// When a FATAL or PANIC is logged, only the OUTERMOST handler
	// (LogHandlerFunc here) triggers the crash.  The fileHandler
	// (TinyLogHandlerFunc) simply writes the message and returns.
}
