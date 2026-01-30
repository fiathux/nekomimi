// Package nekomimi provides a very light-weight and simple logging module for golang
package nekomimi

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// LogLevel represents the severity level of a log message
type LogLevel int

const (
	// DEBUG level for detailed debugging information
	DEBUG LogLevel = iota
	// INFO level for general informational messages
	INFO
	// WARN level for warning messages
	WARN
	// ERROR level for error messages
	ERROR
)

// String returns the string representation of the log level
func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Logger represents a simple logger instance
type Logger struct {
	mu         sync.RWMutex
	level      LogLevel
	output     io.Writer
	prefix     string
	timeFormat string
}

// New creates a new Logger instance with default settings
func New() *Logger {
	return &Logger{
		level:      INFO,
		output:     os.Stdout,
		prefix:     "",
		timeFormat: "2006-01-02 15:04:05",
	}
}

// SetLevel sets the minimum log level
func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// SetOutput sets the output destination
func (l *Logger) SetOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.output = w
}

// SetPrefix sets the prefix for log messages
func (l *Logger) SetPrefix(prefix string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prefix = prefix
}

// SetTimeFormat sets the time format for log messages
func (l *Logger) SetTimeFormat(format string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.timeFormat = format
}

// log is the internal logging function
func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	l.mu.RLock()
	currentLevel := l.level
	l.mu.RUnlock()

	if level < currentLevel {
		return
	}

	l.mu.RLock()
	timestamp := time.Now().Format(l.timeFormat)
	message := fmt.Sprintf(format, args...)
	
	var logLine string
	if l.prefix != "" {
		logLine = fmt.Sprintf("[%s] [%s] [%s] %s\n", timestamp, level.String(), l.prefix, message)
	} else {
		logLine = fmt.Sprintf("[%s] [%s] %s\n", timestamp, level.String(), message)
	}

	output := l.output
	l.mu.RUnlock()

	output.Write([]byte(logLine))
}

// Debug logs a debug message
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(DEBUG, format, args...)
}

// Info logs an info message
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(INFO, format, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(WARN, format, args...)
}

// Error logs an error message
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ERROR, format, args...)
}

// Default logger instance
var defaultLogger = New()

// Debug logs a debug message using the default logger
func Debug(format string, args ...interface{}) {
	defaultLogger.Debug(format, args...)
}

// Info logs an info message using the default logger
func Info(format string, args ...interface{}) {
	defaultLogger.Info(format, args...)
}

// Warn logs a warning message using the default logger
func Warn(format string, args ...interface{}) {
	defaultLogger.Warn(format, args...)
}

// Error logs an error message using the default logger
func Error(format string, args ...interface{}) {
	defaultLogger.Error(format, args...)
}

// SetLevel sets the minimum log level for the default logger
func SetLevel(level LogLevel) {
	defaultLogger.SetLevel(level)
}

// SetOutput sets the output destination for the default logger
func SetOutput(w io.Writer) {
	defaultLogger.SetOutput(w)
}

// SetPrefix sets the prefix for the default logger
func SetPrefix(prefix string) {
	defaultLogger.SetPrefix(prefix)
}

// SetTimeFormat sets the time format for the default logger
func SetTimeFormat(format string) {
	defaultLogger.SetTimeFormat(format)
}

// Fatal logs an error message and exits the program
func Fatal(format string, args ...interface{}) {
	defaultLogger.Error(format, args...)
	os.Exit(1)
}
