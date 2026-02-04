# nekomimi

A very light-weight and simple logging module for golang with advanced features like trace logging, derived loggers, and customizable handlers.

## Installation

```bash
go get github.com/fiathux/nekomimi
```

## Features

- Simple and lightweight logging
- Multiple log levels: DEBUG, INFO, WARN, ERROR, PANIC, FATAL
- Three types of logging methods per level: simple, formatted, and deferred
- Trace logging for request/operation tracking with unique IDs
- Derived loggers with hierarchical prefixes
- Customizable log handlers with composition support
- Built-in file logging handler with automatic flushing
- Handler wrapping for chaining multiple handlers
- Customizable time format
- Configurable call trace level
- Thread-safe operations
- Zero-cost deferred logging (skipped if level is disabled)

## Quick Start

```go
package main

import (
	"github.com/fiathux/nekomimi"
)

func main() {
	// Create a logger with default configuration
	logger := nekomimi.New("MyApp", nekomimi.LogConfig{})
	
	// Use different log levels
	logger.Dbg("Debug message")
	logger.Inf("Application started")
	logger.War("This is a warning")
	logger.Err("This is an error")
}
```

## Usage Examples

### Basic Logging

Each log level supports three types of methods:

```go
logger := nekomimi.New("App", nekomimi.LogConfig{})

// Simple message logging (fastest)
logger.Dbg("Debug message")
logger.Inf("Info message")
logger.War("Warning message")
logger.Err("Error message")

// Formatted message logging
logger.Dbgf("User %s logged in", username)
logger.Inff("Processing %d items", count)
logger.Warf("Retry attempt %d/%d", current, max)
logger.Errf("Failed to connect: %v", err)

// Deferred message logging (useful for expensive operations)
// Only executed if the log level is enabled
if logFunc := logger.DbgP(); logFunc != nil {
	expensiveData := computeExpensiveData()
	logFunc("Debug data:", expensiveData)
}
```

### Custom Logger Configuration

```go
logger := nekomimi.New("MyService", nekomimi.LogConfig{
	Level:          nekomimi.INFO,  // Minimum log level
	LevelWithTrace: nekomimi.WARN,  // Show call trace for WARN and above
	TimeFormat:     "15:04:05.000", // Custom time format
	Handler:        customHandler,   // Optional custom handler
})

logger.Dbg("This will NOT be logged (below INFO level)")
logger.Inf("This will be logged")
logger.War("This will be logged with file:line info")
```

### Trace Logging

Track operations or requests with unique trace IDs:

```go
logger := nekomimi.New("API", nekomimi.LogConfig{})

// Create a trace logger for a specific operation
trace := logger.Trace("RequestHandler")
trace.Inf("Processing request")
trace.Dbgf("Request ID: %s", trace.TraceID())
trace.Inf("Request completed")
// Output includes trace name and ID: <RequestHandler:019c2342-46d6-720c-a672-6f61f38d2f19>
```

### Derived Loggers

Create loggers with hierarchical prefixes for different components:

```go
mainLogger := nekomimi.New("App", nekomimi.LogConfig{})

// Create derived loggers for different components
dbLogger := mainLogger.Derive("Database")
apiLogger := mainLogger.Derive("API")

dbLogger.Inf("Connection established")
// Output: [INFO], App.Database - Connection established

apiLogger.Inf("Server started")
// Output: [INFO], App.API - Server started

// Derived loggers can have independent log levels
dbLogger.SetLevel(nekomimi.WARN)
```

### File Logging

Use the built-in file handler for automatic file logging with periodic flushing:

```go
import (
	"context"
	"github.com/fiathux/nekomimi"
)

// Create a context for file lifecycle management
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

// Create a file handler
fileHandler, err := nekomimi.NewFileAccessorLogHandler(ctx, "app.log")
if err != nil {
	panic(err)
}

// Wrap the file handler with the native handler for console output
logger := nekomimi.New("App", nekomimi.LogConfig{
	Handler: nekomimi.NewNativeLogHandler(fileHandler),
})

logger.Inf("This message goes to both file and console")
// The file is automatically flushed every 2 seconds
// When context is cancelled, the file is flushed and closed
```

### Custom Log Handler

Implement custom log handling (e.g., send to external service):

```go
import "io"

customHandler := &nekomimi.LogHandlerFunc{
	RegularLogFunc: func(level nekomimi.LogLevel, pnt func(io.StringWriter)) {
		// Write to custom destination
		pnt(os.Stdout)
	},
}

logger := nekomimi.New("CustomLogger", nekomimi.LogConfig{
	Handler: customHandler,
})
```

### Handler Composition

Combine multiple handlers using the wrapper pattern:

```go
ctx := context.Background()

// Create file handler
fileHandler, _ := nekomimi.NewFileAccessorLogHandler(ctx, "app.log")

// Create custom handler that wraps file handler
customHandler := &nekomimi.LogHandlerFunc{
	RegularLogFunc: func(level nekomimi.LogLevel, pnt func(io.StringWriter)) {
		// Add custom processing here
		pnt(os.Stdout)
	},
	Warpper: fileHandler, // Chain to file handler
}

logger := nekomimi.New("ComposedLogger", nekomimi.LogConfig{
	Handler: customHandler,
})

// Logs go through custom handler and then to file
logger.Inf("This is logged to both stdout and file")
```

### Panic and Fatal Logging

```go
logger := nekomimi.New("App", nekomimi.LogConfig{})

// Panic logs the message with stack trace and then panics
logger.Panic("Critical error occurred")
logger.Panicf("Panic: %v", err)

// Fatal logs the message with stack trace and exits the program
logger.Fatal("Fatal error, exiting")
logger.Fatalf("Fatal: %v", err)
```

## API Reference

### Creating a Logger

```go
func New(name string, config LogConfig) Logger
```

Creates a new logger instance with the given name and configuration.

### LogConfig

```go
type LogConfig struct {
	Handler        LogHandler // Custom log handler (optional)
	Level          LogLevel   // Minimum log level (default: DEBUG)
	LevelWithTrace LogLevel   // Level to include call trace (default: none)
	TimeFormat     string     // Time format (default: "2006-01-02 15:04:05.000")
}
```

### Log Handler Interface

The `LogHandler` interface defines how log messages are processed and written:

```go
type LogHandler interface {
	// RegularLog handles regular log messages with a specified log level
	RegularLog(level LogLevel, header string, message ...any)
	
	// RegularWriter provides low-level access to write log content
	RegularWriter(level LogLevel, pnt func(io.StringWriter))
	
	// PanicLog handles panic-level log messages and triggers panic
	PanicLog(header string, message ...any)
	
	// FatalLog handles fatal-level log messages and terminates the program
	FatalLog(header string, message ...any)
}
```

#### Built-in Handlers

**NativeLogHandler** - Default handler writing to stdout/stderr:
```go
logger := nekomimi.New("App", nekomimi.LogConfig{
	Handler: nekomimi.NativeLogHandler,
})
```

**NewFileAccessorLogHandler** - File handler with automatic flushing:
```go
ctx := context.Background()
fileHandler, err := nekomimi.NewFileAccessorLogHandler(ctx, "app.log")
// Returns TinyLogHandlerFunc
```

**NewNativeLogHandler** - Creates a native handler with optional wrapper:
```go
// With wrapper
handler := nekomimi.NewNativeLogHandler(fileHandler)
// Without wrapper
handler := nekomimi.NewNativeLogHandler(nil)
```

#### Custom Handler Implementation

**LogHandlerFunc** - Flexible handler with optional features:
```go
type LogHandlerFunc struct {
	Lock           sync.Locker  // Optional lock for thread safety
	Converter      func(...)     // Optional message format converter
	RegularLogFunc func(...)     // Regular log function
	PanicLogFunc   func(...) func() // Panic log with finalizer
	FatalLogFunc   func(...) func() // Fatal log with finalizer
	Warpper        LogHandler    // Optional chained handler
}
```

**TinyLogHandlerFunc** - Minimal handler implementation:
```go
type TinyLogHandlerFunc func(level LogLevel, pnt func(io.StringWriter))
```

### Log Levels

```go
const (
	DEBUG  // Detailed debugging information
	INFO   // General informational messages
	WARN   // Warning messages
	ERROR  // Error messages
	PANIC  // Critical errors that cause panic
	FATAL  // Fatal errors that terminate the program
)
```

### Logger Interface

```go
type Logger interface {
	BaiscLogger
	
	// Panic/Fatal logging
	Panic(message ...any)
	Panicf(format string, args ...any)
	Fatal(message ...any)
	Fatalf(format string, args ...any)
	
	// Create a trace logger
	Trace(name string) TraceLogger
	
	// Create a derived logger
	Derive(prefix string) Logger
	
	// Configuration
	SetLevel(level LogLevel)
	SetCallTraceLevel(level LogLevel)
	SetTimeFormat(format string)
	SetLogHandler(handler LogHandler)
	WrapLogHandler(wrapper func(old LogHandler) LogHandler)
}
```

### BaiscLogger Interface

```go
type BaiscLogger interface {
	// Debug level
	Dbg(message ...any)
	Dbgf(format string, args ...any)
	DbgP() func(message ...any)
	
	// Info level
	Inf(message ...any)
	Inff(format string, args ...any)
	InfP() func(message ...any)
	
	// Warning level
	War(message ...any)
	Warf(format string, args ...any)
	WarP() func(message ...any)
	
	// Error level
	Err(message ...any)
	Errf(format string, args ...any)
	ErrP() func(message ...any)
}
```

### TraceLogger Interface

```go
type TraceLogger interface {
	BaiscLogger
	
	// Get trace information
	TraceID() string
	TraceName() string
}
```

## Examples

See the [examples/basic](examples/basic/main.go) directory for a comprehensive demo covering all features.

To run the demo:

```bash
cd examples/basic
go run main.go
```

## License

See LICENSE file for details
