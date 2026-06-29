package netlog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fiathux/nekomimi"
)

// reconnectInterval is the interval between TCP reconnection attempts.
// Replaced in tests to speed up reconnection verification.
var reconnectInterval = 2 * time.Second

// ioDeadline is the per-operation deadline for TCP connections:
// - write timeout in sendJSON (stalled connection detection)
// - dial timeout in bgLoop reconnection attempts
const ioDeadline = 2 * time.Second

// exitFunc is the function called for program termination in FatalLog.
// Replaced in tests to verify FatalLog behavior without os.Exit.
var exitFunc = os.Exit

// Config defines the configuration for the network log handler.
type Config struct {
	// Connect is the target address in URL-style format.
	// Supported schemes: "tcp://host:port", "udp://host:port".
	Connect string
	// WrapOnly disables panic/exit behavior in PanicLog and FatalLog.
	// When true, the handler only sends log messages without
	// triggering program termination. Useful when nested inside
	// another handler chain.
	WrapOnly bool
	// Wrapper is an optional LogHandler that receives log messages
	// before this handler does. Typically used to chain handlers.
	Wrapper nekomimi.LogHandler
}

// netHandler implements nekomimi.LogHandler for network log transport.
type netHandler struct {
	cfg     Config
	mu      sync.Mutex
	conn    net.Conn
	ctx     context.Context
	cancel  context.CancelFunc
	network string // "tcp" or "udp"
	addr    string // host:port
}

// New creates a new network log handler. The Connect URL must use
// "tcp" or "udp" scheme. Returns an error on URL parse failure,
// unsupported scheme, or initial connect failure.
func New(ctx context.Context, cfg Config) (nekomimi.LogHandler, error) {
	u, err := url.Parse(cfg.Connect)
	if err != nil {
		return nil, fmt.Errorf("netlog: parse connect URL: %w", err)
	}
	if u.Scheme != "tcp" && u.Scheme != "udp" {
		return nil, fmt.Errorf(
			"netlog: unsupported scheme %q, must be tcp or udp",
			u.Scheme,
		)
	}
	addr := u.Host
	if addr == "" {
		return nil, fmt.Errorf(
			"netlog: missing host in connect URL %q",
			cfg.Connect,
		)
	}

	conn, err := net.Dial(u.Scheme, addr)
	if err != nil {
		return nil, fmt.Errorf(
			"netlog: dial %s: %w", cfg.Connect, err,
		)
	}

	hctx, cancel := context.WithCancel(ctx)

	h := &netHandler{
		cfg:     cfg,
		conn:    conn,
		ctx:     hctx,
		cancel:  cancel,
		network: u.Scheme,
		addr:    addr,
	}

	go h.bgLoop()

	return h, nil
}

// bgLoop runs a background goroutine for lifecycle management.
// For TCP, it also handles reconnection via a ticker that fires
// every reconnectInterval. For UDP, it only watches ctx.Done()
// to close the connection on cancellation.
//
// Lock discipline: h.mu is held only for brief reads/writes of h.conn.
// net.Dial can block for seconds on timeout and must be done outside
// the lock so it does not stall RegularLog / RegularWriter callers.
func (h *netHandler) bgLoop() {
	if h.network == "tcp" {
		ticker := time.NewTicker(reconnectInterval)
		defer ticker.Stop()
		for {
			select {
			case <-h.ctx.Done():
				h.mu.Lock()
				// conn.Close may panic on broken connections;
				// defer guarantees the lock is always released.
				defer h.mu.Unlock()
				if h.conn != nil {
					h.conn.Close()
					h.conn = nil
				}
				return
			case <-ticker.C:
				// check disconnect status under lock (fast path)
				h.mu.Lock()
				needDial := h.conn == nil
				h.mu.Unlock()

				if needDial {
					// DialContext respects h.ctx: on cancel it
					// returns immediately instead of blocking
					// up to the system TCP timeout (~2 min).
					// A 2 s dial timeout prevents a single
					// unreachable host from stalling the ticker.
					d := net.Dialer{Timeout: ioDeadline}
					conn, err := d.DialContext(
						h.ctx, "tcp", h.addr)
					if err == nil {
						h.mu.Lock()
						h.conn = conn
						h.mu.Unlock()
					}
				}
			}
		}
	} else {
		<-h.ctx.Done()
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.conn != nil {
			h.conn.Close()
			h.conn = nil
		}
	}
}

// sendJSON marshals the log entry to NDJSON and writes it to the
// connection. Must be called with h.mu held. For TCP, write failure
// triggers disconnect. For UDP, failure is silently ignored.
func (h *netHandler) sendJSON(
	level nekomimi.LogLevel, header, body string,
) {
	body = strings.TrimSuffix(body, "\n")
	entry := map[string]string{
		"level":  level.String(),
		"header": header,
		"body":   body,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return // marshal failure, drop log
	}
	data = append(data, '\n')
	if h.network == "tcp" {
		h.conn.SetWriteDeadline(time.Now().Add(ioDeadline))
	}
	_, err = h.conn.Write(data)
	if err != nil && h.network == "tcp" {
		h.conn.Close()
		h.conn = nil // mark disconnected, ticker will retry
	}
}

// makePnt creates a pnt function that writes header + message body.
// Used when forwarding PanicLog/FatalLog to the wrapper handler.
func makePnt(header string, message ...any) func(io.StringWriter) {
	sp := fmt.Sprintln(message...)
	return func(w io.StringWriter) {
		w.WriteString(header)
		w.WriteString(sp)
	}
}

// RegularLog handles regular log messages with a specified log level.
func (h *netHandler) RegularLog(
	level nekomimi.LogLevel, header string, message ...any,
) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cfg.Wrapper != nil {
		h.cfg.Wrapper.RegularLog(level, header, message...)
	}
	if h.conn == nil {
		return
	}
	h.sendJSON(level, header, fmt.Sprint(message...))
}

// RegularWriter is a low-level log writer. It captures the pnt output
// as the JSON body and sends it with an empty header.
func (h *netHandler) RegularWriter(
	level nekomimi.LogLevel, pnt func(io.StringWriter),
) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cfg.Wrapper != nil {
		h.cfg.Wrapper.RegularWriter(level, pnt)
	}
	if h.conn == nil {
		return
	}
	var buf bytes.Buffer
	pnt(&buf)
	h.sendJSON(level, "", buf.String())
}

// PanicLog handles panic-level log messages. After sending the log,
// it panics unless WrapOnly is true. The lock is released via defer
// so it is always freed — even if sendJSON panics.
func (h *netHandler) PanicLog(header string, message ...any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cfg.Wrapper != nil {
		pnt := makePnt(header, message...)
		h.cfg.Wrapper.RegularWriter(nekomimi.PANIC, pnt)
	}
	if h.conn != nil {
		h.sendJSON(nekomimi.PANIC, header, fmt.Sprint(message...))
	}
	if !h.cfg.WrapOnly {
		panic(fmt.Sprint(message...))
	}
}

// FatalLog handles fatal-level log messages. After sending the log,
// it terminates the program via exitFunc unless WrapOnly is true.
// The lock is released via defer so it is always freed — even if
// sendJSON panics.  exitFunc is called after the defer stack unwinds.
func (h *netHandler) FatalLog(header string, message ...any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cfg.Wrapper != nil {
		pnt := makePnt(header, message...)
		h.cfg.Wrapper.RegularWriter(nekomimi.FATAL, pnt)
	}
	if h.conn != nil {
		h.sendJSON(nekomimi.FATAL, header, fmt.Sprint(message...))
	}
	if !h.cfg.WrapOnly {
		exitFunc(1)
	}
}
