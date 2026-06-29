// Package filerotate provides a file rotation log handler for nekomimi.
//
// The handler writes log messages to files with automatic rotation based on
// file size, item count, or TTL. Rotated files are archived with timestamped
// names and can be compressed with gzip. The handler supports a three-state
// state machine (active, suspended, closed) and provides crash-safe operations
// for panic and fatal log levels.
//
// # Features
//
//   - Automatic rotation by max file size, max log entries, or max file TTL
//   - Timestamp-based archive naming (prefix_yymmdd_seconds.log)
//   - Optional gzip compression of archived files
//   - Fallback file naming when primary name is unavailable
//   - Suspended state with automatic audit recovery
//   - Synchronous panic/fatal writes with forced fsync before crash
//
// # Usage
//
//	handler, err := filerotate.New(ctx, filerotate.Config{
//	    Path:         "/var/log/myapp",
//	    FilePrefix:   "app",
//	    MaxFileSize:  10240,   // 10 MB
//	    MaxFileItems: 100000,
//	    MaxFileTTL:   1440,    // 24 hours
//	    MaxArchives:  30,
//	    Compress:     true,
//	})
//	if err != nil {
//	    // handle error
//	}
//	log := nekomimi.New("myapp", nekomimi.LogConfig{Handler: handler})
package filerotate
