// Package netlog provides a network log handler for nekomimi that
// sends JSON-formatted log messages over TCP or UDP.
//
// The handler directly implements the nekomimi.LogHandler interface
// to have full control over log level, header, and message body during
// JSON formatting. Each log entry is sent as a single NDJSON line:
//
//	{"level":"INFO","header":"2026-06-27 10:30:00.123 [INFO], ... - ","body":"hello"}
//
// TCP mode supports automatic reconnection when the connection drops.
// A background ticker attempts to reconnect every 2 seconds,
// and log messages are silently discarded while disconnected.
// UDP mode is connectionless with no reconnection logic.
//
// # Usage
//
//	handler, err := netlog.New(ctx, netlog.Config{
//	    Connect:  "tcp://127.0.0.1:28280",
//	    WrapOnly: true,
//	})
//	if err != nil {
//	    // handle error (URL parse failure, dial failure)
//	}
//	log := nekomimi.New("myapp", nekomimi.LogConfig{Handler: handler})
package netlog
