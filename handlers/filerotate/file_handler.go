package filerotate

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fiathux/nekomimi"
)

// handler state constants
const (
	stateActive    int32 = iota // normal log writing
	stateSuspended              // writes dropped, audit retries
	stateClosed                 // ctx cancelled, resources released
)

// default intervals
const (
	defaultTickInterval  = 10 * time.Second
	defaultAuditInterval = 6 // every 6 ticks ≈ 60s
	maxFallbackSlots     = 5
)

// testForceFailNewFile is a test-only counter. When > 0, openNewFile
// decrements the counter and returns an error. When <= 0, it works
// normally. Tests use this to simulate failures for specific attempts.
var testForceFailNewFileCount atomic.Int32

// exitFunc is the function used for program termination.
// Replaced in tests to verify FatalLog behavior.
var exitFunc = os.Exit

// Config defines the configuration for the file rotation log handler.
// All size/value fields use 0 to mean "unlimited".
type Config struct {
	// Path is the directory where log files are stored.
	Path string
	// FilePrefix is the prefix for log file names (e.g. "app" → "app.log").
	FilePrefix string
	// MaxFileSize is the maximum size of a single log file in KB.
	// 0 means no limit.
	MaxFileSize int64
	// MaxFileItems is the maximum number of log entries in a single file.
	// Values below 100 are treated as 100. 0 means no limit.
	MaxFileItems int64
	// MaxFileTTL is the maximum lifetime of a single log file in minutes.
	// 0 means no limit.
	MaxFileTTL int64
	// MaxArchives is the maximum number of archived log files to retain.
	// When exceeded, the oldest archives are deleted. 0 means no limit.
	MaxArchives int
	// Compress enables gzip compression for archived log files.
	Compress bool
	// RotatePanic causes a panic if log file rotation fails.
	// When false, the handler suspends writes instead of crashing.
	RotatePanic bool
	// WrapOnly disables panic/exit behavior in PanicLog and FatalLog.
	// The handler only writes log messages without triggering program
	// termination. Useful when nested inside another handler chain.
	WrapOnly bool
	// Wrapper is an optional LogHandler that receives log messages before
	// this handler does.
	Wrapper nekomimi.LogHandler

	// testTickCh is an optional channel for triggering ticker events in
	// tests. When set, the handler uses this channel instead of a real
	// time.Ticker. Only used by tests in the same package.
	testTickCh chan time.Time
	// testTickInterval overrides the tick interval for tests.
	testTickInterval time.Duration
	// testAuditInterval overrides the number of ticks between audits.
	testAuditInterval int
	// testForceCreationTime overrides the file creation time used for
	// TTL rotation checks in tests.
	testForceCreationTime time.Time
}

// handler implements the file rotation log handler using LogHandlerFunc.
type handler struct {
	// cfg is the immutable configuration
	cfg Config

	// mu protects all mutable state. Also assigned as LogHandlerFunc.Lock.
	mu sync.Mutex

	// fp is the current log file handle. nil when suspended or closed.
	fp *os.File
	// state tracks the handler lifecycle
	state int32
	// byteCount tracks bytes written to the current file
	byteCount int64
	// itemCount tracks log entries written to the current file
	itemCount int64
	// fileCreatedAt records when the current file was opened
	fileCreatedAt time.Time
	// currentName is the basename of the current log file
	currentName string
	// lastFlushCount tracks byteCount at last flush
	lastFlushCount int64

	// ticker bookkeeping
	tickCount int

	// compressWG waits for ongoing compression goroutines on shutdown
	compressWG sync.WaitGroup

	// compressing tracks archive timestamp keys currently being gzipped.
	// cleanArchives snapshots this set to skip entries whose source file
	// is still being read by a compression goroutine.  Key format:
	// "yymmdd_seconds" (e.g. "260627_45045").  Protected by mu.
	compressing map[string]struct{}

	// test-only: tick interval override
	tickInterval time.Duration
	// test-only: audit interval override (in ticks)
	auditInterval int
}

// countWriter wraps an io.StringWriter and counts bytes written.
type countWriter struct {
	w     io.StringWriter
	count *int64
}

// WriteString writes string data and increments the byte counter.
func (cw *countWriter) WriteString(s string) (int, error) {
	n, err := cw.w.WriteString(s)
	*cw.count += int64(n)
	return n, err
}

// New creates a new file rotation log handler. It returns an error if
// the target directory cannot be created or the log file cannot be opened.
// The ctx controls the lifetime of background tasks (ticker, compression).
func New(ctx context.Context, cfg Config) (nekomimi.LogHandler, error) {
	if cfg.FilePrefix == "" {
		return nil, fmt.Errorf("filerotate: FilePrefix must not be empty")
	}
	if cfg.Path == "" {
		return nil, fmt.Errorf("filerotate: Path must not be empty")
	}
	// Clamp MaxFileItems
	if cfg.MaxFileItems > 0 && cfg.MaxFileItems < 100 {
		cfg.MaxFileItems = 100
	}
	if cfg.MaxFileTTL < 0 {
		cfg.MaxFileTTL = 0
	}
	if cfg.MaxFileSize < 0 {
		cfg.MaxFileSize = 0
	}

	ti := defaultTickInterval
	if cfg.testTickInterval > 0 {
		ti = cfg.testTickInterval
	}
	ai := defaultAuditInterval
	if cfg.testAuditInterval > 0 {
		ai = cfg.testAuditInterval
	}
	h := &handler{
		cfg:           cfg,
		tickInterval:  ti,
		auditInterval: ai,
		compressing:   make(map[string]struct{}),
	}

	// Ensure target directory exists
	if err := os.MkdirAll(cfg.Path, 0o755); err != nil {
		return nil, fmt.Errorf(
			"filerotate: create log directory %s: %w", cfg.Path, err)
	}

	// Scan and archive all residual log files
	h.scanAndArchive()

	// Create initial log file
	if err := h.createLogFile(); err != nil {
		return nil, fmt.Errorf("filerotate: create log file: %w", err)
	}

	// Build LogHandlerFunc
	lhf := &nekomimi.LogHandlerFunc{
		Lock:           &h.mu,
		RegularLogFunc: h.regularLogFunc,
		PanicLogFunc:   h.panicLogFunc,
		FatalLogFunc:   h.fatalLogFunc,
		Wrapper:        cfg.Wrapper,
	}

	// Start background goroutine
	go h.tickerLoop(ctx)

	return lhf, nil
}

// scanAndArchive lists all residual log files and archives them.
func (h *handler) scanAndArchive() {
	entries, err := os.ReadDir(h.cfg.Path)
	if err != nil {
		return // silent
	}
	for _, entry := range entries {
		name := entry.Name()
		if !h.matchLogFile(name) {
			continue
		}
		h.archiveFile(name)
	}
}

// archiveFile renames a log file to its timestamp-based archive name and
// optionally starts background gzip compression.
func (h *handler) archiveFile(name string) {
	srcPath := filepath.Join(h.cfg.Path, name)
	ts := h.extractTimestamp(srcPath)
	t := time.Unix(ts, 0).UTC()
	archiveName := fmt.Sprintf("%s_%s_%d.log",
		h.cfg.FilePrefix,
		t.Format("060102"),
		ts%86400,
	)
	dstPath := filepath.Join(h.cfg.Path, archiveName)

	if err := os.Rename(srcPath, dstPath); err != nil {
		return // silent ignore
	}

	if h.cfg.Compress {
		h.compressWG.Add(1)
		go func() {
			defer h.compressWG.Done()
			h.compressFile(dstPath)
		}()
	}
}

// extractTimestamp reads the first line of a file to extract a creation
// timestamp. Falls back to file mtime on failure.
func (h *handler) extractTimestamp(path string) int64 {
	f, err := os.Open(path)
	if err != nil {
		return h.fallbackMtime(path)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return h.fallbackMtime(path)
	}
	line := sc.Text()

	if strings.HasPrefix(line, "#created:") {
		ts, err := strconv.ParseInt(line[len("#created:"):], 10, 64)
		if err == nil {
			return ts
		}
	}
	return h.fallbackMtime(path)
}

// fallbackMtime returns the file's mtime as unix seconds.
func (h *handler) fallbackMtime(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return time.Now().Unix()
	}
	return fi.ModTime().Unix()
}

// creationTime returns the current time to use as the file creation
// timestamp.  During tests, Config.testForceCreationTime can override
// this for predictable TTL rotation checks.
func (h *handler) creationTime() time.Time {
	if !h.cfg.testForceCreationTime.IsZero() {
		return h.cfg.testForceCreationTime
	}
	return time.Now()
}

// createLogFile opens the initial log file for writing. Tries main name
// first, then fallback names. Returns error if all attempts fail.
func (h *handler) createLogFile() error {
	now := h.creationTime()

	// Try main name
	path := filepath.Join(h.cfg.Path, h.cfg.FilePrefix+".log")
	fp, err := h.openNewFile(path, now)
	if err == nil {
		h.fp = fp
		h.currentName = h.cfg.FilePrefix + ".log"
		h.fileCreatedAt = now
		h.state = stateActive
		return nil
	}

	// Fallback names
	for n := 1; n <= maxFallbackSlots; n++ {
		name := fmt.Sprintf("%s_%d.log", h.cfg.FilePrefix, n)
		path = filepath.Join(h.cfg.Path, name)
		fp, err = h.openNewFile(path, now)
		if err == nil {
			h.fp = fp
			h.currentName = name
			h.fileCreatedAt = now
			h.state = stateActive
			return nil
		}
	}
	return fmt.Errorf(
		"filerotate: all file creation attempts failed for %s", h.cfg.FilePrefix)
}

// openNewFile creates a new file and writes the creation timestamp stamp.
// Uses O_EXCL to ensure file creation fails if the path already exists,
// preventing silent data loss when os.Rename failed during rotation and
// the old file still occupies the target path. O_EXCL is supported on
// Linux, macOS, and Windows.
func (h *handler) openNewFile(
	path string, ts time.Time,
) (*os.File, error) {
	if testForceFailNewFileCount.Load() > 0 {
		testForceFailNewFileCount.Add(-1)
		return nil, fmt.Errorf("test: forced open failure")
	}
	fp, err := os.OpenFile(
		path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	if err := h.writeCreatedStamp(fp, ts); err != nil {
		fp.Close()
		return nil, fmt.Errorf("write created stamp: %w", err)
	}
	return fp, nil
}

// writeCreatedStamp writes the #created:<unix_ts> header line to a file.
func (h *handler) writeCreatedStamp(
	fp *os.File, ts time.Time,
) error {
	_, err := fp.WriteString(
		fmt.Sprintf("#created:%d\n", ts.Unix()))
	return err
}

// regularLogFunc writes a log entry with byte counting and rotation check.
func (h *handler) regularLogFunc(
	_ nekomimi.LogLevel, pnt func(io.StringWriter),
) {
	if h.state != stateActive || h.fp == nil {
		return
	}
	cw := &countWriter{w: h.fp, count: &h.byteCount}
	pnt(cw)
	h.itemCount++
	if h.shouldRotate() {
		h.rotate()
	}
}

// panicLogFunc writes a panic-level log synchronously and returns a
// finalizer that may trigger a panic (or noop for WrapOnly).
func (h *handler) panicLogFunc(
	pnt func(io.StringWriter), info string,
) func() {
	if h.state == stateActive && h.fp != nil {
		pnt(h.fp)
		h.fp.Sync()
	}
	if h.cfg.WrapOnly {
		return func() {}
	}
	return func() { panic(info) }
}

// fatalLogFunc writes a fatal-level log synchronously and returns a
// finalizer that may terminate the program (or noop for WrapOnly).
func (h *handler) fatalLogFunc(
	pnt func(io.StringWriter),
) func() {
	if h.state == stateActive && h.fp != nil {
		pnt(h.fp)
		h.fp.Sync()
	}
	if h.cfg.WrapOnly {
		return func() {}
	}
	return func() { exitFunc(1) }
}

// shouldRotate checks size and item count rotation thresholds.
func (h *handler) shouldRotate() bool {
	if h.cfg.MaxFileSize > 0 &&
		h.byteCount >= h.cfg.MaxFileSize*1024 {
		return true
	}
	if h.cfg.MaxFileItems > 0 &&
		h.itemCount >= h.cfg.MaxFileItems {
		return true
	}
	return false
}

// shouldRotateByTTL checks if the current file has exceeded its TTL.
func (h *handler) shouldRotateByTTL() bool {
	if h.cfg.MaxFileTTL <= 0 {
		return false
	}
	return time.Since(h.fileCreatedAt) >=
		time.Duration(h.cfg.MaxFileTTL)*time.Minute
}

// rotate performs the two-phase file rotation: archive old file, create new.
// Must be called with mu held.
func (h *handler) rotate() {
	if h.fp == nil {
		return
	}

	// Phase A: Archive old file
	h.fp.Sync()
	h.fp.Close()

	archiveName := h.archiveName()
	currentPath := filepath.Join(h.cfg.Path, h.currentName)
	archivePath := filepath.Join(h.cfg.Path, archiveName)

	if err := os.Rename(currentPath, archivePath); err == nil {
		if h.cfg.Compress {
			h.compressWG.Add(1)
			go func() {
				defer h.compressWG.Done()
				h.compressFile(archivePath)
			}()
		}
	}
	// rename failure: silently ignored, continue to Phase B

	// Phase B: Create new file
	h.fp = nil
	h.currentName = ""
	now := h.creationTime()

	// Try main name first
	path := filepath.Join(h.cfg.Path, h.cfg.FilePrefix+".log")
	fp, err := h.openNewFile(path, now)
	if err == nil {
		h.fp = fp
		h.currentName = h.cfg.FilePrefix + ".log"
		h.fileCreatedAt = now
		h.byteCount = 0
		h.itemCount = 0
		return
	}

	// Fallback names
	for n := 1; n <= maxFallbackSlots; n++ {
		name := fmt.Sprintf("%s_%d.log", h.cfg.FilePrefix, n)
		path = filepath.Join(h.cfg.Path, name)
		fp, err = h.openNewFile(path, now)
		if err == nil {
			h.fp = fp
			h.currentName = name
			h.fileCreatedAt = now
			h.byteCount = 0
			h.itemCount = 0
			return
		}
	}

	// All attempts failed
	if h.cfg.RotatePanic {
		panic("filerotate: log rotation failed for all file paths")
	}
	h.state = stateSuspended
}

// archiveName generates the archive filename from the current file's
// creation timestamp.
func (h *handler) archiveName() string {
	t := h.fileCreatedAt.UTC()
	return fmt.Sprintf("%s_%s_%d.log",
		h.cfg.FilePrefix,
		t.Format("060102"),
		h.fileCreatedAt.Unix()%86400,
	)
}

// tickerLoop runs the background ticker for flush, TTL, and audit tasks.
func (h *handler) tickerLoop(ctx context.Context) {
	// If test tick channel is set, use it instead of a real ticker
	if h.cfg.testTickCh != nil {
		for {
			select {
			case <-ctx.Done():
				h.shutdown()
				return
			case <-h.cfg.testTickCh:
				h.onTick()
			}
		}
	}

	ticker := time.NewTicker(h.tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			h.shutdown()
			return
		case <-ticker.C:
			h.onTick()
		}
	}
}

// shutdown closes resources on context cancellation.
// h.mu is released before waiting for compressWG to avoid deadlock
// with compression goroutines that need h.mu in their defer block.
func (h *handler) shutdown() {
	func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.state != stateClosed {
			if h.fp != nil {
				h.fp.Sync()
				h.fp.Close()
				h.fp = nil
			}
			h.state = stateClosed
		}
	}()
	h.compressWG.Wait()
}

// onTick handles periodic flush, TTL check, and audit tasks.
// Lock-protected work runs in an anonymous scope with defer unlock.
// cleanArchives runs outside the lock because ReadDir + sort + Remove
// are file-system I/O that should not block log writes.
func (h *handler) onTick() {
	doAudit := false
	skipKeys := make(map[string]struct{})

	func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.state == stateClosed {
			return
		}

		// flush if there is new data
		if h.fp != nil && h.state == stateActive &&
			h.byteCount != h.lastFlushCount {
			h.fp.Sync()
			h.lastFlushCount = h.byteCount
		}

		// TTL rotation check
		if h.state == stateActive && h.fp != nil &&
			h.shouldRotateByTTL() {
			h.rotate()
		}

		h.tickCount++
		doAudit = h.tickCount%h.auditInterval == 0

		// audit crash recovery (modifies handler state → under lock)
		if doAudit {
			h.auditRecoveryLocked()
		}

		// snapshot keys currently being compressed for cleanArchives
		for k := range h.compressing {
			skipKeys[k] = struct{}{}
		}
	}()

	// archive cleanup (I/O heavy → outside lock)
	if doAudit {
		h.cleanArchives(skipKeys)
	}
}


// auditRecoveryLocked attempts crash recovery when the handler is in
// suspended state.  Must be called with h.mu held — it modifies handler
// state, fp, and may archive residual files.
func (h *handler) auditRecoveryLocked() {
	if h.state == stateSuspended {
		h.archiveAllResidual() // free blocked paths if any
		if err := h.tryRecover(); err == nil {
			h.state = stateActive
		}
	}
}

// archiveAllResidual archives all residual log files in the directory.
// Returns true if at least one file was successfully archived.
func (h *handler) archiveAllResidual() bool {
	entries, err := os.ReadDir(h.cfg.Path)
	if err != nil {
		return false
	}

	hasArchived := false
	for _, entry := range entries {
		name := entry.Name()
		if !h.matchLogFile(name) {
			continue
		}
		srcPath := filepath.Join(h.cfg.Path, name)
		ts := h.extractTimestamp(srcPath)
		t := time.Unix(ts, 0).UTC()
		archiveName := fmt.Sprintf("%s_%s_%d.log",
			h.cfg.FilePrefix,
			t.Format("060102"),
			ts%86400,
		)
		dstPath := filepath.Join(h.cfg.Path, archiveName)

		if err := os.Rename(srcPath, dstPath); err != nil {
			continue
		}
		hasArchived = true

		if h.cfg.Compress {
			h.compressWG.Add(1)
			go func(p string) {
				defer h.compressWG.Done()
				h.compressFile(p)
			}(dstPath)
		}
	}
	return hasArchived
}

// tryRecover attempts to create a new log file for recovery. Tries main
// name first, then fallback names.
func (h *handler) tryRecover() error {
	now := h.creationTime()
	// Try main name
	path := filepath.Join(h.cfg.Path, h.cfg.FilePrefix+".log")
	fp, err := h.openNewFile(path, now)
	if err == nil {
		h.fp = fp
		h.currentName = h.cfg.FilePrefix + ".log"
		h.fileCreatedAt = now
		h.byteCount = 0
		h.itemCount = 0
		return nil
	}

	// Fallback names
	for n := 1; n <= maxFallbackSlots; n++ {
		name := fmt.Sprintf("%s_%d.log", h.cfg.FilePrefix, n)
		path = filepath.Join(h.cfg.Path, name)
		fp, err = h.openNewFile(path, now)
		if err == nil {
			h.fp = fp
			h.currentName = name
			h.fileCreatedAt = now
			h.byteCount = 0
			h.itemCount = 0
			return nil
		}
	}
	return fmt.Errorf("filerotate: all recovery attempts failed")
}

// cleanArchives removes the oldest archived log files when the number of
// archives exceeds MaxArchives.  skipKeys contains timestamp keys of
// archives currently being compressed; those entries are excluded from
// counting and deletion.  This function performs file-system I/O and
// must NOT be called with h.mu held.
func (h *handler) cleanArchives(skipKeys map[string]struct{}) {
	if h.cfg.MaxArchives <= 0 {
		return
	}

	entries, err := os.ReadDir(h.cfg.Path)
	if err != nil {
		return
	}

	// collect archive files, skipping those being compressed
	var archives []os.DirEntry
	for _, entry := range entries {
		name := entry.Name()
		if !isArchiveFile(name, h.cfg.FilePrefix) {
			continue
		}
		if _, skip := skipKeys[extractTimestampKey(name)]; skip {
			continue // being compressed, skip entirely
		}
		archives = append(archives, entry)
	}

	if len(archives) <= h.cfg.MaxArchives {
		return
	}

	// sort by name (timestamp order)
	sort.Slice(archives, func(i, j int) bool {
		return archives[i].Name() < archives[j].Name()
	})

	// delete oldest
	toDelete := len(archives) - h.cfg.MaxArchives
	for i := 0; i < toDelete; i++ {
		p := filepath.Join(h.cfg.Path, archives[i].Name())
		os.Remove(p) // silent ignore on error
	}
}

// isArchiveFile reports whether name matches the archive file pattern:
// <prefix>_<yymmdd>_<seconds>.log[.gz]
func isArchiveFile(name, prefix string) bool {
	base := name
	base = strings.TrimSuffix(base, ".gz")
	base = strings.TrimSuffix(base, ".log")

	p := prefix + "_"
	if !strings.HasPrefix(base, p) {
		return false
	}
	rest := base[len(p):]
	parts := strings.SplitN(rest, "_", 2)
	if len(parts) != 2 {
		return false
	}
	_, err1 := strconv.Atoi(parts[0])
	_, err2 := strconv.Atoi(parts[1])
	return err1 == nil && err2 == nil && len(parts[0]) == 6
}

// matchLogFile reports whether a file is a residual log file that should
// be archived. Matches <prefix>.log and <prefix>_<n>.log (fallback files),
// but not already-archived files.
func (h *handler) matchLogFile(name string) bool {
	pfx := h.cfg.FilePrefix
	// Main log file
	if name == pfx+".log" {
		return true
	}
	// Already archived: <prefix>_<yymmdd>_<seconds>.log[.gz]
	if isArchiveFile(name, pfx) {
		return false
	}
	// Fallback file: <prefix>_<n>.log
	if !strings.HasPrefix(name, pfx+"_") {
		return false
	}
	rest := strings.TrimSuffix(name, ".log")
	rest = rest[len(pfx)+1:]
	if _, err := strconv.Atoi(rest); err == nil {
		return true
	}
	return false
}

// extractTimestampKey returns the timestamp portion of an archive filename
// as a grouping key.  For "prefix_260627_45045.log" or ".log.gz" it
// returns "260627_45045".  Returns "" for non-archive filenames.
func extractTimestampKey(name string) string {
	name = strings.TrimSuffix(name, ".gz")
	name = strings.TrimSuffix(name, ".log")
	idx := strings.Index(name, "_")
	if idx < 0 {
		return ""
	}
	return name[idx+1:]
}

// compressFile gzip-compresses an archive file.  It writes to a temp
// file first (.log.gz.tmp) then atomically renames to .log.gz to avoid
// leaving a partial .gz that audit could confuse as complete.  While
// compression is in progress the file's timestamp key is registered in
// h.compressing so that cleanArchives skips it.
func (h *handler) compressFile(path string) {
	key := extractTimestampKey(filepath.Base(path))

	h.mu.Lock()
	h.compressing[key] = struct{}{}
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.compressing, key)
		h.mu.Unlock()
	}()

	in, err := os.Open(path)
	if err != nil {
		return
	}
	defer in.Close()

	outPath := path + ".gz"
	tmpPath := outPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return
	}

	gw := gzip.NewWriter(out)
	_, copyErr := io.Copy(gw, in)
	gwCloseErr := gw.Close()
	out.Close()

	if copyErr != nil || gwCloseErr != nil {
		os.Remove(tmpPath) // clean up partial temp
		return
	}

	// atomically promote temp → real
	if err := os.Rename(tmpPath, outPath); err != nil {
		os.Remove(tmpPath)
		return
	}

	// safely remove the uncompressed original
	os.Remove(path)
}
