package nekomimi

import (
	"fmt"
	"os"
)

// sysTeminateCode is the exit code used when FatalLog is called
const sysTeminateCode = 1

// sysTerminate is the function called to terminate the program
var sysTerminate = func() {
	os.Exit(sysTeminateCode)
}

// LogHandler represents the interface for handling log messages
// Panic or Fatal log is supported. It's allowed output log message and raise
// panic or terminate the program after logging. which like the standard log.
// the log handler should implement these features by itself.
type LogHandler interface {
	// RegularLog handles regular log messages with a specified log level
	// Panic and Fatal levels also possibly go through here when the handler is
	// set as a wrapper.
	RegularLog(level LogLevel, header string, message ...any)
	// PanicLog handles panic-level log messages.
	// will automatically occur a panic after logging
	PanicLog(header string, message ...any)
	// FatalLog handles fatal-level log messages
	// will automatically terminate the program after logging
	FatalLog(header string, message ...any)
}

// LogHandlerFunc is a function-based implementation of the LogHandler interface
type LogHandlerFunc struct {
	warpper        LogHandler
	RegularLogFunc func(level LogLevel, header string, message ...any)
	PanicLogFunc   func(header string, message ...any)
	FatalLogFunc   func(header string, message ...any)
}

// NativeLogHandler uses the standard log package for logging
var NativeLogHandler LogHandler = &LogHandlerFunc{
	RegularLogFunc: func(level LogLevel, header string, message ...any) {
		os.Stdout.WriteString(header)
		os.Stdout.WriteString(fmt.Sprintln(message...))
	},
	PanicLogFunc: func(header string, message ...any) {
		smsgs := []any{header}
		smsgs = append(smsgs, message...)
		s := fmt.Sprintln(smsgs...)
		os.Stderr.WriteString(s)
		panic(s)
	},
	FatalLogFunc: func(header string, message ...any) {
		nmsgs := []any{header}
		nmsgs = append(nmsgs, message...)
		os.Stderr.WriteString(fmt.Sprintln(nmsgs...))
		sysTerminate()
	},
}

// ------- implement LogHandler interface for LogHandlerFunc -------

func (lh *LogHandlerFunc) RegularLog(
	level LogLevel, header string, message ...any,
) {
	if lh.warpper != nil {
		lh.warpper.RegularLog(level, header, message...)
	}
	if lh.RegularLogFunc != nil {
		lh.RegularLogFunc(level, header, message...)
	}
}

func (lh *LogHandlerFunc) PanicLog(header string, message ...any) {
	if lh.warpper != nil {
		lh.warpper.RegularLog(pANIC, header, message...)
	}
	if lh.PanicLogFunc != nil {
		lh.PanicLogFunc(header, message...)
	}
}

func (lh *LogHandlerFunc) FatalLog(header string, message ...any) {
	if lh.warpper != nil {
		lh.warpper.RegularLog(fATAL, header, message...)
	}
	if lh.FatalLogFunc != nil {
		lh.FatalLogFunc(header, message...)
	}
}

// --------------------------------------------------------------
