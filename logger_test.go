package nekomimi

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestLogLevelString(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected string
	}{
		{DEBUG, "DEBUG"},
		{INFO, "INFO"},
		{WARN, "WARN"},
		{ERROR, "ERROR"},
	}

	for _, tt := range tests {
		if got := tt.level.String(); got != tt.expected {
			t.Errorf("LogLevel.String() = %v, want %v", got, tt.expected)
		}
	}
}

func TestNew(t *testing.T) {
	logger := New()
	if logger == nil {
		t.Fatal("New() returned nil")
	}
	if logger.level != INFO {
		t.Errorf("Default level = %v, want %v", logger.level, INFO)
	}
}

func TestLogger_SetLevel(t *testing.T) {
	logger := New()
	logger.SetLevel(DEBUG)
	if logger.level != DEBUG {
		t.Errorf("SetLevel() level = %v, want %v", logger.level, DEBUG)
	}
}

func TestLogger_SetPrefix(t *testing.T) {
	logger := New()
	prefix := "TEST"
	logger.SetPrefix(prefix)
	if logger.prefix != prefix {
		t.Errorf("SetPrefix() prefix = %v, want %v", logger.prefix, prefix)
	}
}

func TestLogger_Debug(t *testing.T) {
	var buf bytes.Buffer
	logger := New()
	logger.SetOutput(&buf)
	logger.SetLevel(DEBUG)
	
	logger.Debug("test debug message")
	
	output := buf.String()
	if !strings.Contains(output, "[DEBUG]") {
		t.Errorf("Debug() output = %v, should contain [DEBUG]", output)
	}
	if !strings.Contains(output, "test debug message") {
		t.Errorf("Debug() output = %v, should contain message", output)
	}
}

func TestLogger_Info(t *testing.T) {
	var buf bytes.Buffer
	logger := New()
	logger.SetOutput(&buf)
	
	logger.Info("test info message")
	
	output := buf.String()
	if !strings.Contains(output, "[INFO]") {
		t.Errorf("Info() output = %v, should contain [INFO]", output)
	}
	if !strings.Contains(output, "test info message") {
		t.Errorf("Info() output = %v, should contain message", output)
	}
}

func TestLogger_Warn(t *testing.T) {
	var buf bytes.Buffer
	logger := New()
	logger.SetOutput(&buf)
	
	logger.Warn("test warning message")
	
	output := buf.String()
	if !strings.Contains(output, "[WARN]") {
		t.Errorf("Warn() output = %v, should contain [WARN]", output)
	}
	if !strings.Contains(output, "test warning message") {
		t.Errorf("Warn() output = %v, should contain message", output)
	}
}

func TestLogger_Error(t *testing.T) {
	var buf bytes.Buffer
	logger := New()
	logger.SetOutput(&buf)
	
	logger.Error("test error message")
	
	output := buf.String()
	if !strings.Contains(output, "[ERROR]") {
		t.Errorf("Error() output = %v, should contain [ERROR]", output)
	}
	if !strings.Contains(output, "test error message") {
		t.Errorf("Error() output = %v, should contain message", output)
	}
}

func TestLogger_LogLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	logger := New()
	logger.SetOutput(&buf)
	logger.SetLevel(WARN)
	
	// These should not appear in output
	logger.Debug("debug message")
	logger.Info("info message")
	
	// These should appear
	logger.Warn("warn message")
	logger.Error("error message")
	
	output := buf.String()
	if strings.Contains(output, "debug message") {
		t.Error("Debug message should be filtered out")
	}
	if strings.Contains(output, "info message") {
		t.Error("Info message should be filtered out")
	}
	if !strings.Contains(output, "warn message") {
		t.Error("Warn message should be present")
	}
	if !strings.Contains(output, "error message") {
		t.Error("Error message should be present")
	}
}

func TestLogger_WithPrefix(t *testing.T) {
	var buf bytes.Buffer
	logger := New()
	logger.SetOutput(&buf)
	logger.SetPrefix("MyApp")
	
	logger.Info("test message")
	
	output := buf.String()
	if !strings.Contains(output, "[MyApp]") {
		t.Errorf("Output should contain prefix [MyApp], got: %v", output)
	}
}

func TestDefaultLogger(t *testing.T) {
	var buf bytes.Buffer
	SetOutput(&buf)
	SetLevel(INFO)
	
	Info("default logger test")
	
	output := buf.String()
	if !strings.Contains(output, "[INFO]") {
		t.Errorf("Default logger output = %v, should contain [INFO]", output)
	}
	if !strings.Contains(output, "default logger test") {
		t.Errorf("Default logger output = %v, should contain message", output)
	}
}

func TestLogger_SetTimeFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := New()
	logger.SetOutput(&buf)
	logger.SetTimeFormat("2006/01/02")
	
	logger.Info("test message")
	
	output := buf.String()
	// Check that output contains date in the format YYYY/MM/DD
	if !strings.Contains(output, "/") {
		t.Errorf("Output should contain custom time format with /, got: %v", output)
	}
}

func TestLogger_Concurrency(t *testing.T) {
	logger := New()
	// Use io.Discard to avoid race on shared buffer
	logger.SetOutput(io.Discard)
	
	// Test concurrent logging
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(n int) {
			logger.Info("Message %d", n)
			done <- true
		}(i)
	}
	
	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
	
	// If we got here without panicking, the test passes
}
