package main

import (
	"github.com/fiathux/nekomimi"
)

func main() {
	// Example 1: Using the default logger
	nekomimi.Info("Application started")
	nekomimi.Debug("This is a debug message (won't show by default)")
	nekomimi.Warn("This is a warning message")
	nekomimi.Error("This is an error message")

	// Example 2: Change log level to show debug messages
	nekomimi.SetLevel(nekomimi.DEBUG)
	nekomimi.Debug("Now debug messages will show")

	// Example 3: Set a prefix for the default logger
	nekomimi.SetPrefix("MyApp")
	nekomimi.Info("Message with prefix")

	// Example 4: Create a custom logger instance
	customLogger := nekomimi.New()
	customLogger.SetPrefix("CustomLogger")
	customLogger.SetLevel(nekomimi.WARN)
	
	customLogger.Info("This won't show (below WARN level)")
	customLogger.Warn("This will show")
	customLogger.Error("Custom error message")

	// Example 5: Using formatted strings
	name := "Alice"
	age := 30
	nekomimi.Info("User %s is %d years old", name, age)
	
	// Example 6: Multiple logger instances
	logger1 := nekomimi.New()
	logger1.SetPrefix("Service1")
	logger1.Info("Service 1 started")
	
	logger2 := nekomimi.New()
	logger2.SetPrefix("Service2")
	logger2.Info("Service 2 started")
}
