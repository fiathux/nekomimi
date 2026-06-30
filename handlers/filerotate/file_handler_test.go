package filerotate

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fiathux/nekomimi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tempDir creates a temporary directory for testing.
func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "filerotate_test")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// readFileContent reads the entire content of a file.
func readFileContent(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

// listFiles returns file names in a directory.
func listFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// createOldLog creates a residual log file for archive testing.
func createOldLog(t *testing.T, dir, name, firstLine string) {
	t.Helper()
	path := filepath.Join(dir, name)
	err := os.WriteFile(path, []byte(firstLine+"\nline2\n"), 0o644)
	require.NoError(t, err)
}

// ============================================================
// TestNew_ValidConfig
// ============================================================
func TestNew_ValidConfig(t *testing.T) {
	dir := tempDir(t)
	ctx := context.Background()

	h, err := New(ctx, Config{
		Path:       dir,
		FilePrefix: "test",
	})
	require.NoError(t, err)
	require.NotNil(t, h)

	logPath := filepath.Join(dir, "test.log")
	require.FileExists(t, logPath)

	content := readFileContent(t, logPath)
	assert.True(t, strings.HasPrefix(content, "#created:"),
		"first line should be #created:<ts>")
}

// ============================================================
// TestNew_DirectoryNotExist
// ============================================================
func TestNew_DirectoryNotExist(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "nekomimi_filerotate_nonexist",
		"subdir_"+strconv.FormatInt(time.Now().UnixNano(), 10))
	os.RemoveAll(filepath.Dir(dir))

	ctx := context.Background()
	h, err := New(ctx, Config{
		Path:       dir,
		FilePrefix: "test",
	})
	require.NoError(t, err)
	require.NotNil(t, h)
	require.DirExists(t, dir)
	os.RemoveAll(filepath.Dir(dir))
}

// ============================================================
// TestNew_FileOpenFailed
// ============================================================
func TestNew_FileOpenFailed(t *testing.T) {
	dir := tempDir(t)
	roSub := filepath.Join(dir, "readonly")
	require.NoError(t, os.Mkdir(roSub, 0o444))
	t.Cleanup(func() { os.Chmod(roSub, 0o755) })

	ctx := context.Background()
	_, err := New(ctx, Config{
		Path:       roSub,
		FilePrefix: "test",
	})
	require.Error(t, err)
}

// ============================================================
// TestNew_EmptyFilePrefix
// ============================================================
func TestNew_EmptyFilePrefix(t *testing.T) {
	dir := tempDir(t)
	ctx := context.Background()
	_, err := New(ctx, Config{
		Path:       dir,
		FilePrefix: "",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "FilePrefix")
}

// ============================================================
// TestNew_ArchivesOldFiles
// ============================================================
func TestNew_ArchivesOldFiles(t *testing.T) {
	dir := tempDir(t)
	createOldLog(t, dir, "app.log", "#created:100")

	ctx := context.Background()
	h, err := New(ctx, Config{
		Path:       dir,
		FilePrefix: "app",
	})
	require.NoError(t, err)
	require.NotNil(t, h)

	files := listFiles(t, dir)
	assert.Contains(t, files, "app_700101_00100_0000.log")
	assert.Contains(t, files, "app.log")
}

// ============================================================
// TestNew_ArchivesOldFallbackFiles
// ============================================================
func TestNew_ArchivesOldFallbackFiles(t *testing.T) {
	dir := tempDir(t)
	createOldLog(t, dir, "app.log", "#created:100")
	createOldLog(t, dir, "app_1.log", "#created:200")

	ctx := context.Background()
	h, err := New(ctx, Config{
		Path:       dir,
		FilePrefix: "app",
	})
	require.NoError(t, err)
	require.NotNil(t, h)

	files := listFiles(t, dir)
	assert.Contains(t, files, "app_700101_00100_0000.log")
	assert.Contains(t, files, "app_700101_00200_0000.log")
	assert.Contains(t, files, "app.log")
}

// ============================================================
// TestNew_TimestampFallbackToMtime
// ============================================================
func TestNew_TimestampFallbackToMtime(t *testing.T) {
	dir := tempDir(t)
	oldPath := filepath.Join(dir, "app.log")
	require.NoError(t, os.WriteFile(oldPath,
		[]byte("not a timestamp\nsome log\n"), 0o644))

	ctx := context.Background()
	h, err := New(ctx, Config{
		Path:       dir,
		FilePrefix: "app",
	})
	require.NoError(t, err)
	require.NotNil(t, h)

	files := listFiles(t, dir)
	hasArchive := false
	for _, f := range files {
		if strings.HasPrefix(f, "app_") && strings.HasSuffix(f, ".log") &&
			f != "app.log" {
			hasArchive = true
			break
		}
	}
	assert.True(t, hasArchive, "old file should be archived using mtime")
}

// ============================================================
// TestNew_MaxFileItemsClamping
// ============================================================
func TestNew_MaxFileItemsClamping(t *testing.T) {
	dir := tempDir(t)
	ctx := context.Background()

	h, err := New(ctx, Config{
		Path:         dir,
		FilePrefix:   "test",
		MaxFileItems: 50, // clamped to 100
	})
	require.NoError(t, err)
	require.NotNil(t, h)

	// Write 99 items — should not rotate (clamped to 100)
	for i := 0; i < 99; i++ {
		h.RegularLog(nekomimi.INFO, "h ", "msg")
	}
	files := listFiles(t, dir)
	assert.Len(t, files, 1,
		"99 items should not trigger rotation when clamped to 100")
}

// ============================================================
// TestRegularLog_WritesToFile
// ============================================================
func TestRegularLog_WritesToFile(t *testing.T) {
	dir := tempDir(t)
	ctx := context.Background()

	h, err := New(ctx, Config{
		Path:       dir,
		FilePrefix: "test",
	})
	require.NoError(t, err)

	h.RegularLog(nekomimi.INFO, "header - ", "test message")

	content := readFileContent(t, filepath.Join(dir, "test.log"))
	assert.Contains(t, content, "header - test message")
}

// ============================================================
// TestRegularWriter
// ============================================================
func TestRegularWriter(t *testing.T) {
	dir := tempDir(t)
	ctx := context.Background()

	h, err := New(ctx, Config{
		Path:       dir,
		FilePrefix: "test",
	})
	require.NoError(t, err)

	h.RegularWriter(nekomimi.DEBUG, func(w io.StringWriter) {
		w.WriteString("raw data\n")
	})

	content := readFileContent(t, filepath.Join(dir, "test.log"))
	assert.Contains(t, content, "raw data")
}

// ============================================================
// TestLogRotation_BySize
// ============================================================
func TestLogRotation_BySize(t *testing.T) {
	dir := tempDir(t)
	ctx := context.Background()

	h, err := New(ctx, Config{
		Path:        dir,
		FilePrefix:  "test",
		MaxFileSize: 1, // 1 KB
	})
	require.NoError(t, err)

	bigMsg := strings.Repeat("x", 2048)
	h.RegularLog(nekomimi.INFO, "h ", bigMsg)

	files := listFiles(t, dir)
	assert.GreaterOrEqual(t, len(files), 2,
		"should have archive file and current file")
	hasArchive := false
	for _, f := range files {
		if strings.HasPrefix(f, "test_") && strings.HasSuffix(f, ".log") &&
			f != "test.log" {
			hasArchive = true
			break
		}
	}
	assert.True(t, hasArchive, "archive file should exist after size rotation")
}

// ============================================================
// TestLogRotation_ByItems
// ============================================================
func TestLogRotation_ByItems(t *testing.T) {
	dir := tempDir(t)
	ctx := context.Background()

	h, err := New(ctx, Config{
		Path:         dir,
		FilePrefix:   "test",
		MaxFileItems: 100, // min effective value
	})
	require.NoError(t, err)

	for i := 0; i < 100; i++ {
		h.RegularLog(nekomimi.INFO, "h ", "msg")
	}

	files := listFiles(t, dir)
	hasArchive := false
	for _, f := range files {
		if strings.HasPrefix(f, "test_") && strings.HasSuffix(f, ".log") &&
			f != "test.log" {
			hasArchive = true
			break
		}
	}
	assert.True(t, hasArchive, "archive should exist after 100 items")

	// Write 5 more — should go to new file
	for i := 0; i < 5; i++ {
		h.RegularLog(nekomimi.INFO, "h ", "msg2")
	}

	content := readFileContent(t, filepath.Join(dir, "test.log"))
	lines := strings.Split(strings.TrimSpace(content), "\n")
	assert.Len(t, lines, 6, "new file should have 1 stamp + 5 log lines")
}

// ============================================================
// TestLogRotation_ArchiveNaming
// ============================================================
func TestLogRotation_ArchiveNaming(t *testing.T) {
	dir := tempDir(t)
	ctx := context.Background()

	h, err := New(ctx, Config{
		Path:        dir,
		FilePrefix:  "app",
		MaxFileSize: 1,
	})
	require.NoError(t, err)

	bigMsg := strings.Repeat("x", 2048)
	h.RegularLog(nekomimi.INFO, "h ", bigMsg)

	files := listFiles(t, dir)
	hasArchive := false
	for _, f := range files {
		if strings.HasPrefix(f, "app_") && strings.HasSuffix(f, ".log") &&
			f != "app.log" && !strings.HasSuffix(f, ".gz") {
			assert.Regexp(t, `^app_\d{6}_\d{5}_\d{4}\.log$`, f,
				"archive should match prefix_yymmdd_seconds_counter.log")
			hasArchive = true
		}
	}
	assert.True(t, hasArchive, "archive file should exist")
}

// ============================================================
// TestLogRotation_SameSecond
// ============================================================
func TestLogRotation_SameSecond(t *testing.T) {
	dir := tempDir(t)
	ctx := context.Background()

	// Force all rotations to use the same timestamp so they collide
	// within one second.  This verifies the counter suffix avoids
	// filename clashes.
	fixedTime := time.Now()
	h, err := New(ctx, Config{
		Path:        dir,
		FilePrefix:  "app",
		MaxFileSize: 1,
		testForceCreationTime: fixedTime,
	})
	require.NoError(t, err)

	bigMsg := strings.Repeat("x", 2048)
	// trigger 3 rotations within the same logical second
	for i := 0; i < 3; i++ {
		h.RegularLog(nekomimi.INFO, "h ", bigMsg)
	}

	files := listFiles(t, dir)
	archiveNames := make(map[string]bool)
	for _, f := range files {
		if isArchiveFile(f, "app") {
			archiveNames[f] = true
		}
	}
	assert.Len(t, archiveNames, 3,
		"same-second rotations must produce distinct archive names")
	// each should have a unique counter suffix
	assert.True(t, archiveNames["app_"+fixedTime.UTC().Format("060102")+"_"+
		fmt.Sprintf("%05d", fixedTime.Unix()%86400)+"_0000.log"])
	assert.True(t, archiveNames["app_"+fixedTime.UTC().Format("060102")+"_"+
		fmt.Sprintf("%05d", fixedTime.Unix()%86400)+"_0001.log"])
	assert.True(t, archiveNames["app_"+fixedTime.UTC().Format("060102")+"_"+
		fmt.Sprintf("%05d", fixedTime.Unix()%86400)+"_0002.log"])
}

// ============================================================
// TestLogRotation_Compress
// ============================================================
func TestLogRotation_Compress(t *testing.T) {
	dir := tempDir(t)
	ctx := context.Background()

	h, err := New(ctx, Config{
		Path:        dir,
		FilePrefix:  "app",
		MaxFileSize: 1,
		Compress:    true,
	})
	require.NoError(t, err)

	bigMsg := strings.Repeat("x", 2048)
	h.RegularLog(nekomimi.INFO, "h ", bigMsg)

	// Compression is async
	time.Sleep(500 * time.Millisecond)

	files := listFiles(t, dir)
	hasGz := false
	for _, f := range files {
		if strings.HasSuffix(f, ".log.gz") {
			hasGz = true
			gzPath := filepath.Join(dir, f)
			gf, err := os.Open(gzPath)
			require.NoError(t, err)
			defer gf.Close()

			gr, err := gzip.NewReader(gf)
			require.NoError(t, err)
			defer gr.Close()

			content, err := io.ReadAll(gr)
			require.NoError(t, err)
			assert.Contains(t, string(content), "#created:")
			break
		}
	}
	assert.True(t, hasGz, "compressed .log.gz should exist")
}

// ============================================================
// TestLogRotation_FallbackNaming
// ============================================================
func TestLogRotation_FallbackNaming(t *testing.T) {
	dir := tempDir(t)
	ctx := context.Background()

	// Force app.log creation to fail once, so New() falls back to app_1.log
	testForceFailNewFileCount.Store(1) // fail B1 (app.log), B2 should succeed
	defer testForceFailNewFileCount.Store(0)

	h, err := New(ctx, Config{
		Path:       dir,
		FilePrefix: "app",
	})
	require.NoError(t, err)
	require.NotNil(t, h)

	files := listFiles(t, dir)
	assert.Contains(t, files, "app_1.log",
		"should create fallback file when main name is blocked")

	h.RegularLog(nekomimi.INFO, "h ", "fallback write")
	content := readFileContent(t, filepath.Join(dir, "app_1.log"))
	assert.Contains(t, content, "fallback write")
}

// ============================================================
// TestLogRotation_AllFallbackFail
// ============================================================
func TestLogRotation_AllFallbackFail(t *testing.T) {
	dir := tempDir(t)
	tickCh := make(chan time.Time, 10)
	ctx := context.Background()

	h, err := New(ctx, Config{
		Path:        dir,
		FilePrefix:  "app",
		MaxFileSize: 1,
		testTickCh:  tickCh,
	})
	require.NoError(t, err)

	// Force all new file creations to fail
	testForceFailNewFileCount.Store(100)
	defer testForceFailNewFileCount.Store(0)

	// Trigger rotation — all Phase B attempts fail → suspended
	bigMsg := strings.Repeat("x", 2048)
	h.RegularLog(nekomimi.INFO, "h ", bigMsg)

	// Writes should be dropped silently
	h.RegularLog(nekomimi.INFO, "h ", "this should be dropped")
	h.RegularWriter(nekomimi.DEBUG, func(w io.StringWriter) {
		w.WriteString("dropped too\n")
	})
}

// ============================================================
// TestLogRotation_RotatePanicTrue
// ============================================================
func TestLogRotation_RotatePanicTrue(t *testing.T) {
	dir := tempDir(t)
	ctx := context.Background()

	h, err := New(ctx, Config{
		Path:        dir,
		FilePrefix:  "app",
		MaxFileSize: 1,
		RotatePanic: true,
	})
	require.NoError(t, err)

	// Force all new file creations to fail
	testForceFailNewFileCount.Store(100)
	defer testForceFailNewFileCount.Store(0)

	// Trigger rotation — all Phase B fail → panic
	bigMsg := strings.Repeat("x", 2048)
	assert.Panics(t, func() {
		h.RegularLog(nekomimi.INFO, "h ", bigMsg)
	}, "rotate with RotatePanic=true should panic when all paths fail")
}

// ============================================================
// TestLogRotation_MultipleConsecutive
// ============================================================
func TestLogRotation_MultipleConsecutive(t *testing.T) {
	dir := tempDir(t)
	ctx := context.Background()

	h, err := New(ctx, Config{
		Path:        dir,
		FilePrefix:  "app",
		MaxFileSize: 1,
	})
	require.NoError(t, err)

	bigMsg := strings.Repeat("x", 2048)

	// Sleep before first write to ensure distinct archive timestamps
	time.Sleep(1100 * time.Millisecond)

	// Trigger 3 rotations with sleeps to ensure unique archive names
	for i := 0; i < 3; i++ {
		h.RegularLog(nekomimi.INFO, "h ", bigMsg)
		time.Sleep(1100 * time.Millisecond)
	}

	files := listFiles(t, dir)
	archiveCount := 0
	for _, f := range files {
		if isArchiveFile(f, "app") {
			archiveCount++
		}
	}
	assert.Equal(t, 3, archiveCount, "should have 3 archive files")
}

// ============================================================
// TestLogRotation_ByTTL
// ============================================================
func TestLogRotation_ByTTL(t *testing.T) {
	dir := tempDir(t)
	tickCh := make(chan time.Time, 10)
	ctx := context.Background()

	h, err := New(ctx, Config{
		Path:              dir,
		FilePrefix:        "app",
		MaxFileTTL:        1, // 1 minute TTL
		testTickCh:        tickCh,
		testAuditInterval: 1,
		testForceCreationTime: time.Now().Add(-2 * time.Minute),
	})
	require.NoError(t, err)

	// Write one entry
	h.RegularLog(nekomimi.INFO, "h - ", "ttl-test")

	// Trigger a tick to flush and check TTL
	tickCh <- time.Now()
	time.Sleep(100 * time.Millisecond)

	// Rotation should have been triggered by TTL check on tick
	files := listFiles(t, dir)
	archiveCount := 0
	for _, f := range files {
		if isArchiveFile(f, "app") {
			archiveCount++
		}
	}
	assert.Equal(t, 1, archiveCount,
		"TTL rotation should produce 1 archive file")
}

// ============================================================
// TestSuspended_WritesDropped
// ============================================================
func TestSuspended_WritesDropped(t *testing.T) {
	dir := tempDir(t)
	tickCh := make(chan time.Time, 10)
	ctx := context.Background()

	h, err := New(ctx, Config{
		Path:        dir,
		FilePrefix:  "app",
		MaxFileSize: 1,
		testTickCh:  tickCh,
	})
	require.NoError(t, err)

	// Force all openNewFile to fail
	testForceFailNewFileCount.Store(100)
	defer testForceFailNewFileCount.Store(0)

	// Trigger rotation → suspended
	bigMsg := strings.Repeat("x", 2048)
	h.RegularLog(nekomimi.INFO, "h ", bigMsg)

	// Writes should be dropped silently
	h.RegularLog(nekomimi.INFO, "h ", "dropped message")
	h.RegularWriter(nekomimi.DEBUG, func(w io.StringWriter) {
		w.WriteString("dropped\n")
	})
}

// ============================================================
// TestAudit_RecoversFromSuspended
// ============================================================
func TestAudit_RecoversFromSuspended(t *testing.T) {
	dir := tempDir(t)
	tickCh := make(chan time.Time, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h, err := New(ctx, Config{
		Path:              dir,
		FilePrefix:        "app",
		MaxFileSize:       1,
		testTickCh:        tickCh,
		testAuditInterval: 1, // audit on every tick
	})
	require.NoError(t, err)

	// Force all openNewFile to fail → suspended
	testForceFailNewFileCount.Store(100)
	bigMsg := strings.Repeat("x", 2048)
	h.RegularLog(nekomimi.INFO, "h ", bigMsg)

	// Now allow file creation again
	testForceFailNewFileCount.Store(0)

	// Trigger audit tick → recovery
	tickCh <- time.Now()
	time.Sleep(100 * time.Millisecond)

	// After recovery, writes should work
	h.RegularLog(nekomimi.INFO, "h ", "recovered message")
	content := readFileContent(t, filepath.Join(dir, "app.log"))
	assert.Contains(t, content, "recovered message")
}

// ============================================================
// TestAudit_RecoversWithFallbackName
// ============================================================
func TestAudit_RecoversWithFallbackName(t *testing.T) {
	dir := tempDir(t)
	tickCh := make(chan time.Time, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h, err := New(ctx, Config{
		Path:              dir,
		FilePrefix:        "app",
		MaxFileSize:       1,
		testTickCh:        tickCh,
		testAuditInterval: 1,
	})
	require.NoError(t, err)

	// Force all openNewFile to fail → suspended
	testForceFailNewFileCount.Store(100)
	bigMsg := strings.Repeat("x", 2048)
	h.RegularLog(nekomimi.INFO, "h ", bigMsg)
	testForceFailNewFileCount.Store(0)

	// Block app.log so recovery falls back to app_1.log
	require.NoError(t, os.Mkdir(filepath.Join(dir, "app.log"), 0o755))

	// Trigger audit tick → recovery with fallback
	tickCh <- time.Now()
	time.Sleep(100 * time.Millisecond)

	assert.FileExists(t, filepath.Join(dir, "app_1.log"),
		"audit should recover with fallback name")

	h.RegularLog(nekomimi.INFO, "h ", "fallback recovered")
	content := readFileContent(t, filepath.Join(dir, "app_1.log"))
	assert.Contains(t, content, "fallback recovered")
}

// ============================================================
// TestAudit_CleansArchives
// ============================================================
func TestAudit_CleansArchives(t *testing.T) {
	dir := tempDir(t)
	tickCh := make(chan time.Time, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create 5 pre-existing archive files
	for i := 0; i < 5; i++ {
		ts := int64(100 + i)
		tm := time.Unix(ts, 0).UTC()
		name := "app_" + tm.Format("060102") + "_" +
			strconv.FormatInt(ts%86400, 10) + ".log"
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, name),
			[]byte("archive\n"), 0o644))
	}

	_, err := New(ctx, Config{
		Path:              dir,
		FilePrefix:        "app",
		MaxArchives:       2,
		testTickCh:        tickCh,
		testAuditInterval: 1,
	})
	require.NoError(t, err)

	// Let goroutine start
	time.Sleep(50 * time.Millisecond)

	tickCh <- time.Now()
	time.Sleep(100 * time.Millisecond)

	files := listFiles(t, dir)
	archiveCount := 0
	for _, f := range files {
		if isArchiveFile(f, "app") {
			archiveCount++
		}
	}
	assert.LessOrEqual(t, archiveCount, 2,
		"should have at most MaxArchives archive files remaining")
}

// ============================================================
// TestPanicLog_WritesAndPanics
// ============================================================
func TestPanicLog_WritesAndPanics(t *testing.T) {
	dir := tempDir(t)
	ctx := context.Background()

	h, err := New(ctx, Config{
		Path:       dir,
		FilePrefix: "test",
	})
	require.NoError(t, err)

	var panicCaught bool
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicCaught = true
			}
		}()
		h.PanicLog("header - ", "panic msg")
	}()
	assert.True(t, panicCaught, "PanicLog should panic")

	content := readFileContent(t, filepath.Join(dir, "test.log"))
	assert.Contains(t, content, "header - panic msg")
}

// ============================================================
// TestPanicLog_WrapOnlyNoPanic
// ============================================================
func TestPanicLog_WrapOnlyNoPanic(t *testing.T) {
	dir := tempDir(t)
	ctx := context.Background()

	h, err := New(ctx, Config{
		Path:       dir,
		FilePrefix: "test",
		WrapOnly:   true,
	})
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		h.PanicLog("header - ", "panic msg wrap")
	})

	content := readFileContent(t, filepath.Join(dir, "test.log"))
	assert.Contains(t, content, "header - panic msg wrap")
}

// ============================================================
// TestFatalLog_WritesAndTerminates
// ============================================================
func TestFatalLog_WritesAndTerminates(t *testing.T) {
	dir := tempDir(t)
	ctx := context.Background()

	h, err := New(ctx, Config{
		Path:       dir,
		FilePrefix: "test",
	})
	require.NoError(t, err)

	origExit := exitFunc
	exitCalled := false
	exitCode := 0
	exitFunc = func(code int) {
		exitCalled = true
		exitCode = code
	}
	defer func() { exitFunc = origExit }()

	h.FatalLog("header - ", "fatal msg")
	assert.True(t, exitCalled, "FatalLog should call exit")
	assert.Equal(t, 1, exitCode)

	content := readFileContent(t, filepath.Join(dir, "test.log"))
	assert.Contains(t, content, "header - fatal msg")
}

// ============================================================
// TestFatalLog_WrapOnlyNoExit
// ============================================================
func TestFatalLog_WrapOnlyNoExit(t *testing.T) {
	dir := tempDir(t)
	ctx := context.Background()

	h, err := New(ctx, Config{
		Path:       dir,
		FilePrefix: "test",
		WrapOnly:   true,
	})
	require.NoError(t, err)

	origExit := exitFunc
	exitCalled := false
	exitFunc = func(code int) { exitCalled = true }
	defer func() { exitFunc = origExit }()

	h.FatalLog("header - ", "fatal msg noexit")
	assert.False(t, exitCalled, "WrapOnly should prevent exit")

	content := readFileContent(t, filepath.Join(dir, "test.log"))
	assert.Contains(t, content, "header - fatal msg noexit")
}

// ============================================================
// TestContextCancel_ReleasesResources
// ============================================================
func TestContextCancel_ReleasesResources(t *testing.T) {
	dir := tempDir(t)
	tickCh := make(chan time.Time, 10)
	ctx, cancel := context.WithCancel(context.Background())

	h, err := New(ctx, Config{
		Path:       dir,
		FilePrefix: "test",
		testTickCh: tickCh,
	})
	require.NoError(t, err)

	h.RegularLog(nekomimi.INFO, "h ", "before cancel")

	cancel()
	time.Sleep(200 * time.Millisecond)

	// After cancel, writes should be silently dropped
	h.RegularLog(nekomimi.INFO, "h ", "after cancel")
}

// ============================================================
// TestWrapper_ForwardsRegularLog
// ============================================================
func TestWrapper_ForwardsRegularLog(t *testing.T) {
	dir := tempDir(t)
	ctx := context.Background()

	var wrapperCalled bool
	wrapper := nekomimi.TinyLogHandlerFunc(
		func(level nekomimi.LogLevel, pnt func(io.StringWriter)) {
			wrapperCalled = true
		},
	)

	h, err := New(ctx, Config{
		Path:       dir,
		FilePrefix: "test",
		Wrapper:    wrapper,
	})
	require.NoError(t, err)

	h.RegularLog(nekomimi.INFO, "h - ", "msg")
	assert.True(t, wrapperCalled, "wrapper should receive regular log")

	content := readFileContent(t, filepath.Join(dir, "test.log"))
	assert.Contains(t, content, "h - msg")
}

// ============================================================
// TestWrapper_PanicForwardsAsRegular
// ============================================================
func TestWrapper_PanicForwardsAsRegular(t *testing.T) {
	dir := tempDir(t)
	ctx := context.Background()

	var wrapperCalled bool
	var wrapperLevel nekomimi.LogLevel
	wrapper := nekomimi.TinyLogHandlerFunc(
		func(level nekomimi.LogLevel, pnt func(io.StringWriter)) {
			wrapperCalled = true
			wrapperLevel = level
		},
	)

	h, err := New(ctx, Config{
		Path:       dir,
		FilePrefix: "test",
		Wrapper:    wrapper,
		WrapOnly:   true,
	})
	require.NoError(t, err)

	h.PanicLog("h - ", "p")

	assert.True(t, wrapperCalled, "wrapper should receive panic as regular")
	assert.Equal(t, nekomimi.PANIC, wrapperLevel)

	content := readFileContent(t, filepath.Join(dir, "test.log"))
	assert.Contains(t, content, "h - p")
}

// ============================================================
// TestWrapper_FatalForwardsAsRegular
// ============================================================
func TestWrapper_FatalForwardsAsRegular(t *testing.T) {
	dir := tempDir(t)
	ctx := context.Background()

	var wrapperCalled bool
	var wrapperLevel nekomimi.LogLevel
	wrapper := nekomimi.TinyLogHandlerFunc(
		func(level nekomimi.LogLevel, pnt func(io.StringWriter)) {
			wrapperCalled = true
			wrapperLevel = level
		},
	)

	h, err := New(ctx, Config{
		Path:       dir,
		FilePrefix: "test",
		Wrapper:    wrapper,
		WrapOnly:   true,
	})
	require.NoError(t, err)

	h.FatalLog("h - ", "f")

	assert.True(t, wrapperCalled, "wrapper should receive fatal as regular")
	assert.Equal(t, nekomimi.FATAL, wrapperLevel)

	content := readFileContent(t, filepath.Join(dir, "test.log"))
	assert.Contains(t, content, "h - f")
}

// ============================================================
// TestTicker_FlushesOnWrite
// ============================================================
func TestTicker_FlushesOnWrite(t *testing.T) {
	dir := tempDir(t)
	ctx := context.Background()

	h, err := New(ctx, Config{
		Path:       dir,
		FilePrefix: "test",
	})
	require.NoError(t, err)

	h.RegularLog(nekomimi.INFO, "h ", "flush test")

	logPath := filepath.Join(dir, "test.log")
	content := readFileContent(t, logPath)
	assert.Contains(t, content, "flush test")
}

// ============================================================
// TestConcurrent_MultipleGoroutines
// ============================================================
func TestConcurrent_MultipleGoroutines(t *testing.T) {
	dir := tempDir(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h, err := New(ctx, Config{
		Path:       dir,
		FilePrefix: "test",
	})
	require.NoError(t, err)

	var wg sync.WaitGroup
	numGoroutines := 10
	numLogsPerGoroutine := 100

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < numLogsPerGoroutine; i++ {
				h.RegularLog(nekomimi.INFO, "h ", "concurrent msg")
			}
		}(g)
	}
	wg.Wait()

	logPath := filepath.Join(dir, "test.log")
	content := readFileContent(t, logPath)
	lines := strings.Split(strings.TrimSpace(content), "\n")
	expectedLines := 1 + numGoroutines*numLogsPerGoroutine
	assert.Len(t, lines, expectedLines,
		"all concurrent writes should be present")
}

// ============================================================
// TestArchiveFileNaming
// ============================================================
func TestArchiveFileNaming(t *testing.T) {
	assert.True(t, isArchiveFile("app_260627_45045_0000.log", "app"))
	assert.True(t, isArchiveFile("app_260627_45045_0001.log.gz", "app"))
	assert.True(t, isArchiveFile("app_700101_00100_0000.log", "app"))
	assert.False(t, isArchiveFile("app.log", "app"))
	assert.False(t, isArchiveFile("app_1.log", "app"))
	assert.False(t, isArchiveFile("other_260627_45045_0000.log", "app"))
}

// ============================================================
// TestPanicLog_SuspendedDrops
// ============================================================
func TestPanicLog_SuspendedDrops(t *testing.T) {
	dir := tempDir(t)
	tickCh := make(chan time.Time, 10)
	ctx := context.Background()

	h, err := New(ctx, Config{
		Path:        dir,
		FilePrefix:  "app",
		MaxFileSize: 1,
		WrapOnly:    true,
		testTickCh:  tickCh,
	})
	require.NoError(t, err)

	// Force suspended via test failure injection
	testForceFailNewFileCount.Store(100)
	defer testForceFailNewFileCount.Store(0)

	bigMsg := strings.Repeat("x", 2048)
	h.RegularLog(nekomimi.INFO, "h ", bigMsg) // triggers rotation → suspended

	// PanicLog in suspended: should not panic and not write
	assert.NotPanics(t, func() {
		h.PanicLog("h ", "panic in suspend")
	})
}

// ============================================================
// TestFatalLog_SuspendedExits
// ============================================================
func TestFatalLog_SuspendedExits(t *testing.T) {
	dir := tempDir(t)
	tickCh := make(chan time.Time, 10)
	ctx := context.Background()

	origExit := exitFunc
	exitCalled := false
	exitFunc = func(code int) { exitCalled = true }
	defer func() { exitFunc = origExit }()

	h, err := New(ctx, Config{
		Path:        dir,
		FilePrefix:  "app",
		MaxFileSize: 1,
		testTickCh:  tickCh,
	})
	require.NoError(t, err)

	// Force suspended
	testForceFailNewFileCount.Store(100)
	bigMsg := strings.Repeat("x", 2048)
	h.RegularLog(nekomimi.INFO, "h ", bigMsg) // triggers rotation → suspended
	testForceFailNewFileCount.Store(0)

	// FatalLog in suspended: fp is nil so no write, but exit still happens
	h.FatalLog("h ", "fatal in suspend")
	assert.True(t, exitCalled, "fatal should still exit even in suspended")
}

// ============================================================
// TestFatalLog_SuspendedNoExitWhenWrapped
// ============================================================
func TestFatalLog_SuspendedNoExitWhenWrapped(t *testing.T) {
	dir := tempDir(t)
	tickCh := make(chan time.Time, 10)
	ctx := context.Background()

	origExit := exitFunc
	exitCalled := false
	exitFunc = func(code int) { exitCalled = true }
	defer func() { exitFunc = origExit }()

	h, err := New(ctx, Config{
		Path:        dir,
		FilePrefix:  "app",
		MaxFileSize: 1,
		WrapOnly:    true,
		testTickCh:  tickCh,
	})
	require.NoError(t, err)

	// Force suspended
	testForceFailNewFileCount.Store(100)
	bigMsg := strings.Repeat("x", 2048)
	h.RegularLog(nekomimi.INFO, "h ", bigMsg) // triggers rotation → suspended
	testForceFailNewFileCount.Store(0)

	// FatalLog in suspended with WrapOnly: no write, no exit
	h.FatalLog("h ", "fatal in suspend wrap")
	assert.False(t, exitCalled,
		"WrapOnly fatal should not exit in suspended")
}
