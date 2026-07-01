---
name: nekomimi-log
description: >
  Lightweight Go logging with nekomimi.  Use when a Go project needs
  structured logging, handler composition (file rotation, network
  transport), Gin/HTTP middleware integration, or custom log handlers.
  Automatically loads alongside golang-srv for server-side Go projects.
---

# nekomimi — Lightweight Go Logging

[nekomimi](https://github.com/fiathux/nekomimi) is a zero-dependency
logging library for Go.  It provides six log levels, three families of
log methods (plain, formatted, deferred), derived loggers, trace
tracking, and composable handlers — including advanced file rotation and
TCP/UDP network transport.

```bash
go get github.com/fiathux/nekomimi
```

---

## Quick Start

```go
import "github.com/fiathux/nekomimi"

log := nekomimi.New("myapp", nekomimi.LogConfig{})

log.Dbg("starting up")
log.Inf("listening on :8080")
log.War("deprecated API called")
log.Err("failed to open file")
```

---

## Three Log Methods

Each log level exposes three method families.  Choose based on the
trade-off between convenience, formatting flexibility, and performance.

| Family | Signature | Cost | Guidance |
|--------|-----------|------|----------|
| **Plain** | `Inf(args ...any)` | Low | Default choice.  Arguments are serialised sequentially via `fmt.Sprintln`. |
| **Formatted** | `Inff(fmt string, args ...any)` | Medium | Use when multiple arguments benefit from `fmt.Sprintf`-style formatting.  Parsing the format string adds a small overhead. |
| **Deferred** | `InfP() func(args ...any)` | Low (zero when skipped) | Returns `nil` if the log level is disabled.  Use for high-frequency DEBUG/INFO logs that require pre-processing of expensive data before they can be logged. |

```go
// Plain — default for most cases
log.Inf("user", username, "logged in from", remoteAddr)

// Formatted — when format strings read better
log.Inff("user %s logged in from %s", username, remoteAddr)

// Deferred — expensive operation only evaluated when DEBUG is enabled
if fn := log.DbgP(); fn != nil {
    stacks := captureAllGoroutineStacks()  // expensive
    fn("goroutine dump:\n", stacks)
}
```

### Choosing between them

- **New code**: start with Plain.  Promoted from `Inf`/`Dbg`/`War`/`Err`.
- **Many args / structured output**: switch to Formatted (`Inff`/`Dbgf`).
- **High-frequency debug logs with expensive data preparation**: use
  Deferred (`InfP`/`DbgP`) to avoid computing data that will never be
  written.

---

## Log Levels

```
DEBUG → INFO → WARN → ERROR → PANIC → FATAL
```

- `LogConfig.Level` sets the **minimum** level that will be output.
  Defaults to `DEBUG` (everything).
- `SetLevel(lvl)` changes the threshold at runtime without restart.
- `LogConfig.LevelWithTrace` controls at which level the log header
  includes a `file:line(function)` call-trace suffix.

```go
log := nekomimi.New("api", nekomimi.LogConfig{
    Level:          nekomimi.INFO,   // ignore DEBUG
    LevelWithTrace: nekomimi.WARN,   // show caller location at WARN+
})
```

PANIC and FATAL always write regardless of the configured level.  They
append a full stack trace to the log header and then trigger
`panic(...)` / `os.Exit(1)` **from the outermost handler**.  Inner
(wrapped) handlers should NOT crash — they only write or transport the
log entry.  See the [Wrapping](#wrapping-dual-output-stdout--file)
section for how this is enforced for each handler type.

The deferred-format helper `nekomimi.Fmt(...)` is available for
collecting formatted data and only rendering the formatted version if the
level is active.

---

## Logger Initialization Patterns

### Package-level singleton via `init()`

Idiomatic for utility packages or global singletons.  `init()` runs
before any user code, and `nekomimi.New` does no I/O until the first
write, so it is safe at init time.

```go
var log nekomimi.Logger

func init() {
    log = nekomimi.New("mypkg", nekomimi.LogConfig{})
}
```

### Standalone instance with custom config

Create a dedicated logger for a struct or service.  Lets each instance
carry its own config (handler, level, time format).

```go
type Service struct {
    log nekomimi.Logger
}

func NewService() *Service {
    return &Service{
        log: nekomimi.New("Svc", nekomimi.LogConfig{
            Level: nekomimi.INFO,
        }),
    }
}
```

### Hierarchical via `Derive()`

Create child loggers that inherit the parent's handler and config but
append a dot-separated prefix.  Each child can independently adjust its
log level.

```go
base := nekomimi.New("App", nekomimi.LogConfig{})

dbLog  := base.Derive("Database")   // prefix "App.Database"
apiLog := base.Derive("API")        // prefix "App.API"

apiLog.Inf("server started")
// output: [INFO], App.API - server started

dbLog.SetLevel(nekomimi.WARN)       // only warnings from DB
dbLog.Dbg("this is silenced")
dbLog.War("slow query detected")
// output: [WARN], App.Database - slow query detected
```

The parent logger does NOT need to be stored in a struct field — child
loggers obtain independent references.  See `examples/derived.go`.

---

## Handlers

A `LogHandler` controls *where* log messages go.  The interface has five
methods: `RegularLog`, `RegularWriter`, `PanicLog`, `FatalLog`, and
`IsShutdown`.

### Default: `NativeLogHandler` (stdout/stderr)

No configuration required.  Regular levels go to stdout; PANIC/FATAL go
to stderr and then terminate the program.

```go
// these are equivalent
nekomimi.New("app", nekomimi.LogConfig{})
nekomimi.New("app", nekomimi.LogConfig{Handler: nekomimi.NativeLogHandler})
```

### Shared handler across packages

Define a handler in a common package, initialise in `init()`, and
reference it from other packages.

```go
// common/log.go
package common

var LogHandler nekomimi.LogHandler

func init() {
    LogHandler = nekomimi.NewNativeLogHandler(nil)
}
```

```go
// other packages import common and pass the shared handler
log := nekomimi.New("pkg", nekomimi.LogConfig{Handler: common.LogHandler})
```

### Wrapping: dual output (stdout + file)

`NewNativeLogHandler(wrap)` chains a second handler as the `Wrapper`.
Regular logs are forwarded to the wrapper first, then written to stdout.

**Rule**: Panic/Fatal crash behaviour must **only** live in the
outermost handler.  Wrapped (inner) handlers should write or transport
log entries without triggering `panic()` / `os.Exit`.  The mechanisms
vary by handler type:

- `TinyLogHandlerFunc` — inherently has no crash behaviour.
- `LogHandlerFunc` — leave `PanicLogFunc` and `FatalLogFunc` nil, and
  the handler writes without crashing.
- `filerotate` and `netlog` — set `WrapOnly: true` (a convenience flag
  that disables their built-in crash triggers).

```go
fh, _ := filerotate.New(ctx, filerotate.Config{
    Path: "/var/log/app", FilePrefix: "app",
    WrapOnly:     true,   // only persist, never crash
    RotatePanic:  false,
})
log := nekomimi.New("app", nekomimi.LogConfig{
    Handler: nekomimi.NewNativeLogHandler(fh),  // outermost → handles crash
})
// output goes to both file and console; panic/exit comes from NativeLogHandler
```

For simple file logging without rotation, `NewFileAccessorLogHandler`
returns a `TinyLogHandlerFunc` which by design has no crash behaviour —
ideal as an innermost wrapped handler.  Contrast with `filerotate`
in `examples/handler_compose.go`.

### File rotation handler (`handlers/filerotate`)

Automatic rotation by size, item count, or TTL.  Old archives can be
gzip-compressed.  See `examples/handler_compose.go`.

```go
import "github.com/fiathux/nekomimi/handlers/filerotate"

fh, err := filerotate.New(ctx, filerotate.Config{
    Path:         "/var/log/myapp",
    FilePrefix:   "app",
    MaxFileSize:  10240,   // 10 MB
    MaxFileItems: 100000,  // 100k entries
    MaxFileTTL:   1440,    // 24 hours
    MaxArchives:  30,      // keep 30 archives
    Compress:     true,    // gzip old files
    WrapOnly:     true,    // only persist, never crash
    RotatePanic:  false,   // suspend on rotation failure
})
```

### Network handler (`handlers/netlog`)

Sends NDJSON over TCP or UDP.  See `examples/handler_compose.go`.

```go
import "github.com/fiathux/nekomimi/handlers/netlog"

nh, err := netlog.New(ctx, netlog.Config{
    Connect:  "tcp://log-collector:28280",
    WrapOnly: true,  // only transport, don't crash
})
```

Output format (one JSON object per line):
```json
{"level":"INFO","header":"2026-06-27 [INFO], app - ","body":"service started"}
```

### Multi-handler chain

Combine multiple handlers by nesting `Wrapper` fields.  See
`examples/handler_compose.go` for a file → network → stdout chain.

### Handler Lifecycle & Graceful Shutdown

Advanced handlers (`filerotate`, `netlog`) are bound to a
`context.Context`.  Cancelling the context triggers a graceful shutdown
sequence: freeze writes, flush buffers, wait for background tasks
(compression, reconnection) to drain, then release resources.

Use `IsShutdown()` to wait for the handler to fully terminate **after**
cancelling the context.  It returns `true` only when all I/O and
background goroutines have safely completed.

```go
ctx, cancel := context.WithCancel(context.Background())

// Create handlers bound to the context
fh, _ := filerotate.New(ctx, filerotate.Config{
    Path: "/var/log/app", FilePrefix: "app",
    WrapOnly:    true,
})
nh, _ := netlog.New(ctx, netlog.Config{
    Connect: "tcp://collector:28280", WrapOnly: true,
})
log := nekomimi.New("app", nekomimi.LogConfig{
    Handler: nekomimi.NewNativeLogHandlerWithContext(ctx, fh),
})
// ... application runs ...

// --- shutdown sequence ---
cancel()

// Wait for filerotate to flush, close files, and drain compression
for !fh.IsShutdown() {
    time.Sleep(100 * time.Millisecond)
}
// Wait for netlog to close the socket and exit bgLoop
for !nh.IsShutdown() {
    time.Sleep(100 * time.Millisecond)
}
// Safe to exit: all I/O is done, no leaked goroutines
```

**Shutdown semantics by handler type:**

| Handler | `IsShutdown()` becomes `true` when |
|---------|------------------------------------|
| `filerotate` | ctx cancelled, file flushed+closed, compression goroutines drained |
| `netlog` TCP | ctx cancelled, socket closed, bgLoop exited |
| `netlog` UDP | ctx cancelled, socket closed, bgLoop exited |
| `NewFileAccessorLogHandler` | ctx cancelled, file flushed+closed |
| `NewNativeLogHandlerWithContext` | ctx.Done() fires |
| `NewNativeLogHandler` | Never (uses `context.Background()`) |
| bare `LogHandlerFunc` | Never (no `IsShutdownFunc` set) |

**Implementing IsShutdown in custom handlers:**

- **`LogHandlerFunc`**: set the `IsShutdownFunc func() bool` field.  The
  framework's `IsShutdown()` first checks `Wrapper.IsShutdown()`, then
  calls `IsShutdownFunc`.  Both must return `true` for the handler to be
  considered fully shut down.  Leave `IsShutdownFunc` nil if the handler
  has no shutdown awareness.

- **`TinyLogHandlerFunc`**: shutdown is detected via a probe mechanism.
  When the underlying resource is permanently closed (e.g. `fp == nil`),
  the implementation should **return without calling `pnt`**.  The probe
  sends a `TINY_DONE` sentinel level with a marker function; if the
  marker is never called, the handler is considered shut down.

---

## Gin Integration

The pattern: use `log.GetWriter()` to obtain nekomimi's internal
`io.StringWriter`, wrap it as an `io.Writer`, and pass it to Gin's
middleware.  See `examples/gin_adapter.go`.

**Step 1** — define an adapter:
```go
type ginLogWriter struct {
    log nekomimi.Logger
}
func (w *ginLogWriter) Write(p []byte) (int, error) {
    // GetWriter returns an io.StringWriter bound to INFO level
    return w.log.GetWriter(nekomimi.INFO, false).WriteString(string(p))
}
```

**Step 2** — attach to Gin:
```go
engine := gin.New()
engine.Use(gin.LoggerWithConfig(gin.LoggerConfig{
    Output: &ginLogWriter{log: componentLog},
}))
```

**Step 3** — (optional) redirect Gin's internal debug prints:
```go
// only in debug builds
gin.DefaultWriter = &ginLogWriter{log: debugLog}
gin.DebugPrintRouteFunc = func(httpMethod, absolutePath, handlerName string, nuHandlers int) {
    debugLog.Dbgf("%-6s %-25s → %s (%d handlers)", httpMethod, absolutePath, handlerName, nuHandlers)
}
```

---

## Custom Handler Development

### `TinyLogHandlerFunc` — minimal implementation

A single-function handler.  Panic and Fatal are written as regular log
entries (no crash).  Designed to be wrapped by an outer handler that
provides locking and crash behaviour.

```go
h := nekomimi.TinyLogHandlerFunc(
    func(lv nekomimi.LogLevel, pnt func(io.StringWriter)) {
        pnt(os.Stdout)
    },
)
```

### `LogHandlerFunc` — full implementation

Six configurable fields.  `PanicLogFunc` and `FatalLogFunc` should
**only** be set when this handler is the outermost one in the chain;
leave them nil otherwise.  `IsShutdownFunc` reports whether the
handler's own resources have been released.  For a complete annotated
example see `examples/custom_handler.go`.

```go
h := &nekomimi.LogHandlerFunc{
    Lock:           &sync.Mutex{},        // thread safety
    RegularLogFunc: func(lv nekomimi.LogLevel, pnt func(io.StringWriter)) {
        pnt(os.Stdout)                   // write regular logs
    },
    PanicLogFunc: func(pnt func(io.StringWriter), info string) func() {
        pnt(os.Stderr)                   // write before crash
        return func() { panic(info) }    // finaliser called outside lock
    },
    FatalLogFunc: func(pnt func(io.StringWriter)) func() {
        pnt(os.Stderr)
        return func() { os.Exit(1) }
    },
    IsShutdownFunc: func() bool {        // shutdown awareness
        select {
        case <-ctx.Done(): return true
        default:          return false
        }
    },
    Wrapper: someOtherHandler,           // chain another handler
}
```

**Key detail**: `PanicLogFunc` and `FatalLogFunc` return a **finaliser
closure**.  The log framework calls the finaliser *after* releasing the
mutex.  This prevents the lock from being held during `panic()`/`os.Exit`.

**Wrapper forwarding**: When a handler has a non-nil `Wrapper`, every
log call is forwarded to `Wrapper.RegularWriter()` **before** the
handler's own function runs.  Panic/Fatal are forwarded as
`RegularWriter(PANIC, ...)` — the wrapper never sees raw panic/fatal
calls, so it does not crash unless it is the outermost handler.

---

## Reference

| API | Signature | Note |
|-----|-----------|------|
| `nekomimi.New` | `(name string, cfg LogConfig) Logger` | Create a logger |
| `nekomimi.LogConfig` | `{Handler, Level, LevelWithTrace, TimeFormat}` | Logger configuration |
| `.Derive(prefix)` | `Logger` | Child logger with dotted prefix |
| `.Trace(name)` | `TraceLogger` | Creates a UUIDv7-tagged trace logger |
| `.SetLevel(lvl)` | — | Dynamic level change |
| `.SetLogHandler(h)` | — | Replace handler at runtime |
| `.WrapLogHandler(fn)` | — | Wrap existing handler with a function |
| `.GetWriter(level, calltrace)` | `io.StringWriter` | Low-level writer for custom adapters |
| `.RawWriter()` | `RawWriter` | Bypasses header formatting |
| `filerotate.New(ctx, cfg)` | `(LogHandler, error)` | File rotation handler |
| `netlog.New(ctx, cfg)` | `(LogHandler, error)` | Network (TCP/UDP) handler |
| `.IsShutdown()` | `bool` | Returns `true` when handler IO/goroutines terminated |
| `nekomimi.NewNativeLogHandlerWithContext(ctx, wrap)` | `LogHandler` | Context-aware native handler for shutdown |

### Examples

| File | Description |
|------|-------------|
| `examples/basic.go` | Simple init, three method families, dynamic level |
| `examples/derived.go` | Package singleton, `Derive()` hierarchy |
| `examples/handler_compose.go` | stdout + rotation + network wrapper chain |
| `examples/custom_handler.go` | Building custom handlers |
| `examples/gin_adapter.go` | Gin middleware integration |
