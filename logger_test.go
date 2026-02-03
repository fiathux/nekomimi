package nekomimi

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type testLogHandler struct {
	logs        []any
	h           string
	panicCalled bool
	fatalCalled bool
	hnd         *LogHandlerFunc
	wrpcalled   bool
	wrpspcalled bool
}

func (tlh *testLogHandler) clean() {
	tlh.logs = []any{}
	tlh.h = ""
	tlh.panicCalled = false
	tlh.fatalCalled = false
}

func TestLogger(t *testing.T) {
	tlh := &testLogHandler{}
	tlh.hnd = &LogHandlerFunc{
		RegularLogFunc: func(level LogLevel, header string, message ...any) {
			tlh.h = header
			tlh.logs = append(tlh.logs, message...)
		},
		PanicLogFunc: func(header string, message ...any) {
			tlh.panicCalled = true
			tlh.h = header
			tlh.logs = append(tlh.logs, message...)
		},
		FatalLogFunc: func(header string, message ...any) {
			tlh.fatalCalled = true
			tlh.h = header
			tlh.logs = append(tlh.logs, message...)
		},
		warpper: &LogHandlerFunc{
			RegularLogFunc: func(level LogLevel, header string, message ...any) {
				tlh.wrpcalled = true
			},
			PanicLogFunc: func(header string, message ...any) {
				tlh.wrpspcalled = true
			},
			FatalLogFunc: func(header string, message ...any) {
				tlh.wrpspcalled = true
			},
		},
	}

	Convey("Logger tests", t, func() {
		Convey("Create logger in default config", func() {
			l := New("", LogConfig{})
			So(l, ShouldNotBeNil)
			loginst, ok := l.(*logger)
			So(ok, ShouldBeTrue)
			So(loginst.level, ShouldEqual, DEBUG)
			So(loginst.logHandler, ShouldEqual, NativeLogHandler)
			So(loginst.prefix, ShouldEqual, "*")
			So(loginst.timefmt, ShouldEqual, "2006-01-02 15:04:05.000")
			So(loginst.fmtHeader, ShouldNotBeNil)
			// try all log levels
			l.Dbg("debug message", "a", 1, true)
			l.Dbgf("formatted debug: %s - %d", "test", 42)
			dbgprt := l.DbgP()
			So(dbgprt, ShouldNotBeNil)
			dbgprt("deferred debug", 3.14)
			l.Inf("info message", 123)
			l.Inff("formatted info: %s - %d", "info", 99)
			infprt := l.InfP()
			So(infprt, ShouldNotBeNil)
			infprt("deferred info", false)
			l.War("warn message")
			l.Warf("formatted warn: %s - %d", "warn", 77)
			warprt := l.WarP()
			So(warprt, ShouldNotBeNil)
			warprt("deferred warn", 'c')
			l.Err("error message")
			l.Errf("formatted error: %s - %d", "error", 55)
			errprt := l.ErrP()
			So(errprt, ShouldNotBeNil)
			errprt("deferred error", -1)
			var panicCalled bool
			func() {
				defer func() {
					if p := recover(); p != nil {
						panicCalled = true
					}
				}()
				l.Panic("panic message")
			}()
			So(panicCalled, ShouldBeTrue)
			panicCalled = false
			func() {
				defer func() {
					if p := recover(); p != nil {
						panicCalled = true
					}
				}()
				l.Panicf("formatted panic: %s - %d", "panic", 11)
			}()
			So(panicCalled, ShouldBeTrue)
			var terminateCalled bool
			backupTm := sysTerminate
			sysTerminate = func() {
				terminateCalled = true
			}
			l.Fatal("fatal message")
			So(terminateCalled, ShouldBeTrue)
			terminateCalled = false
			l.Fatalf("formatted fatal: %s - %d", "fatal", 22)
			So(terminateCalled, ShouldBeTrue)
			sysTerminate = backupTm
			// defferred log test
			l.SetLevel(ERROR)
			So(loginst.level, ShouldEqual, ERROR)
			So(l.DbgP(), ShouldBeNil)
			So(l.InfP(), ShouldBeNil)
			So(l.WarP(), ShouldBeNil)
			So(l.ErrP(), ShouldNotBeNil)
			// trace log
			tlog := l.Trace("")
			So(tlog, ShouldNotBeNil)
			tid := tlog.TraceID()
			So(tid, ShouldNotBeEmpty)
			So(tlog.TraceName(), ShouldEqual, "")
			tlog.Dbg("trace debug")
			tlog.Dbgf("trace formatted debug: %s", "dbg")
			tlog.Inf("trace info")
			tlog.Inff("trace formatted info: %s", "inf")
			tlog.War("trace warn")
			tlog.Warf("trace formatted warn: %s", "war")
			tlog.Err("trace error")
			tlog.Errf("trace formatted error: %s", "err")
			So(tlog.DbgP(), ShouldBeNil)
			So(tlog.InfP(), ShouldBeNil)
			So(tlog.WarP(), ShouldBeNil)
			errtp := tlog.ErrP()
			So(errtp, ShouldNotBeNil)
			errtp("deferred trace error")
			l.SetLevel(DEBUG)
			dbgtp := tlog.DbgP()
			So(dbgtp, ShouldNotBeNil)
			dbgtp("deferred trace debug")
			inftp := tlog.InfP()
			So(inftp, ShouldNotBeNil)
			inftp("deferred trace info")
			wartp := tlog.WarP()
			So(wartp, ShouldNotBeNil)
			wartp("deferred trace warn")
			// Misc
			l.SetCallTraceLevel(WARN)
			So(loginst.levelct, ShouldEqual, WARN)
			l.SetTimeFormat("15:04")
			So(loginst.timefmt, ShouldEqual, "15:04")
		})
		Convey("Create logger with custom config", func() {
			l := New("TestPrefix", LogConfig{
				Level:          INFO,
				Handler:        tlh.hnd,
				LevelWithTrace: WARN,
				TimeFormat:     "15:04:05.000",
			})
			So(l, ShouldNotBeNil)
			loginst, ok := l.(*logger)
			So(ok, ShouldBeTrue)
			So(loginst.level, ShouldEqual, INFO)
			So(loginst.logHandler, ShouldEqual, tlh.hnd)
			So(loginst.prefix, ShouldEqual, "TestPrefix")
			So(loginst.timefmt, ShouldEqual, "15:04:05.000")
			So(loginst.fmtHeader, ShouldNotBeNil)
			l.Dbg("a", "b", "C")
			So(len(tlh.logs), ShouldEqual, 0)
			So(tlh.h, ShouldEqual, "")
			So(tlh.wrpcalled, ShouldBeFalse)
			l.SetLevel(DEBUG)
			So(loginst.level, ShouldEqual, DEBUG)
			l.Dbg("a", "b", "C")
			So(len(tlh.logs), ShouldEqual, 3)
			So(tlh.h[13:], ShouldEqual, "[DEBUG], TestPrefix - ")
			So(tlh.wrpcalled, ShouldBeTrue) // warpper should be called
			tlh.clean()
			l.Inf("info message", 123)
			So(len(tlh.logs), ShouldEqual, 2)
			So(tlh.h[13:], ShouldEqual, "[INFO], TestPrefix - ")
			tlh.clean()
			l.War("warn message")
			So(len(tlh.logs), ShouldEqual, 1)
			So(tlh.h[13:32], ShouldEqual, "[WARN], TestPrefix ")
			So(tlh.h[32:], ShouldStartWith, "logger_test.go")
			tlh.clean()
			l.Err("error message")
			So(len(tlh.logs), ShouldEqual, 1)
			So(tlh.h[13:33], ShouldEqual, "[ERROR], TestPrefix ")
			So(tlh.h[33:], ShouldStartWith, "logger_test.go")
			tlh.clean()
			tlh.wrpcalled = false
			l.Panic("panic message")
			So(len(tlh.logs), ShouldEqual, 1)
			So(tlh.panicCalled, ShouldBeTrue)
			So(tlh.h[13:33], ShouldEqual, "[PANIC], TestPrefix ")
			So(tlh.h[33:], ShouldStartWith, ">> Stacks:\n")
			tlh.clean()
			l.Fatal("fatal message")
			So(len(tlh.logs), ShouldEqual, 1)
			So(tlh.fatalCalled, ShouldBeTrue)
			So(tlh.h[13:33], ShouldEqual, "[FATAL], TestPrefix ")
			So(tlh.h[33:], ShouldStartWith, ">> Stacks:\n")
			// should not call Panic/Fatal on warpper
			So(tlh.wrpspcalled, ShouldBeFalse)
			// should call regular log on warpper
			So(tlh.wrpcalled, ShouldBeTrue)
			tlh.clean()
			// Trace log
			tlog := l.Trace("TR")
			So(tlog.TraceName(), ShouldEqual, "TR")
			tid := tlog.TraceID()
			So(tlog, ShouldNotBeNil)
			tlog.Err("trace error")
			So(len(tlh.logs), ShouldEqual, 1)
			So(tlh.h[13:73], ShouldEqual, "[ERROR], TestPrefix<TR:"+tid+">")
			tlh.clean()
			// Derive log
			dlog := l.Derive("DER")
			dlog.Inf("derived", "info")
			So(len(tlh.logs), ShouldEqual, 2)
			So(tlh.h[13:35], ShouldEqual, "[INFO], TestPrefix.DER")
			tlh.clean()
			dlog.Warf("%s - %d", "formatted", 456)
			So(len(tlh.logs), ShouldEqual, 1)
			So(tlh.h[13:35], ShouldEqual, "[WARN], TestPrefix.DER")
			tlh.clean()
			// defferred log test
			dlog.SetLevel(ERROR)
			dinst, ok := dlog.(*logger)
			So(ok, ShouldBeTrue)
			So(dinst.level, ShouldEqual, ERROR)
			dlog.War("this will not be logged")
			So(len(tlh.logs), ShouldEqual, 0)
			So(tlh.h, ShouldEqual, "")
			wp := dlog.WarP()
			So(wp, ShouldBeNil)
			ep := dlog.ErrP()
			So(ep, ShouldNotBeNil)
			ep("derived error with P", "x")
			So(len(tlh.logs), ShouldEqual, 2)
			So(tlh.h[13:36], ShouldEqual, "[ERROR], TestPrefix.DER")
			// Log hander replace test
			l.WrapLogHandler(func(old LogHandler) LogHandler {
				return old // not change
			})
			So(loginst.logHandler, ShouldEqual, tlh.hnd)
			l.WrapLogHandler(func(old LogHandler) LogHandler {
				return nil // reset to default
			})
			So(loginst.logHandler, ShouldEqual, NativeLogHandler)
			l.SetLogHandler(tlh.hnd)
			So(loginst.logHandler, ShouldEqual, tlh.hnd)
		})
	})
}
