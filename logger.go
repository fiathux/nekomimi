// Package nekomimi provides a very light-weight and simple logging module for
// golang
package nekomimi

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// LogLevel represents the severity level of a log message
type LogLevel uint32

const (
	// DEBUG level for detailed debugging information
	DEBUG LogLevel = iota
	// INFO level for general informational messages
	INFO
	// WARN level for warning messages
	WARN
	// ERROR level for error messages
	ERROR
	// pANIC level for critical error messages
	pANIC
	// fATAL level for fatal error messages
	fATAL
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
	case pANIC:
		return "PANIC"
	case fATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// BaiscLogger defines the basic logging methods for different log levels
// following log levels are supported:
//   - Dbg: Debug level logging
//   - Inf: Info level logging
//   - War: Warning level logging
//   - Err: Error level logging
//
// each level supports three types of logging methods:
//   - Simple message logging: e.g., Dbg(message ...any)
//   - Formatted message logging: e.g., Dbgf(format string, args ...any)
//   - Deferred message logging: e.g., DbgP() func(message ...any)
//
// the Simple type is the fastest, and Deferred is useful for expensive log
// message construction. the Deferred type might return nil if the log level is
// not enabled.
type BaiscLogger interface {
	// Debug level - simple output
	Dbg(message ...any)
	// Debug level - formatted output
	Dbgf(format string, args ...any)
	// Debug level - deferred output
	DbgP() func(message ...any)
	// Info level logging
	Inf(message ...any)
	// Info level - formatted output
	Inff(format string, args ...any)
	// Info level - deferred output
	InfP() func(message ...any)
	// Warning level logging
	War(message ...any)
	// Warning level - formatted output
	Warf(format string, args ...any)
	// Warning level - deferred output
	WarP() func(message ...any)
	// Error level logging
	Err(message ...any)
	// Error level - formatted output
	Errf(format string, args ...any)
	// Error level - deferred output
	ErrP() func(message ...any)
}

// TraceLogger extends BaiscLogger with tracing capabilities
type TraceLogger interface {
	BaiscLogger

	// Retrieve the Trace ID
	TraceID() string
	// Retrieve the Trace Name
	TraceName() string
}

// Logger is the full-featured logger interface
type Logger interface {
	BaiscLogger
	// Panic level logging
	Panic(message ...any)
	Panicf(format string, args ...any)
	// Fatal level logging
	Fatal(message ...any)
	Fatalf(format string, args ...any)
	// Create a new TraceLogger with the given name
	Trace(name string) TraceLogger
	// Derive a new Logger with the given prefix name
	Derive(pfx string) Logger
	// Set log level
	SetLevel(level LogLevel)
	// Set log level that includes call trace information
	SetCallTraceLevel(level LogLevel)
	// Set the time format for log messages
	SetTimeFormat(format string)
	// Set the log handler
	SetLogHandler(handler LogHandler)
	// Replace the current log handler with a wrapped function.
	// if the wrapper returns nil, the log handler will be reset to the default
	// handler (NativeLogHandler).
	WrapLogHandler(wrapper func(old LogHandler) LogHandler)
}

// LogConfig provides configuration options for the logger
type LogConfig struct {
	Handler        LogHandler
	Level          LogLevel
	LevelWithTrace LogLevel
	TimeFormat     string
}

// traceID represents a trace identifier with a name and ID
type traceID struct {
	name string
	id   string
}

// logger implements the Logger interface
type logger struct {
	mtx        sync.RWMutex
	logHandler LogHandler
	level      LogLevel
	levelct    LogLevel
	prefix     string
	timefmt    string
	fmtHeader  func(level LogLevel, tid *traceID) string
}

// traceLogger implements the TraceLogger interface
type traceLogger struct {
	parent *logger
	tid    traceID
}

// newTraceID generates a new traceID with the given name
func newTraceID(name string) traceID {
	id, _ := uuid.NewV7()
	return traceID{
		name: name,
		id:   id.String(),
	}
}

// getStackHeader retrieves the caller information for logging
func getStackHeader(skip int) string {
	pc, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "unknown:0 "
	}
	fn := runtime.FuncForPC(pc)
	// split base file name
	basefile := file
	if idx := strings.LastIndex(file, "/"); idx != -1 {
		basefile = file[idx+1:]
	}
	// split base function name (without package path)
	fnName := fn.Name()
	if idx := strings.LastIndex(fnName, "/"); idx != -1 {
		fnName = fnName[idx+1:]
	}
	return fmt.Sprintf(" %s:%d(%s)", basefile, line, fnName)
}

// formatStack formats the current call stack for logging
func formatStack(skip int) string {
	pc := make([]uintptr, 10)
	n := runtime.Callers(skip, pc)
	frames := runtime.CallersFrames(pc[:n])

	stack := make([]string, 0, n)
	for {
		frame, more := frames.Next()
		stack = append(stack,
			fmt.Sprintf(" %s:%d(%s)", frame.File, frame.Line, frame.Function))
		if !more {
			break
		}
	}
	return fmt.Sprintf(" >> Stacks:\n    %s\n<<<<", strings.Join(stack, "\n    "))
}

// String returns the string representation of the traceID
func (tid *traceID) String() string {
	if tid == nil {
		return ""
	}
	if tid.name != "" {
		return fmt.Sprintf("<%s:%s>", tid.name, tid.id)
	}
	return fmt.Sprintf("<%s>", tid.id)
}

// getHeaderFromatter constructs the log message header
func getHeaderFromatter(
	timefmt string,
	prefix string,
	levelcalltrace LogLevel,
	tbskip int,
) func(level LogLevel, tid *traceID) string {
	return func(level LogLevel, tid *traceID) string {
		calltrace := level >= levelcalltrace
		stackInfo := ""
		if level >= pANIC {
			stackInfo = formatStack(tbskip + 1)
		} else if calltrace {
			stackInfo = getStackHeader(tbskip)
		}
		timestr := time.Now().Format(timefmt)
		// FORMAT: time [level], perfix<trace> calltrace -
		return fmt.Sprintf("%s [%s], %s%s%s - ",
			timestr,
			level.String(),
			prefix,
			tid.String(),
			stackInfo,
		)
	}
}

// New creates a new Logger instance with the given name and configuration
func New(name string, config LogConfig) Logger {
	timefmt := config.TimeFormat
	if timefmt == "" {
		timefmt = "2006-01-02 15:04:05.000"
	}
	hander := config.Handler
	if hander == nil {
		hander = NativeLogHandler
	}
	if name == "" {
		name = "*"
	}
	return &logger{
		logHandler: hander,
		level:      config.Level,
		prefix:     name,
		timefmt:    timefmt,
		fmtHeader: getHeaderFromatter(
			timefmt,
			name,
			config.LevelWithTrace,
			4,
		),
	}
}

// getFmtHeader safely retrieves the fmtHeader function
func (l *logger) getFmtHeader() func(level LogLevel, tid *traceID) string {
	l.mtx.RLock()
	defer l.mtx.RUnlock()
	return l.fmtHeader
}

// outputRegularLog outputs a regular log message
func (l *logger) outputRegularLog(level LogLevel, message ...any) {
	header := l.getFmtHeader()(level, nil)
	l.logHandler.RegularLog(level, header, message...)
}

// outputPanicLog outputs a panic log message
func (l *logger) outputPanicLog(message ...any) {
	header := l.getFmtHeader()(pANIC, nil)
	l.logHandler.PanicLog(header, message...)
}

// outputFatalLog outputs a fatal log message
func (l *logger) outputFatalLog(message ...any) {
	header := l.getFmtHeader()(fATAL, nil)
	l.logHandler.FatalLog(header, message...)
}

// ------- implement BaiscLogger interface for logger -------

func (l *logger) Dbg(message ...any) {
	if atomic.LoadUint32((*uint32)(&l.level)) <= uint32(DEBUG) {
		l.outputRegularLog(DEBUG, message...)
	}
}

func (l *logger) Dbgf(format string, args ...any) {
	if atomic.LoadUint32((*uint32)(&l.level)) <= uint32(DEBUG) {
		l.outputRegularLog(DEBUG, fmt.Sprintf(format, args...))
	}
}

func (l *logger) DbgP() func(message ...any) {
	if atomic.LoadUint32((*uint32)(&l.level)) <= uint32(DEBUG) {
		return func(message ...any) {
			l.outputRegularLog(DEBUG, message...)
		}
	}
	return nil
}

func (l *logger) Inf(message ...any) {
	if atomic.LoadUint32((*uint32)(&l.level)) <= uint32(INFO) {
		l.outputRegularLog(INFO, message...)
	}
}

func (l *logger) Inff(format string, args ...any) {
	if atomic.LoadUint32((*uint32)(&l.level)) <= uint32(INFO) {
		l.outputRegularLog(INFO, fmt.Sprintf(format, args...))
	}
}

func (l *logger) InfP() func(message ...any) {
	if atomic.LoadUint32((*uint32)(&l.level)) <= uint32(INFO) {
		return func(message ...any) {
			l.outputRegularLog(INFO, message...)
		}
	}
	return nil
}

func (l *logger) War(message ...any) {
	if atomic.LoadUint32((*uint32)(&l.level)) <= uint32(WARN) {
		l.outputRegularLog(WARN, message...)
	}
}

func (l *logger) Warf(format string, args ...any) {
	if atomic.LoadUint32((*uint32)(&l.level)) <= uint32(WARN) {
		l.outputRegularLog(WARN, fmt.Sprintf(format, args...))
	}
}

func (l *logger) WarP() func(message ...any) {
	if atomic.LoadUint32((*uint32)(&l.level)) <= uint32(WARN) {
		return func(message ...any) {
			l.outputRegularLog(WARN, message...)
		}
	}
	return nil
}

func (l *logger) Err(message ...any) {
	if atomic.LoadUint32((*uint32)(&l.level)) <= uint32(ERROR) {
		l.outputRegularLog(ERROR, message...)
	}
}

func (l *logger) Errf(format string, args ...any) {
	if atomic.LoadUint32((*uint32)(&l.level)) <= uint32(ERROR) {
		l.outputRegularLog(ERROR, fmt.Sprintf(format, args...))
	}
}

func (l *logger) ErrP() func(message ...any) {
	if atomic.LoadUint32((*uint32)(&l.level)) <= uint32(ERROR) {
		return func(message ...any) {
			l.outputRegularLog(ERROR, message...)
		}
	}
	return nil
}

// --------------------------------------------------------------

// ------- implement Logger interface for logger -------

func (l *logger) Panic(message ...any) {
	l.outputPanicLog(message...)
}

func (l *logger) Panicf(format string, args ...any) {
	l.outputPanicLog(fmt.Sprintf(format, args...))
}

func (l *logger) Fatal(message ...any) {
	l.outputFatalLog(message...)
}

func (l *logger) Fatalf(format string, args ...any) {
	l.outputFatalLog(fmt.Sprintf(format, args...))
}

func (l *logger) Trace(name string) TraceLogger {
	tid := newTraceID(name)
	return &traceLogger{
		parent: l,
		tid:    tid,
	}
}

func (l *logger) Derive(pfx string) Logger {
	l.mtx.RLock()
	defer l.mtx.RUnlock()
	newPrefix := l.prefix
	if pfx != "" {
		newPrefix = newPrefix + "." + pfx
	}
	return &logger{
		logHandler: l.logHandler,
		level:      l.level,
		prefix:     newPrefix,
		timefmt:    l.timefmt,
		fmtHeader: getHeaderFromatter(
			l.timefmt,
			newPrefix,
			l.levelct,
			4,
		),
	}
}

func (l *logger) SetLevel(level LogLevel) {
	atomic.StoreUint32((*uint32)(&l.level), uint32(level))
}

func (l *logger) SetCallTraceLevel(level LogLevel) {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	l.levelct = level
	l.fmtHeader = getHeaderFromatter(
		l.timefmt,
		l.prefix,
		l.levelct,
		4,
	)
}

func (l *logger) SetTimeFormat(format string) {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	l.timefmt = format
	l.fmtHeader = getHeaderFromatter(
		l.timefmt,
		l.prefix,
		l.levelct,
		4,
	)
}

func (l *logger) SetLogHandler(handler LogHandler) {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	l.logHandler = handler
}

func (l *logger) WrapLogHandler(wrapper func(old LogHandler) LogHandler) {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	l.logHandler = wrapper(l.logHandler)
	if l.logHandler == nil {
		l.logHandler = NativeLogHandler
	}
}

// --------------------------------------------------------------

// ------- implement TraceLogger interface for traceLogger -------

func (tl *traceLogger) regularLog(level LogLevel, message ...any) {
	header := tl.parent.getFmtHeader()(level, &tl.tid)
	tl.parent.logHandler.RegularLog(level, header, message...)
}

func (tl *traceLogger) Dbg(message ...any) {
	if atomic.LoadUint32((*uint32)(&tl.parent.level)) <= uint32(DEBUG) {
		tl.regularLog(DEBUG, message...)
	}
}

func (tl *traceLogger) Dbgf(format string, args ...any) {
	if atomic.LoadUint32((*uint32)(&tl.parent.level)) <= uint32(DEBUG) {
		tl.regularLog(DEBUG, fmt.Sprintf(format, args...))
	}
}

func (tl *traceLogger) DbgP() func(message ...any) {
	if atomic.LoadUint32((*uint32)(&tl.parent.level)) <= uint32(DEBUG) {
		return func(message ...any) {
			tl.regularLog(DEBUG, message...)
		}
	}
	return nil
}

func (tl *traceLogger) Inf(message ...any) {
	if atomic.LoadUint32((*uint32)(&tl.parent.level)) <= uint32(INFO) {
		tl.regularLog(INFO, message...)
	}
}

func (tl *traceLogger) Inff(format string, args ...any) {
	if atomic.LoadUint32((*uint32)(&tl.parent.level)) <= uint32(INFO) {
		tl.regularLog(INFO, fmt.Sprintf(format, args...))
	}
}

func (tl *traceLogger) InfP() func(message ...any) {
	if atomic.LoadUint32((*uint32)(&tl.parent.level)) <= uint32(INFO) {
		return func(message ...any) {
			tl.regularLog(INFO, message...)
		}
	}
	return nil
}

func (tl *traceLogger) War(message ...any) {
	if atomic.LoadUint32((*uint32)(&tl.parent.level)) <= uint32(WARN) {
		tl.regularLog(WARN, message...)
	}
}

func (tl *traceLogger) Warf(format string, args ...any) {
	if atomic.LoadUint32((*uint32)(&tl.parent.level)) <= uint32(WARN) {
		tl.regularLog(WARN, fmt.Sprintf(format, args...))
	}
}

func (tl *traceLogger) WarP() func(message ...any) {
	if atomic.LoadUint32((*uint32)(&tl.parent.level)) <= uint32(WARN) {
		return func(message ...any) {
			tl.regularLog(WARN, message...)
		}
	}
	return nil
}

func (tl *traceLogger) Err(message ...any) {
	if atomic.LoadUint32((*uint32)(&tl.parent.level)) <= uint32(ERROR) {
		tl.regularLog(ERROR, message...)
	}
}

func (tl *traceLogger) Errf(format string, args ...any) {
	if atomic.LoadUint32((*uint32)(&tl.parent.level)) <= uint32(ERROR) {
		tl.regularLog(ERROR, fmt.Sprintf(format, args...))
	}
}

func (tl *traceLogger) ErrP() func(message ...any) {
	if atomic.LoadUint32((*uint32)(&tl.parent.level)) <= uint32(ERROR) {
		return func(message ...any) {
			tl.regularLog(ERROR, message...)
		}
	}
	return nil
}

func (tl *traceLogger) TraceID() string {
	return tl.tid.id
}

func (tl *traceLogger) TraceName() string {
	return tl.tid.name
}

// --------------------------------------------------------------
