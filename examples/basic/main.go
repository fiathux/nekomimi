package main

import (
	"fmt"
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

	// Example 6: Custom log handler
	customHandlerExample()
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

// customHandlerExample demonstrates using a custom log handler
func customHandlerExample() {
	println("\n=== Custom Log Handler Example ===")

	// Create a custom log handler that writes to a file
	file, err := os.Create("/tmp/app.log")
	if err != nil {
		println("Failed to create log file:", err.Error())
		return
	}
	defer file.Close()

	customHandler := &nekomimi.LogHandlerFunc{
		RegularLogFunc: func(level nekomimi.LogLevel, header string, message ...any) {
			// Write to file
			file.WriteString(header)
			file.WriteString(fmt.Sprintln(message...))
			// Also print to stdout for demo purposes
			fmt.Print(header)
			fmt.Println(message...)
		},
	}

	logger := nekomimi.New("FileLogger", nekomimi.LogConfig{
		Handler: customHandler,
		Level:   nekomimi.INFO,
	})

	logger.Inf("This message goes to the file")
	logger.War("Warning logged to file")
	println("Check /tmp/app.log for the log output")
}
