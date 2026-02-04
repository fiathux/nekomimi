package nekomimi

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// sysTerminateCode is the exit code used when FatalLog is called
const sysTerminateCode = 1

// sysTerminate is the function called to terminate the program
var sysTerminate = func() {
	os.Exit(sysTerminateCode)
}

// LogHandler represents the interface for handling log messages
// Panic or Fatal log is supported. It's allowed output log message and raise
// panic or terminate the program after logging. which like the standard log.
// the log handler should implement these features by itself.
type LogHandler interface {
	// RegularLog handles regular log messages with a specified log level
	RegularLog(level LogLevel, header string, message ...any)
	// RegularWriter is low-level log writer for regular log messages. which
	// not care about body formatting of log message, only provide a StringWriter
	// to write log content.
	// Panic and Fatal levels also possibly go through here when the handler is
	// set as a wrapper.
	RegularWriter(level LogLevel, pnt func(io.StringWriter))
	// PanicLog handles panic-level log messages.
	// will automatically occur a panic after logging
	PanicLog(header string, message ...any)
	// FatalLog handles fatal-level log messages
	// will automatically terminate the program after logging
	FatalLog(header string, message ...any)
}

// LogHandlerFunc is a function-based implementation of the LogHandler interface
type LogHandlerFunc struct {
	// optional lock for concurrent access. If nil, no locking is performed
	// If any changes to the LogHandlerFunc fields are made at runtime, a lock
	// should be provided to ensure thread safety.
	Lock sync.Locker
	// optional converter function allowing custom formatting message body. If
	// nil, the default formatting is used.
	// the parameters `origin` is the default body formatter function.
	Converter func(
		origin func(header string, message ...any) func(io.StringWriter),
		header string,
		message ...any,
	) func(io.StringWriter)
	// regular log function
	RegularLogFunc func(level LogLevel, pnt func(io.StringWriter))
	// should return a finalizer function that will be called after logging to
	// raise panic
	PanicLogFunc func(pnt func(io.StringWriter), info string) (fin func())
	// should return a finalizer function that will be called after logging to
	// terminate the program
	FatalLogFunc func(func(io.StringWriter)) (fin func())
	// optional wrapper LogHandler to chain calls
	Wrapper LogHandler
}

// TinyLogHandlerFunc is a minimal implementation of LogHandler using a single
// function
type TinyLogHandlerFunc func(level LogLevel, pnt func(io.StringWriter))

// NewNativeLogHandler creates a new LogHandler that uses std I/O for logging
func NewNativeLogHandler(warp LogHandler) LogHandler {
	return &LogHandlerFunc{
		Lock: &sync.Mutex{},
		RegularLogFunc: func(level LogLevel, pnt func(io.StringWriter)) {
			pnt(os.Stdout)
		},
		PanicLogFunc: func(pnt func(io.StringWriter), info string) func() {
			pnt(os.Stderr)
			return func() {
				panic(info)
			}
		},
		FatalLogFunc: func(pnt func(io.StringWriter)) func() {
			pnt(os.Stderr)
			return sysTerminate
		},
		Wrapper: warp,
	}
}

// NativeLogHandler uses the standard log package for logging
var NativeLogHandler LogHandler = NewNativeLogHandler(nil)

// NewFileAccessorLogHandler creates a new LogHandler that writes logs to a
// file. it's a very basic implementation and designed for wrapping around
// other LogHandlers.
// This handler is not thread-safe by itself. Should ensure parent handler
// have thread-safety if needed.
// ctx is the context for file lifecycle management.
func NewFileAccessorLogHandler(
	ctx context.Context, path string,
) (LogHandler, error) {
	countwrt := atomic.Uint64{}
	var lastflush uint64 = 0
	fplock := &sync.RWMutex{}
	fp, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}

	// flush file
	flush := func() {
		fplock.RLock()
		defer fplock.RUnlock()
		if fp == nil {
			return
		}
		c := countwrt.Load()
		if c == lastflush {
			return
		}
		lastflush = c
		fp.Sync()
	}

	// tiny log handler function
	handler := func(level LogLevel, pnt func(io.StringWriter)) {
		fplock.RLock()
		defer fplock.RUnlock()
		if fp == nil {
			return
		}
		pnt(fp)
		countwrt.Add(1)
	}

	// file holder thread
	go func() {
		for {
			select {
			case <-ctx.Done():
				func() { // final flush and close
					fplock.Lock()
					defer fplock.Unlock()
					fp.Close()
					fp = nil
				}()
				return
			case <-time.After(2 * time.Second):
				flush() // periodic flush
			}
		}
	}()

	return TinyLogHandlerFunc(handler), nil
}

// ------- implement LogHandler interface for LogHandlerFunc -------

// rawWriteLogFunc provide a default method to formats the message body and writes
// it using the provided i/o writer
func (lh *LogHandlerFunc) rawWriteLogFunc(
	header string, message ...any,
) func(io.StringWriter) {
	sp := fmt.Sprintln(message...)
	return func(w io.StringWriter) {
		w.WriteString(header)
		w.WriteString(sp)
	}
}

// writeLogFunc applies the converter if available, otherwise uses the raw
// write function
func (lh *LogHandlerFunc) writeLogFunc(
	header string, message ...any,
) func(io.StringWriter) {
	if lh.Converter != nil {
		return lh.Converter(lh.rawWriteLogFunc, header, message...)
	}
	return lh.rawWriteLogFunc(header, message...)
}

func (lh *LogHandlerFunc) RegularWriter(
	level LogLevel, pnt func(io.StringWriter),
) {
	if lh.Lock != nil {
		lh.Lock.Lock()
		defer lh.Lock.Unlock()
	}
	if lh.Wrapper != nil {
		lh.Wrapper.RegularWriter(level, pnt)
	}
	if lh.RegularLogFunc != nil {
		lh.RegularLogFunc(level, pnt)
	}
}

func (lh *LogHandlerFunc) RegularLog(
	level LogLevel, header string, message ...any,
) {
	if lh.Lock != nil {
		lh.Lock.Lock()
		defer lh.Lock.Unlock()
	}
	pnt := lh.writeLogFunc(header, message...)
	if lh.Wrapper != nil {
		lh.Wrapper.RegularWriter(level, pnt)
	}
	if lh.RegularLogFunc != nil {
		lh.RegularLogFunc(level, pnt)
	}
}

func (lh *LogHandlerFunc) PanicLog(header string, message ...any) {
	fin := func() func() {
		if lh.Lock != nil {
			lh.Lock.Lock()
			defer lh.Lock.Unlock()
		}
		pnt := lh.writeLogFunc(header, message...)
		if lh.Wrapper != nil {
			lh.Wrapper.RegularWriter(PANIC, pnt)
		}
		if lh.PanicLogFunc != nil {
			return lh.PanicLogFunc(pnt, fmt.Sprintln(message...))
		}
		return nil
	}()
	if fin != nil {
		fin()
	}
}

func (lh *LogHandlerFunc) FatalLog(header string, message ...any) {
	fin := func() func() {
		if lh.Lock != nil {
			lh.Lock.Lock()
			defer lh.Lock.Unlock()
		}
		pnt := lh.writeLogFunc(header, message...)
		if lh.Wrapper != nil {
			lh.Wrapper.RegularWriter(FATAL, pnt)
		}
		if lh.FatalLogFunc != nil {
			return lh.FatalLogFunc(pnt)
		}
		return nil
	}()
	if fin != nil {
		fin()
	}
}

// --------------------------------------------------------------

// ------- implement TinyLogHandlerFunc interface for func -------

func (lf TinyLogHandlerFunc) writeLogFunc(
	header string, message ...any,
) func(io.StringWriter) {
	sp := fmt.Sprintln(message...)
	return func(w io.StringWriter) {
		w.WriteString(header)
		w.WriteString(sp)
	}
}

func (lf TinyLogHandlerFunc) RegularWriter(
	level LogLevel, pnt func(io.StringWriter),
) {
	lf(level, pnt)
}

func (lf TinyLogHandlerFunc) RegularLog(
	level LogLevel, header string, message ...any,
) {
	pnt := lf.writeLogFunc(header, message...)
	lf(level, pnt)
}

func (lf TinyLogHandlerFunc) PanicLog(header string, message ...any) {
	pnt := lf.writeLogFunc(header, message...)
	lf(PANIC, pnt)
}

func (lf TinyLogHandlerFunc) FatalLog(header string, message ...any) {
	pnt := lf.writeLogFunc(header, message...)
	lf(FATAL, pnt)
}

// --------------------------------------------------------------
