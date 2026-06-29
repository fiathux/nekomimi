// Example: building custom log handlers.
//
// Demonstrates TinyLogHandlerFunc for minimal cases and LogHandlerFunc
// for full control (locking, panic/fatal finalisers, wrapper chaining).
package main

import (
	"io"
	"os"
	"sync"

	"github.com/fiathux/nekomimi"
)

func ExampleTinyHandler() {
	// TinyLogHandlerFunc — a single function that receives pre-formatted
	// content via a pnt closure.  Panic/Fatal are treated as regular
	// logs (no crash).  Suitable as an inner/wrapped handler.
	//
	// NOT thread-safe by itself — rely on the outer handler for locking.
	h := nekomimi.TinyLogHandlerFunc(
		func(lv nekomimi.LogLevel, pnt func(io.StringWriter)) {
			pnt(os.Stdout)
		},
	)

	logger := nekomimi.New("tiny", nekomimi.LogConfig{Handler: h})
	logger.Inf("logged via TinyLogHandlerFunc")
}

func ExampleFullHandler() {
	// LogHandlerFunc — five configurable fields for complete control.
	h := &nekomimi.LogHandlerFunc{

		// optional: provide a mutex for thread-safe concurrent writes
		Lock: &sync.Mutex{},

		// optional: custom message body formatter
		// omit to use the default (header + " " + body)
		Converter: nil,

		// required: regular log output — the pnt closure writes the
		// formatted header+body to the provided io.StringWriter
		RegularLogFunc: func(lv nekomimi.LogLevel, pnt func(io.StringWriter)) {
			pnt(os.Stdout)
		},

		// PanicLogFunc returns a "finaliser" closure.
		// Step 1 — write the panic message.
		// Step 2 — return func() { ... } to be called AFTER the lock is
		//          released.  This prevents the mutex from being held
		//          during panic().
		PanicLogFunc: func(pnt func(io.StringWriter), info string) func() {
			pnt(os.Stderr)
			return func() { panic(info) }
		},

		// FatalLogFunc — same pattern as PanicLogFunc.
		// The finaliser calls os.Exit instead of panic.
		FatalLogFunc: func(pnt func(io.StringWriter)) func() {
			pnt(os.Stderr)
			return func() { os.Exit(1) }
		},

		// optional: chain another handler.  All log calls are forwarded
		// to Wrapper.RegularWriter() BEFORE this handler's own functions
		// run.  Panic/Fatal are forwarded as RegularWriter(PANIC, ...).
		Wrapper: nil,
	}

	logger := nekomimi.New("full", nekomimi.LogConfig{Handler: h})
	logger.Inf("logged via LogHandlerFunc")
}

func main() {
	ExampleTinyHandler()
	ExampleFullHandler()
}
