# nekomimi

A very light-weight and simple enough log module for golang

## Installation

```bash
go get github.com/fiathux/nekomimi
```

## Features

- Simple and lightweight logging
- Multiple log levels: DEBUG, INFO, WARN, ERROR
- Customizable log prefix
- Customizable time format
- Support for multiple logger instances
- Printf-style formatting

## Quick Start

```go
package main

import (
    "github.com/fiathux/nekomimi"
)

func main() {
    // Use the default logger
    nekomimi.Info("Application started")
    nekomimi.Warn("This is a warning")
    nekomimi.Error("This is an error")
    
    // Set log level
    nekomimi.SetLevel(nekomimi.DEBUG)
    nekomimi.Debug("Debug messages now visible")
    
    // Add a prefix
    nekomimi.SetPrefix("MyApp")
    nekomimi.Info("Message with prefix")
}
```

## Usage Examples

### Basic Logging

```go
nekomimi.Debug("Debug message")
nekomimi.Info("Info message")
nekomimi.Warn("Warning message")
nekomimi.Error("Error message")
```

### Custom Logger Instance

```go
logger := nekomimi.New()
logger.SetPrefix("Service")
logger.SetLevel(nekomimi.INFO)
logger.Info("Custom logger message")
```

### Log Levels

```go
// Set minimum log level (only messages at this level or higher will be shown)
nekomimi.SetLevel(nekomimi.DEBUG) // Shows all messages
nekomimi.SetLevel(nekomimi.INFO)  // Shows INFO, WARN, ERROR
nekomimi.SetLevel(nekomimi.WARN)  // Shows WARN, ERROR
nekomimi.SetLevel(nekomimi.ERROR) // Shows only ERROR
```

### Formatted Messages

```go
name := "Alice"
age := 30
nekomimi.Info("User %s is %d years old", name, age)
```

### Custom Output

```go
file, _ := os.Create("app.log")
nekomimi.SetOutput(file)
nekomimi.Info("This goes to the file")
```

## API Reference

### Logger Methods

- `New()` - Create a new logger instance
- `SetLevel(level LogLevel)` - Set the minimum log level
- `SetOutput(w io.Writer)` - Set the output destination
- `SetPrefix(prefix string)` - Set the log prefix
- `SetTimeFormat(format string)` - Set the time format
- `Debug(format string, args ...interface{})` - Log debug message
- `Info(format string, args ...interface{})` - Log info message
- `Warn(format string, args ...interface{})` - Log warning message
- `Error(format string, args ...interface{})` - Log error message

### Package-Level Functions

The package provides convenient functions that use a default logger:

- `Debug(format string, args ...interface{})`
- `Info(format string, args ...interface{})`
- `Warn(format string, args ...interface{})`
- `Error(format string, args ...interface{})`
- `Fatal(format string, args ...interface{})` - Log error and exit
- `SetLevel(level LogLevel)`
- `SetOutput(w io.Writer)`
- `SetPrefix(prefix string)`
- `SetTimeFormat(format string)`

## License

See LICENSE file for details
