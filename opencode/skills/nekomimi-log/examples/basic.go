//go:build ignore

// Example: basic nekomimi usage — init, three method families, dynamic
// level changes.
package main

import (
	"github.com/fiathux/nekomimi"
)

func main() {
	// ---- bare-minimum logger ----
	log := nekomimi.New("basic", nekomimi.LogConfig{})

	log.Dbg("transient debug info")
	log.Inf("service started")
	log.War("deprecated endpoint called")
	log.Err("upstream timeout", "after", "3s")

	// ---- three method families ----
	name := "alice"
	count := 42

	log.Inf("plain: user", name, "processed", count)              // Plain
	log.Inff("formatted: user %s processed %d items", name, count) // Formatted

	if fn := log.DbgP(); fn != nil {                               // Deferred
		expensive := computeExpensiveData()
		fn("deferred debug:", expensive)
	}

	// ---- dynamic level adjustment ----
	log.SetLevel(nekomimi.WARN)
	log.Inf("this INFO line is silently dropped")
	log.War("this WARNING line still appears")

	// ---- call-trace threshold ----
	traced := nekomimi.New("traced", nekomimi.LogConfig{
		Level:          nekomimi.DEBUG,
		LevelWithTrace: nekomimi.WARN,
	})
	traced.Inf("no file:line in header")
	traced.War("file:line(func) appears in header")
}

func computeExpensiveData() string {
	// simulate work that takes time
	buf := make([]byte, 0, 128)
	for i := 0; i < 1000; i++ {
		buf = append(buf, byte('a'+i%26))
	}
	return string(buf)
}
