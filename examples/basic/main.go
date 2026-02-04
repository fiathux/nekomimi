package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/fiathux/nekomimi"
)

func main() {
	// Example 1: Basic logger with default configuration
	basicExample()

	// Example 2: Logger with custom configuration
	customConfigExample()

	// Example 3: Different log methods (simple, formatted, deferred)
	logMethodsExample()

	// Example 4: Trace logging
	traceExample()

	// Example 5: Derived loggers
	derivedExample()

	// Example 6: Custom log handler (legacy approach)
	customHandlerExample()

	// Example 7: File logging with built-in handler
	fileLoggingExample()

	// Example 8: Handler composition
	handlerCompositionExample()
}

// basicExample demonstrates basic logger usage
func basicExample() {
	println("\n=== Basic Logger Example ===")
	logger := nekomimi.New("BasicApp", nekomimi.LogConfig{})

	logger.Dbg("This is a debug message")
	logger.Inf("Application started successfully")
	logger.War("This is a warning message")
	logger.Err("This is an error message")
}

// customConfigExample shows how to create a logger with custom configuration
func customConfigExample() {
	println("\n=== Custom Configuration Example ===")
	logger := nekomimi.New("CustomApp", nekomimi.LogConfig{
		Level:          nekomimi.INFO, // Only INFO and above will be logged
		LevelWithTrace: nekomimi.WARN, // Show call trace for WARN and above
		TimeFormat:     "15:04:05.000",
	})

	logger.Dbg("This debug message will NOT be shown")
	logger.Inf("This info message will be shown")
	logger.War("This warning will include call trace info")
	logger.Err("This error will include call trace info")
}

// logMethodsExample demonstrates the three types of logging methods
func logMethodsExample() {
	println("\n=== Log Methods Example ===")
	logger := nekomimi.New("Methods", nekomimi.LogConfig{
		Level: nekomimi.DEBUG,
	})

	// Simple message logging (fastest)
	logger.Inf("Simple message", "with", "multiple", "parts")

	// Formatted message logging
	name := "Alice"
	age := 30
	logger.Inff("User %s is %d years old", name, age)

	// Deferred message logging (useful for expensive operations)
	// The function is only called if the log level is enabled
	if logFunc := logger.DbgP(); logFunc != nil {
		// This expensive operation only runs if DEBUG is enabled
		expensiveData := "expensive computation result"
		logFunc("Deferred debug:", expensiveData)
	}

	// Demonstration of deferred logging efficiency
	logger.SetLevel(nekomimi.ERROR) // Disable DEBUG and INFO
	if logFunc := logger.InfP(); logFunc == nil {
		println("INFO level is disabled, deferred function not called")
	}
}

// traceExample demonstrates trace logging for request tracking
func traceExample() {
	println("\n=== Trace Logging Example ===")
	logger := nekomimi.New("TraceApp", nekomimi.LogConfig{
		Level: nekomimi.DEBUG,
	})

	// Create a trace logger for tracking a specific operation/request
	trace := logger.Trace("RequestHandler")
	trace.Inf("Processing request")
	trace.Dbgf("Request ID: %s", trace.TraceID())
	trace.Inf("Request processed successfully")

	// Another trace for a different operation
	trace2 := logger.Trace("DatabaseQuery")
	trace2.Dbg("Connecting to database")
	trace2.Inf("Query executed")
}

// derivedExample shows how to create derived loggers with different prefixes
func derivedExample() {
	println("\n=== Derived Logger Example ===")
	mainLogger := nekomimi.New("App", nekomimi.LogConfig{
		Level: nekomimi.DEBUG,
	})

	// Create derived loggers for different components
	dbLogger := mainLogger.Derive("Database")
	apiLogger := mainLogger.Derive("API")
	cacheLogger := mainLogger.Derive("Cache")

	dbLogger.Inf("Database connection established")
	apiLogger.Inf("API server started on port 8080")
	cacheLogger.Inf("Cache initialized")

	// Derived loggers can have their own log levels
	dbLogger.SetLevel(nekomimi.WARN)
	dbLogger.Dbg("This debug message will NOT be shown")
	dbLogger.War("Database slow query detected")
}

// customHandlerExample demonstrates using a custom log handler (legacy approach)
func customHandlerExample() {
	println("\n=== Custom Log Handler Example (Legacy) ===")

	// Create a custom log handler using the old-style function signature
	customHandler := &nekomimi.LogHandlerFunc{
		RegularLogFunc: func(level nekomimi.LogLevel, pnt func(io.StringWriter)) {
			// Write to stdout
			pnt(os.Stdout)
		},
	}

	logger := nekomimi.New("CustomLogger", nekomimi.LogConfig{
		Handler: customHandler,
		Level:   nekomimi.INFO,
	})

	logger.Inf("This message uses a custom handler")
	logger.War("Warning with custom handler")
}

// fileLoggingExample demonstrates using the built-in file handler
func fileLoggingExample() {
	println("\n=== File Logging Example ===")

	// Create a context for file lifecycle management
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a file handler that writes to a log file
	fileHandler, err := nekomimi.NewFileAccessorLogHandler(ctx, "/tmp/nekomimi-app.log")
	if err != nil {
		println("Failed to create file handler:", err.Error())
		return
	}

	// Wrap the file handler with the native handler for dual output
	logger := nekomimi.New("FileApp", nekomimi.LogConfig{
		Handler: nekomimi.NewNativeLogHandler(fileHandler),
		Level:   nekomimi.DEBUG,
	})

	logger.Inf("This message goes to both console and file")
	logger.Dbg("Debug message logged to file")
	logger.War("Warning message with dual output")

	println("Log file created at /tmp/nekomimi-app.log")
	println("The file is automatically flushed every 2 seconds")
}

// handlerCompositionExample demonstrates handler chaining and composition
func handlerCompositionExample() {
	println("\n=== Handler Composition Example ===")

	// Create a context for file lifecycle management
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create file handler
	fileHandler, err := nekomimi.NewFileAccessorLogHandler(ctx, "/tmp/nekomimi-composed.log")
	if err != nil {
		println("Failed to create file handler:", err.Error())
		return
	}

	// Create a custom handler that adds prefix and chains to file handler
	prefixHandler := &nekomimi.LogHandlerFunc{
		RegularLogFunc: func(level nekomimi.LogLevel, pnt func(io.StringWriter)) {
			// Add custom prefix to console output
			fmt.Print("[CUSTOM PREFIX] ")
			pnt(os.Stdout)
		},
		Wrapper: fileHandler, // Chain to file handler
	}

	logger := nekomimi.New("ComposedApp", nekomimi.LogConfig{
		Handler: prefixHandler,
		Level:   nekomimi.INFO,
	})

	logger.Inf("This message goes through the composed handler chain")
	logger.War("Warning message with custom prefix and file logging")

	println("Logs are written to console with prefix AND to /tmp/nekomimi-composed.log")
}
