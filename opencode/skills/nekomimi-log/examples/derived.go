//go:build ignore

// Example: logger initialization patterns — init() singleton, Derive()
// hierarchy.
package main

import (
	"github.com/fiathux/nekomimi"
)

// ---- pattern 1: package-level singleton via init() ----
// For utility packages that need a global logger.
// nekomimi.New does no I/O until the first write, so it is safe in init().

var pkgLog nekomimi.Logger

func init() {
	pkgLog = nekomimi.New("mypkg", nekomimi.LogConfig{
		Level:          nekomimi.INFO,
		LevelWithTrace: nekomimi.ERROR,
	})
}

// ---- pattern 2: standalone instance with custom config ----
// Each struct holds its own logger with independent settings.

type Service struct {
	log nekomimi.Logger
}

func NewService() *Service {
	return &Service{
		log: nekomimi.New("Svc", nekomimi.LogConfig{
			Level: nekomimi.WARN,
		}),
	}
}

func (s *Service) Start() {
	s.log.Inf("service started")
}

// ---- pattern 3: hierarchical loggers via Derive() ----
// One root logger, component loggers inherit config with a dotted prefix.

func ExampleDerive() {
	root := nekomimi.New("App", nekomimi.LogConfig{
		Level: nekomimi.DEBUG,
	})

	dbLog := root.Derive("Database")
	apiLog := root.Derive("API")

	apiLog.Inf("listening on :8080")
	// output: [INFO], App.API - listening on :8080

	dbLog.War("slow query (3.2s)")
	// output: [WARN], App.Database - slow query (3.2s)

	// children can override their log level independently
	dbLog.SetLevel(nekomimi.ERROR)
	dbLog.Inf("this INFO is now silenced")
}

func main() {
	pkgLog.Inf("package-level logger works")

	svc := NewService()
	svc.Start()

	ExampleDerive()
}
