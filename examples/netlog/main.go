// Example: network log handler
//
// Demonstrates the netlog handler sending JSON-formatted logs over TCP
// and UDP.  A local echo server is started so the example can run without
// external dependencies.
package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fiathux/nekomimi"
	"github.com/fiathux/nekomimi/handlers/netlog"
)

func main() {
	ctx := context.Background()

	// --- start a local TCP echo server for demonstration ---
	tcpAddr, tcpDone := startTCPEchoServer()
	defer tcpDone()

	// --- start a local UDP echo server for demonstration ---
	_ = startUDPEchoServer()

	// --- TCP handler ---
	fmt.Println("\n=== TCP Network Handler ===")
	tcpHandler, err := netlog.New(ctx, netlog.Config{
		Connect:  "tcp://" + tcpAddr,
		WrapOnly: true, // transport only, don't crash
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "tcp handler: %v\n", err)
		os.Exit(1)
	}

	// Wrap with native handler for console output.
	tcpLogger := nekomimi.New("TCPDemo", nekomimi.LogConfig{
		Handler: nekomimi.NewNativeLogHandler(tcpHandler),
	})

	tcpLogger.Inf("TCP log demo started")
	tcpLogger.War("this is a warning via TCP")
	tcpLogger.Dbg("debug message over the wire")

	// --- UDP handler ---
	fmt.Println("\n=== UDP Network Handler ===")
	udpHandler, err := netlog.New(ctx, netlog.Config{
		Connect:  "udp://127.0.0.1:28280",
		WrapOnly: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "udp handler: %v\n", err)
		os.Exit(1)
	}

	udpLogger := nekomimi.New("UDPDemo", nekomimi.LogConfig{
		Handler: nekomimi.NewNativeLogHandler(udpHandler),
	})

	udpLogger.Inf("UDP log demo started")
	udpLogger.War("this is a warning via UDP")
	udpLogger.Dbg("debug message via UDP")

	// --- demonstrate handler composition: file + network ---
	fmt.Println("\n=== Composition: Network handler wraps native handler ===")
	composedHandler, _ := netlog.New(ctx, netlog.Config{
		Connect:  "tcp://" + tcpAddr,
		WrapOnly: true,
	})
	composed := nekomimi.New("Composed", nekomimi.LogConfig{
		Handler: nekomimi.NewNativeLogHandler(composedHandler),
	})
	composed.Inf("this goes to console AND TCP")
	composed.Err("error reported to both targets")

	time.Sleep(200 * time.Millisecond)
	fmt.Println("\nNetwork log demo finished.")
}

// startTCPEchoServer starts a TCP server on a random port that echoes
// received JSON lines to stdout.  Returns the address and a stop function.
func startTCPEchoServer() (addr string, done func()) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	addr = lis.Addr().String()

	stop := make(chan struct{})
	done = func() {
		close(stop)
		lis.Close()
	}

	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				sc := bufio.NewScanner(conn)
				for sc.Scan() {
					fmt.Printf("  [TCP received] %s\n", sc.Text())
				}
			}()
		}
	}()

	// trap SIGINT for clean server shutdown
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sig:
			lis.Close()
		case <-stop:
		}
	}()

	return addr, done
}

// startUDPEchoServer starts a UDP server on port 28280 that echoes
// received datagrams to stdout.
func startUDPEchoServer() (done func()) {
	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:28280")
	if err != nil {
		panic(err)
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		panic(err)
	}

	stop := make(chan struct{})
	done = func() {
		close(stop)
		conn.Close()
	}

	go func() {
		buf := make([]byte, 65535)
		for {
			select {
			case <-stop:
				return
			default:
				conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
				n, _, err := conn.ReadFromUDP(buf)
				if err != nil {
					continue
				}
				fmt.Printf("  [UDP received] %s\n", string(buf[:n]))
			}
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sig:
			conn.Close()
		case <-stop:
		}
	}()

	return done
}
