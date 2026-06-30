//go:build ignore

// Example: integrating nekomimi as Gin's log output via middleware.
//
// Requires: go get github.com/gin-gonic/gin
//
// The key abstraction is an io.Writer adapter that bridges nekomimi's
// io.StringWriter to Gin's log formatter.  Use log.GetWriter() at the
// desired log level.
//
// This pattern is adapted from fmcloud/daemon/apisrv.
package main

import (
	"github.com/fiathux/nekomimi"

	"github.com/gin-gonic/gin"
)

// ginLogWriter adapts nekomimi.Logger to io.Writer for Gin middleware.
type ginLogWriter struct {
	log nekomimi.Logger
}

// Write implements io.Writer.  Each Gin access-log line is routed through
// nekomimi at the configured level (here: INFO, no call-trace).
func (w *ginLogWriter) Write(p []byte) (int, error) {
	wrt := w.log.GetWriter(nekomimi.INFO, false)
	return wrt.WriteString(string(p))
}

func ExampleGinMiddleware() {
	// ---- standard integration ----
	componentLog := nekomimi.New("api", nekomimi.LogConfig{
		Level: nekomimi.INFO,
	})

	engine := gin.New()

	// Use Gin's built-in logger, but redirect output through nekomimi.
	engine.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		Output: &ginLogWriter{log: componentLog},
	}))

	// ---- optional: redirect Gin's internal debug prints ----
	debugLog := nekomimi.New("gin-debug", nekomimi.LogConfig{
		Level: nekomimi.DEBUG,
	})

	gin.DefaultWriter = &ginLogWriter{log: debugLog}

	gin.DebugPrintRouteFunc = func(
		method, path, handler string, numHandlers int,
	) {
		debugLog.Dbgf("%-6s %-25s → %s (%d handlers)",
			method, path, handler, numHandlers)
	}

	// ---- start server (demo only) ----
	engine.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Uncomment to run:
	// engine.Run(":8080")
	_ = engine
}

func main() {
	ExampleGinMiddleware()
}
