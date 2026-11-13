package sftpfs_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/w33ble/portunus/internal/testutil/sshdtest"
	"github.com/w33ble/portunus/internal/vfs"
)

func writeLocal(t *testing.T, p string, data []byte) {
	t.Helper()
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

func TestReaderAndWriterExposePipelinedInterfaces(t *testing.T) {
	t.Parallel()
	f := newRig(t).connect(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "f")
	writeLocal(t, p, []byte("x"))

	r, err := f.Open(t.Context(), p, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	if _, ok := r.(io.WriterTo); !ok {
		t.Fatal("reader does not implement io.WriterTo")
	}
	w, err := f.Create(t.Context(), filepath.Join(dir, "g"), vfs.CreateOpts{Size: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Abort() }()
	if _, ok := w.(io.ReaderFrom); !ok {
		t.Fatal("writer does not implement io.ReaderFrom")
	}
}

// roundTrip uploads via ReadFrom and downloads via WriteTo, returning bytes/sec for each.
func roundTrip(t *testing.T, f vfs.FS, p string, data []byte) (up, down float64) {
	t.Helper()
	ctx := t.Context()
	w, err := f.Create(ctx, p, vfs.CreateOpts{Size: int64(len(data))})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	// Hide bytes.Reader's WriterTo so io.Copy takes the writer's ReadFrom path.
	if _, err := io.Copy(w, struct{ io.Reader }{bytes.NewReader(data)}); err != nil {
		_ = w.Abort()
		t.Fatalf("upload: %v", err)
	}
	if err := w.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	up = float64(len(data)) / time.Since(start).Seconds()

	r, err := f.Open(ctx, p, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	h := sha256.New()
	start = time.Now()
	n, err := io.Copy(h, r) // io.Copy picks reader.WriteTo
	if err != nil || n != int64(len(data)) {
		t.Fatalf("download: n=%d err=%v", n, err)
	}
	down = float64(len(data)) / time.Since(start).Seconds()
	if want := sha256.Sum256(data); !bytes.Equal(h.Sum(nil), want[:]) {
		t.Fatal("downloaded content differs")
	}
	return up, down
}

func TestThroughputLocal64MiB(t *testing.T) {
	t.Parallel()
	skipUnderRace(t)
	f := newRig(t).connect(t)
	data := randomBytes(64 << 20)
	up, down := roundTrip(t, f, filepath.Join(t.TempDir(), "big"), data)
	t.Logf("64MiB loopback: up %.0f MB/s, down %.0f MB/s", up/1e6, down/1e6)
	if up < 5e6 || down < 5e6 {
		t.Fatalf("throughput too low: up %.1f MB/s, down %.1f MB/s", up/1e6, down/1e6)
	}
}

// With 50ms RTT a request-per-chunk client manages ~0.6 MB/s (32KiB / 50ms); only the
// pipelined WriteTo/ReadFrom paths get several times that. The floor catches that regression
// rather than enforcing the plan's 10 MB/s target, which shared CI runners can't hold.
func TestThroughputOverHighLatencyLink(t *testing.T) {
	t.Parallel()
	skipUnderRace(t)
	r := newRig(t)
	px := sshdtest.StartProxy(t, r.srv.Addr)
	px.SetLatency(25 * time.Millisecond)

	o := r.options(t)
	o.Port = px.Port
	o.StrictHostKeyChecking = "no"
	o.UserKnownHostsFiles = []string{filepath.Join(t.TempDir(), "kh")}
	f := r.connectOpts(t, o)

	data := randomBytes(32 << 20)
	up, down := roundTrip(t, f, filepath.Join(t.TempDir(), "big"), data)
	t.Logf("32MiB at 50ms RTT: up %.1f MB/s, down %.1f MB/s", up/1e6, down/1e6)
	if up < 4e6 || down < 4e6 {
		t.Fatalf("pipelining not effective: up %.1f MB/s, down %.1f MB/s", up/1e6, down/1e6)
	}
}

func TestTempFileLifecycle(t *testing.T) {
	t.Parallel()
	f := newRig(t).connect(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "doc.txt")
	tmps := func() []string {
		m, _ := filepath.Glob(filepath.Join(dir, ".doc.txt.*.portunus-tmp"))
		return m
	}

	w, err := f.Create(t.Context(), target, vfs.CreateOpts{Size: 5})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if len(tmps()) != 1 {
		t.Fatalf("staging files during write = %v, want exactly one", tmps())
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("target visible before Commit")
	}
	if err := w.Commit(); err != nil {
		t.Fatal(err)
	}
	if len(tmps()) != 0 {
		t.Fatalf("staging file left after Commit: %v", tmps())
	}
	if b, _ := os.ReadFile(target); string(b) != "hello" {
		t.Fatalf("target = %q", b)
	}
}

func TestSizeMismatchLeavesTargetAndNoTemp(t *testing.T) {
	t.Parallel()
	f := newRig(t).connect(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "t")
	writeLocal(t, target, []byte("original"))

	w, err := f.Create(t.Context(), target, vfs.CreateOpts{Size: 10})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("short"))
	if err := w.Commit(); !errors.Is(err, vfs.ErrSizeMismatch) {
		t.Fatalf("Commit err = %v", err)
	}
	if b, _ := os.ReadFile(target); string(b) != "original" {
		t.Fatalf("target changed: %q", b)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("leftover files: %v", entries)
	}
}

func TestCommitWritesThroughSymlink(t *testing.T) {
	t.Parallel()
	f := newRig(t).connect(t)
	dir := t.TempDir()
	real, link := filepath.Join(dir, "real"), filepath.Join(dir, "link")
	writeLocal(t, real, []byte("old"))
	if err := os.Symlink("real", link); err != nil { // relative link
		t.Fatal(err)
	}

	w, err := f.Create(t.Context(), link, vfs.CreateOpts{Size: 3})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("new"))
	if err := w.Commit(); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link was replaced by a regular file (err %v)", err)
	}
	if b, _ := os.ReadFile(real); string(b) != "new" {
		t.Fatalf("link target = %q, want new", b)
	}
}

func TestCommitIntoUnwritableDirWritesExistingFileInPlace(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("permission checks don't apply to root")
	}
	f := newRig(t).connect(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "t")
	writeLocal(t, target, []byte("old content"))
	if err := os.Chmod(dir, 0o555); err != nil { //nolint:gosec // read-only directory is the scenario
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) //nolint:gosec // restore so TempDir cleanup works

	w, err := f.Create(t.Context(), target, vfs.CreateOpts{Size: 3})
	if err != nil {
		t.Fatalf("Create in read-only dir over writable file: %v", err)
	}
	_, _ = w.Write([]byte("new"))
	if err := w.Commit(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(target); string(b) != "new" {
		t.Fatalf("target = %q", b)
	}

	_, err = f.Create(t.Context(), filepath.Join(dir, "fresh"), vfs.CreateOpts{Size: 1})
	if !errors.Is(err, vfs.ErrPermission) {
		t.Fatalf("new file in read-only dir: err = %v, want ErrPermission", err)
	}
}

func TestPermissionMapping(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("permission checks don't apply to root")
	}
	f := newRig(t).connect(t)
	p := filepath.Join(t.TempDir(), "secret")
	writeLocal(t, p, []byte("x"))
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Open(t.Context(), p, 0); !errors.Is(err, vfs.ErrPermission) {
		t.Fatalf("Open err = %v, want ErrPermission", err)
	}
}

// slowReader yields data in small pieces so an upload stays in flight until cancelled.
type slowReader struct{ started chan struct{} }

func (s *slowReader) Read(b []byte) (int, error) {
	select {
	case <-s.started:
	default:
		close(s.started)
	}
	time.Sleep(5 * time.Millisecond)
	n := min(len(b), 4096)
	clear(b[:n])
	return n, nil
}

func TestAbortStopsInFlightUploadAndRemovesTemp(t *testing.T) {
	t.Parallel()
	f := newRig(t).connect(t)
	dir := t.TempDir()
	w, err := f.Create(t.Context(), filepath.Join(dir, "big"), vfs.CreateOpts{Size: 1 << 40})
	if err != nil {
		t.Fatal(err)
	}
	src := &slowReader{started: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(w, struct{ io.Reader }{src})
		done <- err
	}()
	<-src.started
	if err := w.Abort(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("copy into aborted writer succeeded")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Abort did not stop the in-flight copy")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("files left after Abort: %v", entries)
	}
}

type blockingWriter struct{ started chan struct{} }

func (b *blockingWriter) Write(p []byte) (int, error) {
	select {
	case <-b.started:
	default:
		close(b.started)
	}
	time.Sleep(2 * time.Millisecond)
	return len(p), nil
}

func TestCloseStopsInFlightDownload(t *testing.T) {
	t.Parallel()
	f := newRig(t).connect(t)
	p := filepath.Join(t.TempDir(), "big")
	writeLocal(t, p, randomBytes(64<<20))

	r, err := f.Open(t.Context(), p, 0)
	if err != nil {
		t.Fatal(err)
	}
	dst := &blockingWriter{started: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(dst, r)
		done <- err
	}()
	<-dst.started
	closed := make(chan struct{})
	go func() { _ = r.Close(); close(closed) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("download finished despite Close")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not stop the in-flight download")
	}
	<-closed
}

func TestContextCancelStopsTransfers(t *testing.T) {
	t.Parallel()
	f := newRig(t).connect(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "big")
	writeLocal(t, p, randomBytes(32<<20))

	ctx, cancel := context.WithCancel(t.Context())
	r, err := f.Open(ctx, p, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	cancel()
	if _, err := io.Copy(io.Discard, r); !errors.Is(err, context.Canceled) {
		t.Fatalf("download after cancel: err = %v", err)
	}
	if _, err := f.Stat(ctx, p); !errors.Is(err, context.Canceled) {
		t.Fatalf("Stat with cancelled ctx: err = %v", err)
	}
}

func TestCloseClosesSSHConnection(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	f := r.connect(t)
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := f.Home(t.Context()); err == nil {
		t.Fatal("Home succeeded after Close")
	}
	if err := f.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestBrowseNotBlockedByTransfer(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	px := sshdtest.StartProxy(t, r.srv.Addr)
	px.SetLatency(20 * time.Millisecond)
	o := r.options(t)
	o.Port = px.Port
	o.StrictHostKeyChecking = "no"
	o.UserKnownHostsFiles = []string{filepath.Join(t.TempDir(), "kh")}
	f := r.connectOpts(t, o)

	dir := t.TempDir()
	big := filepath.Join(dir, "big")
	writeLocal(t, big, randomBytes(24<<20))
	rd, err := f.Open(t.Context(), big, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rd.Close() }()
	started := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_, _ = io.Copy(&blockingWriterFast{started: started}, rd)
	}()
	<-started

	start := time.Now()
	if _, err := f.Stat(t.Context(), dir); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("Stat took %v while a transfer was running", d)
	}
	_ = rd.Close()
	<-finished
}

type blockingWriterFast struct{ started chan struct{} }

func (b *blockingWriterFast) Write(p []byte) (int, error) {
	select {
	case <-b.started:
	default:
		close(b.started)
	}
	return len(p), nil
}

func TestSymlinkEntryModeFromTarget(t *testing.T) {
	t.Parallel()
	f := newRig(t).connect(t)
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	writeLocal(t, real, []byte("x"))
	if err := os.Chmod(real, 0o640); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink("real", link); err != nil {
		t.Fatal(err)
	}
	e, err := f.Stat(t.Context(), link)
	if err != nil {
		t.Fatal(err)
	}
	if e.Mode != 0o640 {
		t.Fatalf("link entry Mode = %v, want target's 0640", e.Mode)
	}
}
