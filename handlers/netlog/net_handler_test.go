package netlog

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fiathux/nekomimi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ------- test helpers -------

// serveTCP starts a TCP server on a random port. Returns the address
// string (without scheme), the listener, and a channel receiving all
// data read from accepted connections.
func serveTCP(t *testing.T) (addr string, lis net.Listener, data chan string) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr = lis.Addr().String()
	data = make(chan string, 1024)
	done := make(chan struct{})

	go func() {
		for {
			conn, aerr := lis.Accept()
			if aerr != nil {
				select {
				case <-done:
					return
				default:
					continue
				}
			}
			go func(c net.Conn) {
				defer c.Close()
				sc := bufio.NewScanner(c)
				for sc.Scan() {
					select {
					case data <- sc.Text():
					case <-done:
						return
					}
				}
			}(conn)
		}
	}()
	t.Cleanup(func() {
		close(done)
		lis.Close()
	})
	return
}

// serveTCPNoRead starts a TCP server that accepts but never reads.
// Returns the address and the conn channel for simulating slow reader.
func serveTCPNoRead(t *testing.T) (addr string, lis net.Listener,
	connCh chan net.Conn) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr = lis.Addr().String()
	connCh = make(chan net.Conn, 1)
	done := make(chan struct{})

	go func() {
		for {
			conn, aerr := lis.Accept()
			if aerr != nil {
				select {
				case <-done:
					return
				default:
					continue
				}
			}
			select {
			case connCh <- conn:
			case <-done:
				conn.Close()
				return
			}
		}
	}()
	t.Cleanup(func() {
		close(done)
		lis.Close()
	})
	return
}

// serveUDP starts a UDP server on a random port.
func serveUDP(t *testing.T) (addr string, conn *net.UDPConn, data chan string) {
	t.Helper()
	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)
	conn, err = net.ListenUDP("udp", udpAddr)
	require.NoError(t, err)
	addr = conn.LocalAddr().String()
	data = make(chan string, 1024)
	done := make(chan struct{})

	go func() {
		buf := make([]byte, 65536)
		for {
			n, _, rerr := conn.ReadFromUDP(buf)
			if rerr != nil {
				select {
				case <-done:
					return
				default:
					continue
				}
			}
			for _, line := range strings.Split(
				strings.TrimSuffix(string(buf[:n]), "\n"), "\n",
			) {
				if line != "" {
					select {
					case data <- line:
					case <-done:
						return
					}
				}
			}
		}
	}()
	t.Cleanup(func() {
		close(done)
		conn.Close()
	})
	return
}

// jsonEntry is the deserialized form of a received NDJSON line.
type jsonEntry struct {
	Level  string `json:"level"`
	Header string `json:"header"`
	Body   string `json:"body"`
}

// recvJSON reads one log entry from the channel with a timeout.
func recvJSON(t *testing.T, data <-chan string) jsonEntry {
	t.Helper()
	select {
	case raw := <-data:
		var e jsonEntry
		err := json.Unmarshal([]byte(raw), &e)
		require.NoError(t, err, "invalid JSON: %s", raw)
		return e
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for log entry")
		return jsonEntry{}
	}
}

// assertNoData asserts that no data arrives within the given duration.
func assertNoData(t *testing.T, data <-chan string, d time.Duration) {
	t.Helper()
	select {
	case raw := <-data:
		t.Fatalf("unexpected data received: %s", raw)
	case <-time.After(d):
	}
}

// recvAllJSON reads all available entries from the channel.
func recvAllJSON(t *testing.T, data <-chan string, count int) []jsonEntry {
	t.Helper()
	var entries []jsonEntry
	for range count {
		entries = append(entries, recvJSON(t, data))
	}
	return entries
}

// ------- mock wrapper -------

// mockHandler implements nekomimi.LogHandler to record calls for
// wrapper forwarding tests.
type mockHandler struct {
	regularLogCount    atomic.Int32
	regularWriterCount atomic.Int32
	panicedLevel       nekomimi.LogLevel
	mu                 sync.Mutex
}

func (m *mockHandler) RegularLog(
	level nekomimi.LogLevel, header string, message ...any,
) {
	m.regularLogCount.Add(1)
}

func (m *mockHandler) RegularWriter(
	level nekomimi.LogLevel, pnt func(io.StringWriter),
) {
	m.regularWriterCount.Add(1)
	m.mu.Lock()
	m.panicedLevel = level
	m.mu.Unlock()
}

func (m *mockHandler) PanicLog(header string, message ...any) {}

func (m *mockHandler) FatalLog(header string, message ...any) {}

// ------- tests -------

// newTestContext creates a context that is automatically cancelled when
// the test finishes.  This ensures background goroutines started by
// netlog handlers are properly terminated, preventing data races on
// package-level variables between sequential tests.
func newTestContext(t *testing.T) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return ctx, cancel
}

func TestNew_TCPConnects(t *testing.T) {
	addr, _, _ := serveTCP(t)
	ctx, _ := newTestContext(t)

	h, err := New(ctx, Config{Connect: "tcp://" + addr})
	require.NoError(t, err)
	require.NotNil(t, h)
}

func TestNew_UDPConnects(t *testing.T) {
	_, conn, _ := serveUDP(t)
	addr := conn.LocalAddr().String()
	ctx, _ := newTestContext(t)

	h, err := New(ctx, Config{Connect: "udp://" + addr})
	require.NoError(t, err)
	require.NotNil(t, h)
}

func TestNew_ConnectFailed(t *testing.T) {
	// pick a port that is likely not in use
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := lis.Addr().String()
	lis.Close() // close immediately so Dial fails

	ctx, _ := newTestContext(t)
	h, err := New(ctx, Config{Connect: "tcp://" + port})
	assert.Error(t, err)
	assert.Nil(t, h)
}

func TestNew_InvalidURL(t *testing.T) {
	ctx, _ := newTestContext(t)
	h, err := New(ctx, Config{Connect: "not-a-valid-url"})
	assert.Error(t, err)
	assert.Nil(t, h)
}

func TestNew_UnsupportedScheme(t *testing.T) {
	ctx, _ := newTestContext(t)
	h, err := New(ctx, Config{
		Connect: "http://127.0.0.1:8080",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported scheme")
	assert.Nil(t, h)
}

func TestRegularLog_SendsJSON_TCP(t *testing.T) {
	addr, _, data := serveTCP(t)
	ctx, _ := newTestContext(t)

	h, err := New(ctx, Config{Connect: "tcp://" + addr})
	require.NoError(t, err)

	h.RegularLog(nekomimi.INFO,
		"2026-06-27 10:00:00.000 [INFO], myapp - ",
		"hello world",
	)

	e := recvJSON(t, data)
	assert.Equal(t, "INFO", e.Level)
	assert.Equal(t,
		"2026-06-27 10:00:00.000 [INFO], myapp - ",
		e.Header,
	)
	assert.Equal(t, "hello world", e.Body)
}

func TestRegularLog_SendsJSON_UDP(t *testing.T) {
	_, udpConn, data := serveUDP(t)
	addr := udpConn.LocalAddr().String()
	ctx, _ := newTestContext(t)

	h, err := New(ctx, Config{Connect: "udp://" + addr})
	require.NoError(t, err)

	h.RegularLog(nekomimi.DEBUG, "h - ", "debug msg")

	// UDP may coalesce multiple writes into one datagram, but here
	// we send just one line.
	e := recvJSON(t, data)
	assert.Equal(t, "DEBUG", e.Level)
	assert.Equal(t, "h - ", e.Header)
	assert.Equal(t, "debug msg", e.Body)
}

func TestRegularLog_AllLevels(t *testing.T) {
	addr, _, data := serveTCP(t)
	ctx, _ := newTestContext(t)

	h, err := New(ctx, Config{Connect: "tcp://" + addr})
	require.NoError(t, err)

	levels := []nekomimi.LogLevel{
		nekomimi.DEBUG, nekomimi.INFO, nekomimi.WARN,
		nekomimi.ERROR, nekomimi.PANIC, nekomimi.FATAL,
	}
	for _, lv := range levels {
		h.RegularLog(lv, "h - ", "msg")
	}

	entries := recvAllJSON(t, data, len(levels))
	expected := []string{
		"DEBUG", "INFO", "WARN", "ERROR", "PANIC", "FATAL",
	}
	for i, e := range entries {
		assert.Equal(t, expected[i], e.Level,
			"level mismatch at index %d", i)
	}
}

func TestRegularWriter_CapturesOutputAsBody(t *testing.T) {
	addr, _, data := serveTCP(t)
	ctx, _ := newTestContext(t)

	h, err := New(ctx, Config{Connect: "tcp://" + addr})
	require.NoError(t, err)

	h.RegularWriter(nekomimi.INFO,
		func(w io.StringWriter) {
			w.WriteString("raw header raw body\n")
		},
	)

	e := recvJSON(t, data)
	assert.Equal(t, "INFO", e.Level)
	assert.Equal(t, "", e.Header) // header must be empty
	assert.Equal(t, "raw header raw body", e.Body)
}

func TestRegularLog_HeaderContainsTraceInfo(t *testing.T) {
	addr, _, data := serveTCP(t)
	ctx, _ := newTestContext(t)

	h, err := New(ctx, Config{Connect: "tcp://" + addr})
	require.NoError(t, err)

	complexHeader := "2026-06-27 [PANIC], app  >> Stacks:\n" +
		"    /app/main.go:42(DoStuff)\n<<<< - "

	h.RegularLog(nekomimi.PANIC, complexHeader, "crash")

	e := recvJSON(t, data)
	assert.Equal(t, "PANIC", e.Level)
	assert.Equal(t, complexHeader, e.Header) // header fully preserved
	assert.Equal(t, "crash", e.Body)
}

func TestTCP_DisconnectAndReconnect(t *testing.T) {
	origInterval := reconnectInterval
	reconnectInterval = 50 * time.Millisecond
	defer func() { reconnectInterval = origInterval }()

	addr, _, data := serveTCP(t)
	ctx, _ := newTestContext(t)

	h, err := New(ctx, Config{Connect: "tcp://" + addr})
	require.NoError(t, err)

	// send: should arrive
	h.RegularLog(nekomimi.INFO, "h - ", "before-disconnect")
	e1 := recvJSON(t, data)
	assert.Equal(t, "before-disconnect", e1.Body)

	// force disconnect by closing conn from test side
	nh := h.(*netHandler)
	nh.mu.Lock()
	nh.conn.Close()
	nh.conn = nil
	nh.mu.Unlock()

	// send while disconnected: should be dropped
	h.RegularLog(nekomimi.INFO, "h - ", "during-disconnect")
	assertNoData(t, data, 200*time.Millisecond)

	// wait for ticker to reconnect to the same server
	time.Sleep(200 * time.Millisecond)

	// send after reconnect: should arrive
	h.RegularLog(nekomimi.INFO, "h - ", "after-reconnect")
	e2 := recvJSON(t, data)
	assert.Equal(t, "after-reconnect", e2.Body)
}

func TestTCP_WriteDeadlineTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow deadline timeout test in short mode")
	}
	addr, lis, connCh := serveTCPNoRead(t)
	defer lis.Close()

	ctx, _ := newTestContext(t)

	h, err := New(ctx, Config{Connect: "tcp://" + addr})
	require.NoError(t, err)

	// grab the accepted connection but don't read from it
	conn := <-connCh
	defer conn.Close()

	// reduce send buffer to trigger deadline quickly
	nh := h.(*netHandler)
	nh.mu.Lock()
	if tcpConn, ok := nh.conn.(*net.TCPConn); ok {
		tcpConn.SetWriteBuffer(1)
	}
	nh.mu.Unlock()

	// write large payloads until write blocks and deadline triggers
	disconnected := false
	start := time.Now()
	payload := strings.Repeat("x", 4096) // 4KB per log fills buffer
	for time.Since(start) < 5*time.Second {
		h.RegularLog(nekomimi.INFO, "h - ", payload)
		nh.mu.Lock()
		if nh.conn == nil {
			disconnected = true
			nh.mu.Unlock()
			break
		}
		nh.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	assert.True(t, disconnected,
		"expected connection to be marked disconnected after deadline")
}

func TestUDP_WriteFailureSilent(t *testing.T) {
	_, udpConn, _ := serveUDP(t)
	addr := udpConn.LocalAddr().String()
	ctx, _ := newTestContext(t)

	h, err := New(ctx, Config{Connect: "udp://" + addr})
	require.NoError(t, err)

	// close the UDP server to simulate no receiver
	udpConn.Close()

	// should not panic or error
	h.RegularLog(nekomimi.INFO, "h - ", "log after close")
}

func TestPanicLog_WrapOnlyFalse(t *testing.T) {
	addr, _, data := serveTCP(t)
	ctx, _ := newTestContext(t)

	h, err := New(ctx, Config{
		Connect:  "tcp://" + addr,
		WrapOnly: false,
	})
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		assert.Panics(t, func() {
			h.PanicLog("h - ", "panic msg")
		})
	}()

	<-done
	// verify JSON was sent before panic
	e := recvJSON(t, data)
	assert.Equal(t, "PANIC", e.Level)
	assert.Equal(t, "panic msg", e.Body)
}

func TestPanicLog_WrapOnlyTrue(t *testing.T) {
	addr, _, data := serveTCP(t)
	ctx, _ := newTestContext(t)

	h, err := New(ctx, Config{
		Connect:  "tcp://" + addr,
		WrapOnly: true,
	})
	require.NoError(t, err)

	// should not panic
	h.PanicLog("h - ", "panic msg")

	e := recvJSON(t, data)
	assert.Equal(t, "PANIC", e.Level)
	assert.Equal(t, "panic msg", e.Body)
}

func TestFatalLog_WrapOnlyFalse(t *testing.T) {
	addr, _, data := serveTCP(t)
	ctx, _ := newTestContext(t)

	h, err := New(ctx, Config{
		Connect:  "tcp://" + addr,
		WrapOnly: false,
	})
	require.NoError(t, err)

	var exitCode int
	origExit := exitFunc
	exitFunc = func(code int) { exitCode = code; panic("mock-exit") }
	defer func() { exitFunc = origExit }()

	assert.PanicsWithValue(t, "mock-exit", func() {
		h.FatalLog("h - ", "fatal msg")
	})
	assert.Equal(t, 1, exitCode)

	e := recvJSON(t, data)
	assert.Equal(t, "FATAL", e.Level)
	assert.Equal(t, "fatal msg", e.Body)
}

func TestFatalLog_WrapOnlyTrue(t *testing.T) {
	addr, _, data := serveTCP(t)
	ctx, _ := newTestContext(t)

	h, err := New(ctx, Config{
		Connect:  "tcp://" + addr,
		WrapOnly: true,
	})
	require.NoError(t, err)

	exitCalled := false
	origExit := exitFunc
	exitFunc = func(int) { exitCalled = true }
	defer func() { exitFunc = origExit }()

	// should not call exit
	h.FatalLog("h - ", "fatal msg")
	assert.False(t, exitCalled)

	e := recvJSON(t, data)
	assert.Equal(t, "FATAL", e.Level)
	assert.Equal(t, "fatal msg", e.Body)
}

func TestPanicLog_ConnDown_StillPanics(t *testing.T) {
	// create handler connected to a real server, then close it
	addr, lis, _ := serveTCP(t)
	ctx, _ := newTestContext(t)

	h, err := New(ctx, Config{
		Connect:  "tcp://" + addr,
		WrapOnly: false,
	})
	require.NoError(t, err)

	// close server → disconnect
	lis.Close()

	// send a log to trigger disconnect
	h.RegularLog(nekomimi.INFO, "h - ", "trigger-disconnect")
	time.Sleep(100 * time.Millisecond)

	// conn should be nil now, but panic should still fire
	assert.Panics(t, func() {
		h.PanicLog("h - ", "panic")
	})
}

func TestWrapper_ForwardsRegularLog(t *testing.T) {
	addr, _, data := serveTCP(t)
	ctx, _ := newTestContext(t)

	mock := &mockHandler{}

	h, err := New(ctx, Config{
		Connect: "tcp://" + addr,
		Wrapper: mock,
	})
	require.NoError(t, err)

	h.RegularLog(nekomimi.INFO, "h - ", "msg")

	assert.Equal(t, int32(1), mock.regularLogCount.Load())

	e := recvJSON(t, data)
	assert.Equal(t, "INFO", e.Level)
	assert.Equal(t, "msg", e.Body)
}

func TestWrapper_PanicForwardsAsRegular(t *testing.T) {
	addr, _, data := serveTCP(t)
	ctx, _ := newTestContext(t)

	mock := &mockHandler{}

	h, err := New(ctx, Config{
		Connect:  "tcp://" + addr,
		WrapOnly: true,
		Wrapper:  mock,
	})
	require.NoError(t, err)

	h.PanicLog("h - ", "panic msg")

	assert.Equal(t, int32(1), mock.regularWriterCount.Load())
	assert.Equal(t, nekomimi.PANIC, mock.panicedLevel)

	e := recvJSON(t, data)
	assert.Equal(t, "PANIC", e.Level)
	assert.Equal(t, "panic msg", e.Body)
}

func TestContextCancel_StopsTicker(t *testing.T) {
	addr, _, _ := serveTCP(t)
	ctx, cancel := context.WithCancel(context.Background())

	h, err := New(ctx, Config{Connect: "tcp://" + addr})
	require.NoError(t, err)

	nh := h.(*netHandler)
	require.NotNil(t, nh.conn)

	cancel()

	// give some time for the ticker goroutine to clean up
	time.Sleep(100 * time.Millisecond)

	nh.mu.Lock()
	isNil := nh.conn == nil
	nh.mu.Unlock()
	assert.True(t, isNil, "conn should be nil after context cancel")

	// subsequent writes should be dropped without panic
	h.RegularLog(nekomimi.INFO, "h - ", "after-cancel")
}

func TestContextCancel_UDP_ClosesConn(t *testing.T) {
	_, udpConn, _ := serveUDP(t)
	addr := udpConn.LocalAddr().String()
	ctx, cancel := context.WithCancel(context.Background())

	h, err := New(ctx, Config{Connect: "udp://" + addr})
	require.NoError(t, err)

	cancel()
	time.Sleep(100 * time.Millisecond)

	nh := h.(*netHandler)
	nh.mu.Lock()
	isNil := nh.conn == nil
	nh.mu.Unlock()

	assert.True(t, isNil, "conn should be nil after cancel")

	// writes after cancel should be dropped silently
	h.RegularLog(nekomimi.INFO, "h - ", "after-cancel")
}

func TestConcurrent_TCP(t *testing.T) {
	addr, _, data := serveTCP(t)
	ctx, _ := newTestContext(t)

	h, err := New(ctx, Config{Connect: "tcp://" + addr})
	require.NoError(t, err)

	const goroutines = 10
	const msgPerRoutine = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := range goroutines {
		go func(id int) {
			defer wg.Done()
			for i := range msgPerRoutine {
				h.RegularLog(nekomimi.INFO,
					fmt.Sprintf("h%d - ", id),
					fmt.Sprintf("msg-%d-%d", id, i),
				)
			}
		}(g)
	}
	wg.Wait()

	entries := recvAllJSON(t, data, goroutines*msgPerRoutine)
	assert.Len(t, entries, goroutines*msgPerRoutine)
	for _, e := range entries {
		assert.Equal(t, "INFO", e.Level)
		assert.NotEmpty(t, e.Body)
	}
}

func TestConcurrent_UDP(t *testing.T) {
	_, udpConn, data := serveUDP(t)
	addr := udpConn.LocalAddr().String()
	ctx, _ := newTestContext(t)

	h, err := New(ctx, Config{Connect: "udp://" + addr})
	require.NoError(t, err)

	const goroutines = 10
	const msgPerRoutine = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := range goroutines {
		go func(id int) {
			defer wg.Done()
			for i := range msgPerRoutine {
				h.RegularLog(nekomimi.INFO,
					"h - ",
					fmt.Sprintf("msg-%d-%d", id, i),
				)
			}
		}(g)
	}
	wg.Wait()

	// read at least some entries (UDP may drop packets under load
	// on loopback, but we should get most)
	count := 0
	timeout := time.After(3 * time.Second)
loop:
	for range goroutines * msgPerRoutine {
		select {
		case raw := <-data:
			var e jsonEntry
			if err := json.Unmarshal([]byte(raw), &e); err == nil {
				count++
			}
		case <-timeout:
			break loop
		}
	}
	t.Logf("received %d/%d UDP log entries", count,
		goroutines*msgPerRoutine)
	assert.Greater(t, count, 0,
		"should receive at least some UDP log entries")
}
