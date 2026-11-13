// Package vfstest is the conformance suite every vfs.FS backend runs.
//
// Cases use only vfs.FS methods plus the Harness.Symlink hook, so they work
// against any backend.
package vfstest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/w33ble/portunus/internal/vfs"
)

// Caps declares optional behaviors a backend supports.
type Caps struct {
	Symlinks   bool // backend can create/report symlinks (test creates them via Symlink hook)
	Modes      bool // CreateOpts.Mode honored, Entry.Mode meaningful
	ModTimes   bool // CreateOpts.ModTime honored
	EmptyDirs  bool // Mkdir'd empty dirs are listed (s3: marker objects → true)
	RenameDirs bool // Rename works on directories
}

// Harness adapts a backend to the suite.
type Harness struct {
	// New returns a fresh FS and an existing, empty, writable root dir inside it.
	// The harness owns cleanup of the FS (register it with t.Cleanup).
	New  func(t *testing.T) (fs vfs.FS, root string)
	Caps Caps
	// Symlink creates a symlink at link pointing to target (only called if Caps.Symlinks).
	// Both paths are absolute vfs paths.
	Symlink func(t *testing.T, target, link string)
}

// Run executes every conformance case as a subtest.
func Run(t *testing.T, h Harness) {
	t.Helper()
	cases := []struct {
		name string
		fn   func(*testing.T, Harness)
	}{
		{"ListPaging", testListPaging},
		{"ListStopsOnPageError", testListStopsOnPageError},
		{"ListEmptyDir", testListEmptyDir},
		{"StatFileAndDir", testStat},
		{"NotExist", testNotExist},
		{"OpenAtOffset", testOpenOffset},
		{"CreateCommit", testCreateCommit},
		{"CreateEmptyFile", testCreateEmpty},
		{"CreateAbort", testCreateAbort},
		{"CommitReplacesExisting", testCommitReplaces},
		{"AbortKeepsExisting", testAbortKeepsExisting},
		{"ShortWriteSizeMismatch", testSizeMismatch},
		{"Mode", testMode},
		{"ModTime", testModTime},
		{"MkdirParentsIdempotent", testMkdir},
		{"EmptyDirListed", testEmptyDirListed},
		{"RemoveFileAndEmptyDir", testRemove},
		{"RemoveNonEmptyDirFails", testRemoveNonEmpty},
		{"RenameFile", testRenameFile},
		{"RenameOntoExisting", testRenameExisting},
		{"RenameDir", testRenameDir},
		{"Symlinks", testSymlinks},
		{"Home", testHome},
		{"ConcurrentCreate", testConcurrentCreate},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { c.fn(t, h) })
	}
}

func setup(t *testing.T, h Harness) (vfs.FS, string) {
	t.Helper()
	f, root := h.New(t)
	if f == nil || root == "" {
		t.Fatal("harness New returned nil FS or empty root")
	}
	return f, root
}

func ctxFor(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	return ctx
}

func content(n int, seed byte) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte((i*7 + int(seed)) & 0xff)
	}
	return b
}

func put(t *testing.T, f vfs.FS, p string, data []byte, opts vfs.CreateOpts) {
	t.Helper()
	if err := putErr(ctxFor(t), f, p, data, opts); err != nil {
		t.Fatalf("put %s: %v", p, err)
	}
}

func putErr(ctx context.Context, f vfs.FS, p string, data []byte, opts vfs.CreateOpts) error {
	opts.Size = int64(len(data))
	w, err := f.Create(ctx, p, opts)
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		_ = w.Abort()
		return fmt.Errorf("write: %w", err)
	}
	if err := w.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func get(t *testing.T, f vfs.FS, p string, offset int64) []byte {
	t.Helper()
	r, err := f.Open(ctxFor(t), p, offset)
	if err != nil {
		t.Fatalf("open %s@%d: %v", p, offset, err)
	}
	defer func() { _ = r.Close() }()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read %s@%d: %v", p, offset, err)
	}
	return b
}

func listAll(t *testing.T, f vfs.FS, dir string) []vfs.Entry {
	t.Helper()
	var out []vfs.Entry
	err := f.List(ctxFor(t), dir, func(es []vfs.Entry) error {
		out = append(out, es...)
		return nil
	})
	if err != nil {
		t.Fatalf("list %s: %v", dir, err)
	}
	return out
}

func names(es []vfs.Entry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Name
	}
	sort.Strings(out)
	return out
}

func find(es []vfs.Entry, name string) (vfs.Entry, bool) {
	for _, e := range es {
		if e.Name == name {
			return e, true
		}
	}
	return vfs.Entry{}, false
}

func wantNames(t *testing.T, f vfs.FS, dir string, want ...string) {
	t.Helper()
	got := names(listAll(t, f, dir))
	sort.Strings(want)
	if want == nil {
		want = []string{}
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("list %s = %v, want %v", dir, got, want)
	}
}

func wantContent(t *testing.T, f vfs.FS, p string, want []byte) {
	t.Helper()
	if got := get(t, f, p, 0); !bytes.Equal(got, want) {
		t.Fatalf("content of %s differs: got %d bytes, want %d bytes", p, len(got), len(want))
	}
}

func wantErr(t *testing.T, what string, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("%s: err = %v, want errors.Is %v", what, err, target)
	}
}

func testListPaging(t *testing.T, h Harness) {
	f, root := setup(t, h)
	const n = 1500
	var wg sync.WaitGroup
	errs := make(chan error, n)
	sem := make(chan struct{}, 16)
	ctx := ctxFor(t)
	for i := range n {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			p := path.Join(root, fmt.Sprintf("f%04d.txt", i))
			if err := putErr(ctx, f, p, []byte{byte(i & 0xff)}, vfs.CreateOpts{}); err != nil {
				errs <- fmt.Errorf("%s: %w", p, err)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	pages := 0
	err := f.List(ctx, root, func(es []vfs.Entry) error {
		pages++
		for _, e := range es {
			if seen[e.Name] {
				t.Errorf("duplicate entry %q", e.Name)
			}
			seen[e.Name] = true
			if want := path.Join(root, e.Name); e.Path != want {
				t.Errorf("entry %q Path = %q, want %q", e.Name, e.Path, want)
			}
			if e.IsDir || e.Size != 1 {
				t.Errorf("entry %q: IsDir=%v Size=%d, want file of size 1", e.Name, e.IsDir, e.Size)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if pages == 0 {
		t.Fatal("page callback never called")
	}
	if len(seen) != n {
		t.Fatalf("listed %d unique entries, want %d", len(seen), n)
	}
	for i := range n {
		if name := fmt.Sprintf("f%04d.txt", i); !seen[name] {
			t.Fatalf("missing entry %q", name)
		}
	}
}

func testListStopsOnPageError(t *testing.T, h Harness) {
	f, root := setup(t, h)
	for i := range 5 {
		put(t, f, path.Join(root, fmt.Sprintf("f%d", i)), []byte("x"), vfs.CreateOpts{})
	}
	stop := errors.New("stop here")
	calls := 0
	err := f.List(ctxFor(t), root, func([]vfs.Entry) error {
		calls++
		return stop
	})
	wantErr(t, "list with failing page", err, stop)
	if calls != 1 {
		t.Fatalf("page called %d times after returning an error, want 1", calls)
	}
}

func testListEmptyDir(t *testing.T, h Harness) {
	f, root := setup(t, h)
	wantNames(t, f, root)
}

func testStat(t *testing.T, h Harness) {
	f, root := setup(t, h)
	ctx := ctxFor(t)
	data := content(123, 1)
	fp := path.Join(root, "file.bin")
	put(t, f, fp, data, vfs.CreateOpts{})
	dp := path.Join(root, "sub")
	if err := f.Mkdir(ctx, dp); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	e, err := f.Stat(ctx, fp)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if e.Name != "file.bin" || e.Path != fp || e.IsDir || e.Size != int64(len(data)) || e.IsSymlink() {
		t.Fatalf("stat file = %+v", e)
	}
	e, err = f.Stat(ctx, dp)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if e.Name != "sub" || e.Path != dp || !e.IsDir {
		t.Fatalf("stat dir = %+v", e)
	}
	if e, err = f.Stat(ctx, root); err != nil || !e.IsDir {
		t.Fatalf("stat root = %+v, %v", e, err)
	}
}

func testNotExist(t *testing.T, h Harness) {
	f, root := setup(t, h)
	ctx := ctxFor(t)
	missing := path.Join(root, "missing")
	_, err := f.Stat(ctx, missing)
	wantErr(t, "stat", err, vfs.ErrNotExist)
	_, err = f.Open(ctx, missing, 0)
	wantErr(t, "open", err, vfs.ErrNotExist)
	err = f.List(ctx, missing, func([]vfs.Entry) error { return nil })
	wantErr(t, "list", err, vfs.ErrNotExist)
	wantErr(t, "remove", f.Remove(ctx, missing), vfs.ErrNotExist)
	wantErr(t, "rename", f.Rename(ctx, missing, path.Join(root, "other")), vfs.ErrNotExist)
}

func testOpenOffset(t *testing.T, h Harness) {
	f, root := setup(t, h)
	data := content(1000, 3)
	p := path.Join(root, "data.bin")
	put(t, f, p, data, vfs.CreateOpts{})
	for _, off := range []int64{0, 1, 100, 512, 999} {
		if got := get(t, f, p, off); !bytes.Equal(got, data[off:]) {
			t.Fatalf("open at offset %d: got %d bytes, want %d", off, len(got), len(data)-int(off))
		}
	}
}

func testCreateCommit(t *testing.T, h Harness) {
	f, root := setup(t, h)
	ctx := ctxFor(t)
	data := content(1<<20+17, 5)
	p := path.Join(root, "big.bin")
	w, err := f.Create(ctx, p, vfs.CreateOpts{Size: int64(len(data))})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Write in odd-sized chunks.
	for rest := data; len(rest) > 0; {
		n := min(len(rest), 100003)
		if _, err := w.Write(rest[:n]); err != nil {
			_ = w.Abort()
			t.Fatalf("write: %v", err)
		}
		rest = rest[n:]
	}
	if w.Written() != int64(len(data)) {
		t.Fatalf("Written = %d, want %d", w.Written(), len(data))
	}
	if _, err := f.Stat(ctx, p); !errors.Is(err, vfs.ErrNotExist) {
		t.Fatalf("target visible before commit: %v", err)
	}
	if err := w.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	wantNames(t, f, root, "big.bin")
	wantContent(t, f, p, data)
	if e, err := f.Stat(ctx, p); err != nil || e.Size != int64(len(data)) {
		t.Fatalf("stat after commit = %+v, %v", e, err)
	}
}

func testCreateEmpty(t *testing.T, h Harness) {
	f, root := setup(t, h)
	p := path.Join(root, "empty")
	put(t, f, p, nil, vfs.CreateOpts{})
	wantNames(t, f, root, "empty")
	wantContent(t, f, p, nil)
}

func testCreateAbort(t *testing.T, h Harness) {
	f, root := setup(t, h)
	ctx := ctxFor(t)
	p := path.Join(root, "aborted.bin")
	w, err := f.Create(ctx, p, vfs.CreateOpts{Size: 100})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := w.Write(content(50, 1)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Abort(); err != nil {
		t.Fatalf("abort: %v", err)
	}
	_, err = f.Stat(ctx, p)
	wantErr(t, "stat after abort", err, vfs.ErrNotExist)
	wantNames(t, f, root)
}

func testCommitReplaces(t *testing.T, h Harness) {
	f, root := setup(t, h)
	p := path.Join(root, "f")
	put(t, f, p, content(500, 1), vfs.CreateOpts{})
	repl := content(200, 9)
	put(t, f, p, repl, vfs.CreateOpts{})
	wantContent(t, f, p, repl)
	wantNames(t, f, root, "f")
	if e, err := f.Stat(ctxFor(t), p); err != nil || e.Size != int64(len(repl)) {
		t.Fatalf("stat = %+v, %v", e, err)
	}
}

func testAbortKeepsExisting(t *testing.T, h Harness) {
	f, root := setup(t, h)
	p := path.Join(root, "f")
	orig := content(300, 2)
	put(t, f, p, orig, vfs.CreateOpts{})
	w, err := f.Create(ctxFor(t), p, vfs.CreateOpts{Size: 10})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := w.Write(content(10, 8)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Abort(); err != nil {
		t.Fatalf("abort: %v", err)
	}
	wantContent(t, f, p, orig)
	wantNames(t, f, root, "f")
}

func testSizeMismatch(t *testing.T, h Harness) {
	f, root := setup(t, h)
	ctx := ctxFor(t)

	// New target: nothing appears.
	p := path.Join(root, "short.bin")
	w, err := f.Create(ctx, p, vfs.CreateOpts{Size: 100})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := w.Write(content(40, 1)); err != nil {
		t.Fatalf("write: %v", err)
	}
	wantErr(t, "commit short write", w.Commit(), vfs.ErrSizeMismatch)
	_, err = f.Stat(ctx, p)
	wantErr(t, "stat after failed commit", err, vfs.ErrNotExist)
	wantNames(t, f, root)

	// Existing target: unchanged.
	q := path.Join(root, "keep.bin")
	orig := content(64, 4)
	put(t, f, q, orig, vfs.CreateOpts{})
	w, err = f.Create(ctx, q, vfs.CreateOpts{Size: 100})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := w.Write(content(10, 6)); err != nil {
		t.Fatalf("write: %v", err)
	}
	wantErr(t, "commit short write over existing", w.Commit(), vfs.ErrSizeMismatch)
	wantContent(t, f, q, orig)
	wantNames(t, f, root, "keep.bin")
}

func testMode(t *testing.T, h Harness) {
	if !h.Caps.Modes {
		t.Skip("backend does not honor modes")
	}
	f, root := setup(t, h)
	ctx := ctxFor(t)
	p := path.Join(root, "m")
	put(t, f, p, []byte("hello"), vfs.CreateOpts{Mode: 0o640})
	e, err := f.Stat(ctx, p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := e.Mode.Perm(); got != 0o640 {
		t.Fatalf("stat mode = %v, want 0640", got)
	}
	le, ok := find(listAll(t, f, root), "m")
	if !ok || le.Mode.Perm() != 0o640 {
		t.Fatalf("list entry = %+v (found=%v), want mode 0640", le, ok)
	}

	// Overwrite with Mode 0 keeps the existing mode.
	put(t, f, p, []byte("again"), vfs.CreateOpts{})
	if e, err = f.Stat(ctx, p); err != nil || e.Mode.Perm() != 0o640 {
		t.Fatalf("mode after overwrite = %v (err %v), want 0640 preserved", e.Mode.Perm(), err)
	}
	// Overwrite with an explicit mode applies it.
	put(t, f, p, []byte("third"), vfs.CreateOpts{Mode: 0o600})
	if e, err = f.Stat(ctx, p); err != nil || e.Mode.Perm() != 0o600 {
		t.Fatalf("mode after explicit overwrite = %v (err %v), want 0600", e.Mode.Perm(), err)
	}
}

func testModTime(t *testing.T, h Harness) {
	if !h.Caps.ModTimes {
		t.Skip("backend does not honor modification times")
	}
	f, root := setup(t, h)
	ctx := ctxFor(t)
	mt := time.Date(2021, 3, 4, 5, 6, 7, 0, time.UTC)
	p := path.Join(root, "t")
	put(t, f, p, []byte("hello"), vfs.CreateOpts{ModTime: mt})
	e, err := f.Stat(ctx, p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !e.ModTime.Equal(mt) {
		t.Fatalf("stat modtime = %v, want %v", e.ModTime, mt)
	}
	le, ok := find(listAll(t, f, root), "t")
	if !ok || !le.ModTime.Equal(mt) {
		t.Fatalf("list entry = %+v (found=%v), want modtime %v", le, ok, mt)
	}
}

func testMkdir(t *testing.T, h Harness) {
	f, root := setup(t, h)
	ctx := ctxFor(t)
	deep := path.Join(root, "a", "b", "c")
	if err := f.Mkdir(ctx, deep); err != nil {
		t.Fatalf("mkdir deep: %v", err)
	}
	for _, p := range []string{path.Join(root, "a"), path.Join(root, "a", "b"), deep} {
		e, err := f.Stat(ctx, p)
		if err != nil || !e.IsDir {
			t.Fatalf("stat %s = %+v, %v; want dir", p, e, err)
		}
	}
	if err := f.Mkdir(ctx, deep); err != nil {
		t.Fatalf("mkdir existing: %v", err)
	}
	// Idempotent even when it has content.
	put(t, f, path.Join(deep, "x"), []byte("x"), vfs.CreateOpts{})
	if err := f.Mkdir(ctx, deep); err != nil {
		t.Fatalf("mkdir non-empty existing: %v", err)
	}
	wantNames(t, f, deep, "x")
}

func testEmptyDirListed(t *testing.T, h Harness) {
	if !h.Caps.EmptyDirs {
		t.Skip("backend does not list empty dirs")
	}
	f, root := setup(t, h)
	if err := f.Mkdir(ctxFor(t), path.Join(root, "empty")); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	e, ok := find(listAll(t, f, root), "empty")
	if !ok || !e.IsDir {
		t.Fatalf("empty dir not listed as dir: %+v (found=%v)", e, ok)
	}
	wantNames(t, f, path.Join(root, "empty"))
}

func testRemove(t *testing.T, h Harness) {
	f, root := setup(t, h)
	ctx := ctxFor(t)
	fp := path.Join(root, "f")
	put(t, f, fp, []byte("x"), vfs.CreateOpts{})
	if err := f.Remove(ctx, fp); err != nil {
		t.Fatalf("remove file: %v", err)
	}
	_, err := f.Stat(ctx, fp)
	wantErr(t, "stat removed file", err, vfs.ErrNotExist)

	dp := path.Join(root, "d")
	if err := f.Mkdir(ctx, dp); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := f.Remove(ctx, dp); err != nil {
		t.Fatalf("remove empty dir: %v", err)
	}
	_, err = f.Stat(ctx, dp)
	wantErr(t, "stat removed dir", err, vfs.ErrNotExist)
	wantNames(t, f, root)
}

func testRemoveNonEmpty(t *testing.T, h Harness) {
	f, root := setup(t, h)
	ctx := ctxFor(t)
	dp := path.Join(root, "d")
	if err := f.Mkdir(ctx, dp); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	child := path.Join(dp, "child")
	put(t, f, child, []byte("data"), vfs.CreateOpts{})
	if err := f.Remove(ctx, dp); err == nil {
		t.Fatal("remove of non-empty dir succeeded, want error")
	}
	wantContent(t, f, child, []byte("data"))
	wantNames(t, f, dp, "child")
}

func testRenameFile(t *testing.T, h Harness) {
	f, root := setup(t, h)
	ctx := ctxFor(t)
	data := content(333, 7)
	from, to := path.Join(root, "from"), path.Join(root, "to")
	put(t, f, from, data, vfs.CreateOpts{})
	if err := f.Rename(ctx, from, to); err != nil {
		t.Fatalf("rename: %v", err)
	}
	_, err := f.Stat(ctx, from)
	wantErr(t, "stat renamed-from", err, vfs.ErrNotExist)
	wantContent(t, f, to, data)
	wantNames(t, f, root, "to")
}

func testRenameExisting(t *testing.T, h Harness) {
	f, root := setup(t, h)
	ctx := ctxFor(t)
	a, b := path.Join(root, "a"), path.Join(root, "b")
	da, db := content(10, 1), content(20, 2)
	put(t, f, a, da, vfs.CreateOpts{})
	put(t, f, b, db, vfs.CreateOpts{})
	wantErr(t, "rename onto existing", f.Rename(ctx, a, b), vfs.ErrExist)
	wantContent(t, f, a, da)
	wantContent(t, f, b, db)
	wantNames(t, f, root, "a", "b")
}

func testRenameDir(t *testing.T, h Harness) {
	f, root := setup(t, h)
	ctx := ctxFor(t)
	src, dst := path.Join(root, "src"), path.Join(root, "dst")
	if err := f.Mkdir(ctx, src); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	put(t, f, path.Join(src, "f"), []byte("in dir"), vfs.CreateOpts{})
	if !h.Caps.RenameDirs {
		err := f.Rename(ctx, src, dst)
		if err == nil {
			t.Fatal("rename dir succeeded on backend declaring RenameDirs=false")
		}
		wantErr(t, "rename dir", err, vfs.ErrUnsupported)
		wantContent(t, f, path.Join(src, "f"), []byte("in dir"))
		return
	}
	if err := f.Rename(ctx, src, dst); err != nil {
		t.Fatalf("rename dir: %v", err)
	}
	_, err := f.Stat(ctx, src)
	wantErr(t, "stat renamed-from dir", err, vfs.ErrNotExist)
	if e, err := f.Stat(ctx, dst); err != nil || !e.IsDir {
		t.Fatalf("stat renamed dir = %+v, %v", e, err)
	}
	wantContent(t, f, path.Join(dst, "f"), []byte("in dir"))
}

type linkCase struct {
	name, link, target string
	isDir              bool
	size               int64 // -1: don't check
}

func (c linkCase) check(t *testing.T) func(src string, e vfs.Entry) {
	return func(src string, e vfs.Entry) {
		t.Helper()
		if e.Name != c.name || e.Path != c.link {
			t.Errorf("%s %s: name/path = %q %q", src, c.name, e.Name, e.Path)
		}
		if !e.IsSymlink() || e.Link != c.target {
			t.Errorf("%s %s: Link = %q, want %q", src, c.name, e.Link, c.target)
		}
		if e.IsDir != c.isDir {
			t.Errorf("%s %s: IsDir = %v, want %v", src, c.name, e.IsDir, c.isDir)
		}
		if c.size >= 0 && e.Size != c.size {
			t.Errorf("%s %s: Size = %d, want %d", src, c.name, e.Size, c.size)
		}
	}
}

func testSymlinks(t *testing.T, h Harness) {
	if !h.Caps.Symlinks {
		t.Skip("backend has no symlinks")
	}
	if h.Symlink == nil {
		t.Fatal("Caps.Symlinks set but Harness.Symlink is nil")
	}
	f, root := setup(t, h)
	ctx := ctxFor(t)
	data := content(77, 1)
	file := path.Join(root, "file")
	dir := path.Join(root, "dir")
	put(t, f, file, data, vfs.CreateOpts{})
	if err := f.Mkdir(ctx, dir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	put(t, f, path.Join(dir, "inner"), []byte("inner"), vfs.CreateOpts{})
	missing := path.Join(root, "nowhere")

	linkFile := path.Join(root, "lfile")
	linkDir := path.Join(root, "ldir")
	linkDead := path.Join(root, "ldead")
	h.Symlink(t, file, linkFile)
	h.Symlink(t, dir, linkDir)
	h.Symlink(t, missing, linkDead)

	cases := []linkCase{
		{"lfile", linkFile, file, false, int64(len(data))},
		{"ldir", linkDir, dir, true, -1},
		{"ldead", linkDead, missing, false, 0},
	}
	listed := listAll(t, f, root)
	for _, c := range cases {
		check := c.check(t)
		le, ok := find(listed, c.name)
		if !ok {
			t.Errorf("list: %s missing", c.name)
		} else {
			check("list", le)
		}
		se, err := f.Stat(ctx, c.link)
		if err != nil {
			t.Errorf("stat %s: %v", c.name, err)
			continue
		}
		check("stat", se)
	}
	// Plain entries are not symlinks.
	if e, ok := find(listed, "file"); !ok || e.IsSymlink() {
		t.Errorf("plain file listed as %+v (found=%v)", e, ok)
	}
	wantContent(t, f, linkFile, data)
	// Removing a link removes the link, not its target.
	if err := f.Remove(ctx, linkFile); err != nil {
		t.Fatalf("remove symlink: %v", err)
	}
	wantContent(t, f, file, data)
	_, err := f.Stat(ctx, linkFile)
	wantErr(t, "stat removed symlink", err, vfs.ErrNotExist)
	if err := f.Remove(ctx, linkDir); err != nil {
		t.Fatalf("remove dir symlink: %v", err)
	}
	wantContent(t, f, path.Join(dir, "inner"), []byte("inner"))
}

func testHome(t *testing.T, h Harness) {
	f, _ := setup(t, h)
	ctx := ctxFor(t)
	home, err := f.Home(ctx)
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	if home == "" {
		t.Fatal("home is empty")
	}
	e, err := f.Stat(ctx, home)
	if err != nil || !e.IsDir {
		t.Fatalf("stat home %q = %+v, %v; want existing dir", home, e, err)
	}
	if err := f.List(ctx, home, func([]vfs.Entry) error { return nil }); err != nil {
		t.Fatalf("list home %q: %v", home, err)
	}
}

func testConcurrentCreate(t *testing.T, h Harness) {
	f, root := setup(t, h)
	ctx := ctxFor(t)
	const n = 24
	want := make(map[string][]byte, n)
	var wg sync.WaitGroup
	for i := range n {
		name := fmt.Sprintf("c%02d", i)
		data := content(1000+i*997, byte(i))
		want[name] = data
		wg.Go(func() {
			if err := putErr(ctx, f, path.Join(root, name), data, vfs.CreateOpts{}); err != nil {
				t.Errorf("concurrent put %s: %v", name, err)
			}
		})
	}
	wg.Wait()
	if t.Failed() {
		return
	}
	got := listAll(t, f, root)
	if len(got) != n {
		t.Fatalf("listed %d entries, want %d: %v", len(got), n, names(got))
	}
	for name, data := range want {
		wantContent(t, f, path.Join(root, name), data)
	}
}
