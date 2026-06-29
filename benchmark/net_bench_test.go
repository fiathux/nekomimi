package benchmark_test

import (
	"bufio"
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/fiathux/nekomimi"
	"github.com/fiathux/nekomimi/handlers/netlog"
)

// discardingTCPServer starts a TCP listener that accepts connections and
// discards all received data.  Returns the listener and the address.
func discardingTCPServer(b *testing.B) (addr string, lis net.Listener) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	addr = lis.Addr().String()
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				sc := bufio.NewScanner(conn)
				sc.Split(bufio.ScanLines)
				for sc.Scan() {
					// discard
				}
			}()
		}
	}()
	b.Cleanup(func() { lis.Close() })
	return
}

// discardingUDPServer starts a UDP listener that discards all received
// datagrams.  Returns the local address string.
func discardingUDPServer(b *testing.B) (addr string, conn *net.UDPConn) {
	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	conn, err = net.ListenUDP("udp", udpAddr)
	if err != nil {
		b.Fatal(err)
	}
	go func() {
		buf := make([]byte, 65535)
		for {
			_, _, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			// discard
		}
	}()
	b.Cleanup(func() { conn.Close() })
	return conn.LocalAddr().String(), conn
}

// --- TCP sequential ---

func BenchmarkNet_TCP_RegularLog(b *testing.B) {
	addr, _ := discardingTCPServer(b)
	ctx := context.Background()

	h, err := netlog.New(ctx, netlog.Config{
		Connect: "tcp://" + addr,
	})
	if err != nil {
		b.Fatal(err)
	}

	header := "2026-06-27 10:00:00.000 [INFO], bench - "
	msg := "benchmark log message"
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		h.RegularLog(nekomimi.INFO, header, msg)
	}
}

// --- TCP concurrent ---

func BenchmarkNet_TCP_RegularLog_Parallel(b *testing.B) {
	addr, _ := discardingTCPServer(b)
	ctx := context.Background()

	h, err := netlog.New(ctx, netlog.Config{
		Connect: "tcp://" + addr,
	})
	if err != nil {
		b.Fatal(err)
	}

	header := "2026-06-27 10:00:00.000 [INFO], bench - "
	msg := "benchmark log message"
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			h.RegularLog(nekomimi.INFO, header, msg)
		}
	})
}

// --- TCP RegularWriter ---

func BenchmarkNet_TCP_RegularWriter(b *testing.B) {
	addr, _ := discardingTCPServer(b)
	ctx := context.Background()

	h, err := netlog.New(ctx, netlog.Config{
		Connect: "tcp://" + addr,
	})
	if err != nil {
		b.Fatal(err)
	}

	msg := "benchmark log message"
	b.ResetTimer()
	b.SetBytes(int64(len(msg)))

	for i := 0; i < b.N; i++ {
		h.RegularWriter(nekomimi.INFO, func(w io.StringWriter) {
			w.WriteString(msg)
		})
	}
}

// --- UDP sequential ---

func BenchmarkNet_UDP_RegularLog(b *testing.B) {
	addr, _ := discardingUDPServer(b)
	ctx := context.Background()

	h, err := netlog.New(ctx, netlog.Config{
		Connect: "udp://" + addr,
	})
	if err != nil {
		b.Fatal(err)
	}
	// UDP dial is asynchronous, give it a moment to settle
	time.Sleep(10 * time.Millisecond)

	header := "2026-06-27 10:00:00.000 [INFO], bench - "
	msg := "benchmark log message"
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		h.RegularLog(nekomimi.INFO, header, msg)
	}
}
