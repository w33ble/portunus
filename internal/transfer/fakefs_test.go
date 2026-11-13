package transfer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/w33ble/portunus/internal/vfs"
)

var errFakeClosed = errors.New("fake: closed")

// gauge tracks a concurrent count and its high-water mark.
type gauge struct{ cur, peak atomic.Int64 }

func (g *gauge) inc() {
	n := g.cur.Add(1)
	for {
		p := g.peak.Load()
		if n <= p || g.peak.CompareAndSwap(p, n) {
			return
		}
	}
}

func (g *gauge) dec() { g.cur.Add(-1) }

type fnode struct {
	dir   bool
	data  []byte
	link  string
	mode  fs.FileMode
	mtime time.Time
}

type failure struct {
	err  error
	left int // <0: forever
}

// fakeFS is an in-memory vfs.FS. Every operation first passes through pre, which applies
// latency, injected failures, and the blocking hook, so tests can stall or break any step.
type fakeFS struct {
	name string

	mu       sync.Mutex
	nodes    map[string]*fnode
	extra    map[string][]vfs.Entry // raw entries List adds to a directory
	failures map[string]*failure

	hook             func(ctx context.Context, op, p string) error
	latency          time.Duration
	chunk            int // max bytes per Read (0 = unlimited)
	pageSize         int
	readerWriterTo   bool
	writerReaderFrom bool
	lenientCommit    bool // Commit skips the Written()==Size check

	listCalls  map[string]int
	statCalls  map[string]int
	opens      map[string]int
	creates    map[string]int
	removed    []string
	commits    int
	aborts     int
	misuse     []string
	maxReadBuf int
	viaWriteTo int
	viaReadFrm int

	readers, writers, active gauge
	total                    *gauge // optional, shared: counts open readers across filesystems
}

func newFake(name string) *fakeFS {
	return &fakeFS{
		name:      name,
		nodes:     map[string]*fnode{"/": {dir: true, mode: fs.ModeDir | 0o755}},
		extra:     map[string][]vfs.Entry{},
		failures:  map[string]*failure{},
		pageSize:  3,
		listCalls: map[string]int{},
		statCalls: map[string]int{},
		opens:     map[string]int{},
		creates:   map[string]int{},
	}
}

// ---- tree building and inspection ----

func (f *fakeFS) mkdirLocked(p string) error {
	p = path.Clean(p)
	cur := "/"
	for part := range strings.SplitSeq(strings.Trim(p, "/"), "/") {
		if part == "" {
			continue
		}
		cur = path.Join(cur, part)
		n, ok := f.nodes[cur]
		switch {
		case !ok:
			f.nodes[cur] = &fnode{dir: true, mode: fs.ModeDir | 0o755, mtime: time.Now()}
		case !n.dir:
			return fmt.Errorf("fake mkdir %s: %w", cur, vfs.ErrExist)
		}
	}
	return nil
}

func (f *fakeFS) addFile(p, content string) *fakeFS {
	f.mu.Lock()
	defer f.mu.Unlock()
	_ = f.mkdirLocked(path.Dir(p))
	f.nodes[p] = &fnode{data: []byte(content), mode: 0o644, mtime: time.Unix(1_700_000_000, 0)}
	return f
}

func (f *fakeFS) addDir(p string) *fakeFS {
	f.mu.Lock()
	defer f.mu.Unlock()
	_ = f.mkdirLocked(p)
	return f
}

func (f *fakeFS) addLink(link, target string) *fakeFS {
	f.mu.Lock()
	defer f.mu.Unlock()
	_ = f.mkdirLocked(path.Dir(link))
	f.nodes[link] = &fnode{link: target, mode: fs.ModeSymlink | 0o777}
	return f
}

func (f *fakeFS) addEntry(dir string, e vfs.Entry) *fakeFS {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.extra[dir] = append(f.extra[dir], e)
	return f
}

func (f *fakeFS) setMode(p string, m fs.FileMode) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nodes[p].mode = m
}

func (f *fakeFS) content(p string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.nodes[p]
	if !ok || n.dir {
		return "", false
	}
	return string(n.data), true
}

func (f *fakeFS) exists(p string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.nodes[p]
	return ok
}

func (f *fakeFS) node(p string) fnode {
	f.mu.Lock()
	defer f.mu.Unlock()
	if n, ok := f.nodes[p]; ok {
		return *n
	}
	return fnode{}
}

// tree lists every path below dir (dirs end in "/"), sorted.
func (f *fakeFS) tree(dir string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for p, n := range f.nodes {
		if p == dir || !strings.HasPrefix(p, strings.TrimSuffix(dir, "/")+"/") {
			continue
		}
		if n.dir {
			p += "/"
		}
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}

func (f *fakeFS) setHook(h func(ctx context.Context, op, p string) error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hook = h
}

func (f *fakeFS) failOn(op, p string, err error) { f.failTimes(op, p, -1, err) }

func (f *fakeFS) failTimes(op, p string, n int, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures[op+" "+p] = &failure{err: err, left: n}
}

func (f *fakeFS) setLatency(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.latency = d
}

func (f *fakeFS) configure(fn func(*fakeFS)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *fakeFS) count(m map[string]int, p string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return m[p]
}

func (f *fakeFS) counters() (commits, aborts int, misuse []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.commits, f.aborts, slices.Clone(f.misuse)
}

func (f *fakeFS) removedPaths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.removed)
}

// pre applies latency, the blocking hook, and injected failures.
func (f *fakeFS) pre(ctx context.Context, op, p string) error {
	f.mu.Lock()
	lat, hook := f.latency, f.hook
	var injected error
	if fl := f.failures[op+" "+p]; fl != nil && fl.left != 0 {
		injected = fl.err
		if fl.left > 0 {
			fl.left--
		}
	}
	f.mu.Unlock()
	if lat > 0 {
		select {
		case <-time.After(lat):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if hook != nil {
		if err := hook(ctx, op, p); err != nil {
			return err
		}
	}
	return injected
}

// ---- vfs.FS ----

func (f *fakeFS) Kind() vfs.Kind                       { return vfs.KindLocal }
func (f *fakeFS) Home(context.Context) (string, error) { return "/", nil }
func (f *fakeFS) Close() error                         { return nil }

func notExist(op, p string) error { return fmt.Errorf("fake %s %s: %w", op, p, vfs.ErrNotExist) }

// resolveLocked follows symlinks; it returns the real path and node, or nil for dangling or looping links.
func (f *fakeFS) resolveLocked(p string) (string, *fnode) {
	for range 16 {
		n, ok := f.nodes[p]
		if !ok {
			return "", nil
		}
		if n.link == "" {
			return p, n
		}
		p = n.link
	}
	return "", nil
}

func (f *fakeFS) entryLocked(p string, n *fnode) vfs.Entry {
	e := vfs.Entry{Name: path.Base(p), Path: p, Mode: n.mode, ModTime: n.mtime, IsDir: n.dir, Size: int64(len(n.data))}
	if n.link != "" {
		e.Link = n.link
		e.IsDir, e.Size = false, 0
		if _, t := f.resolveLocked(p); t != nil {
			e.IsDir, e.Size, e.Mode, e.ModTime = t.dir, int64(len(t.data)), t.mode, t.mtime
		}
	}
	return e
}

func (f *fakeFS) List(ctx context.Context, dir string, page func([]vfs.Entry) error) error {
	dir = path.Clean(dir)
	f.mu.Lock()
	f.listCalls[dir]++
	f.mu.Unlock()
	if err := f.pre(ctx, "list", dir); err != nil {
		return err
	}
	f.mu.Lock()
	real, n := f.resolveLocked(dir)
	if n == nil {
		f.mu.Unlock()
		return notExist("list", dir)
	}
	if !n.dir {
		f.mu.Unlock()
		return fmt.Errorf("fake list %s: not a directory", dir)
	}
	var entries []vfs.Entry
	prefix := strings.TrimSuffix(real, "/") + "/"
	for p, c := range f.nodes {
		if p != real && strings.HasPrefix(p, prefix) && !strings.Contains(p[len(prefix):], "/") {
			entries = append(entries, f.entryLocked(p, c))
		}
	}
	entries = append(entries, f.extra[dir]...)
	size := f.pageSize
	f.mu.Unlock()

	slices.SortFunc(entries, func(a, b vfs.Entry) int { return strings.Compare(a.Name, b.Name) })
	for len(entries) > 0 {
		k := min(size, len(entries))
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := page(entries[:k]); err != nil {
			return err
		}
		entries = entries[k:]
	}
	return nil
}

func (f *fakeFS) Stat(ctx context.Context, p string) (vfs.Entry, error) {
	p = path.Clean(p)
	f.mu.Lock()
	f.statCalls[p]++
	f.mu.Unlock()
	if err := f.pre(ctx, "stat", p); err != nil {
		return vfs.Entry{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.nodes[p]
	if !ok {
		return vfs.Entry{}, notExist("stat", p)
	}
	return f.entryLocked(p, n), nil
}

func (f *fakeFS) Open(ctx context.Context, p string, offset int64) (io.ReadCloser, error) {
	p = path.Clean(p)
	f.readers.inc()
	f.active.inc()
	if f.total != nil {
		f.total.inc()
	}
	r := &fakeReader{f: f, path: p}
	r.ctx, r.cancel = context.WithCancel(context.Background())
	f.mu.Lock()
	f.opens[p]++
	f.mu.Unlock()
	if err := f.openData(ctx, r, offset); err != nil {
		_ = r.Close()
		return nil, err
	}
	if f.readerWriterTo {
		return &fakeReaderWT{r}, nil
	}
	return r, nil
}

func (f *fakeFS) openData(ctx context.Context, r *fakeReader, offset int64) error {
	if err := f.pre(ctx, "open", r.path); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	_, n := f.resolveLocked(r.path)
	if n == nil {
		return notExist("open", r.path)
	}
	if n.dir || offset > int64(len(n.data)) {
		return fmt.Errorf("fake open %s: bad target or offset", r.path)
	}
	r.data = n.data[offset:]
	return nil
}

type fakeReader struct {
	f      *fakeFS
	path   string
	data   []byte
	ctx    context.Context // canceled by Close; the engine's ctx is deliberately ignored
	cancel context.CancelFunc

	mu     sync.Mutex
	off    int
	closed bool
}

func (r *fakeReader) Read(p []byte) (int, error) {
	r.f.mu.Lock()
	r.f.maxReadBuf = max(r.f.maxReadBuf, len(p))
	chunk := r.f.chunk
	r.f.mu.Unlock()
	if err := r.f.pre(r.ctx, "read", r.path); err != nil {
		return 0, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, errFakeClosed
	}
	if r.off >= len(r.data) {
		return 0, io.EOF
	}
	n := len(p)
	if chunk > 0 {
		n = min(n, chunk)
	}
	n = copy(p[:n], r.data[r.off:])
	r.off += n
	return n, nil
}

func (r *fakeReader) Close() error {
	r.mu.Lock()
	first := !r.closed
	r.closed = true
	r.mu.Unlock()
	r.cancel()
	if first {
		r.f.readers.dec()
		r.f.active.dec()
		if r.f.total != nil {
			r.f.total.dec()
		}
	}
	return nil
}

// fakeReaderWT additionally implements io.WriterTo.
type fakeReaderWT struct{ *fakeReader }

func (r *fakeReaderWT) WriteTo(w io.Writer) (int64, error) {
	r.f.mu.Lock()
	r.f.viaWriteTo++
	r.f.mu.Unlock()
	return pipe(w, r.fakeReader)
}

func pipe(w io.Writer, r io.Reader) (int64, error) {
	buf := make([]byte, 32*1024)
	var total int64
	for {
		n, err := r.Read(buf)
		if n > 0 {
			m, werr := w.Write(buf[:n])
			total += int64(m)
			if werr != nil {
				return total, werr
			}
		}
		if errors.Is(err, io.EOF) {
			return total, nil
		}
		if err != nil {
			return total, err
		}
	}
}

func (f *fakeFS) Create(ctx context.Context, p string, opts vfs.CreateOpts) (vfs.Writer, error) {
	p = path.Clean(p)
	f.writers.inc()
	f.active.inc()
	w := &fakeWriter{f: f, path: p, opts: opts}
	w.ctx, w.cancel = context.WithCancel(context.Background())
	if err := f.pre(ctx, "create", p); err != nil {
		w.settle("create failed")
		return nil, err
	}
	f.mu.Lock()
	parent, ok := f.nodes[path.Dir(p)]
	if !ok || !parent.dir {
		f.mu.Unlock()
		w.settle("create failed")
		return nil, notExist("create", path.Dir(p))
	}
	f.creates[p]++
	rf := f.writerReaderFrom
	f.mu.Unlock()
	if rf {
		return &fakeWriterRF{w}, nil
	}
	return w, nil
}

type fakeWriter struct {
	f      *fakeFS
	path   string
	opts   vfs.CreateOpts
	ctx    context.Context // canceled by Abort so a blocked Write returns
	cancel context.CancelFunc

	mu      sync.Mutex
	buf     []byte
	settled bool
}

func (w *fakeWriter) Write(p []byte) (int, error) {
	if err := w.f.pre(w.ctx, "write", w.path); err != nil {
		return 0, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.settled {
		return 0, errFakeClosed
	}
	w.buf = append(w.buf, p...)
	return len(p), nil
}

func (w *fakeWriter) Written() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return int64(len(w.buf))
}

// settle ends the writer; a second settle is recorded as misuse.
func (w *fakeWriter) settle(why string) bool {
	w.mu.Lock()
	if w.settled {
		w.mu.Unlock()
		w.f.mu.Lock()
		w.f.misuse = append(w.f.misuse, why+" after writer settled: "+w.path)
		w.f.mu.Unlock()
		return false
	}
	w.settled = true
	w.mu.Unlock()
	w.cancel()
	w.f.writers.dec()
	w.f.active.dec()
	return true
}

func (w *fakeWriter) Abort() error {
	if w.settle("Abort") {
		w.f.mu.Lock()
		w.f.aborts++
		w.f.mu.Unlock()
	}
	return nil
}

func (w *fakeWriter) Commit() error {
	w.mu.Lock()
	data := slices.Clone(w.buf)
	w.mu.Unlock()
	if !w.settle("Commit") {
		return errors.New("fake: commit after settle")
	}
	if err := w.f.pre(context.Background(), "commit", w.path); err != nil {
		return err
	}
	f := w.f
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.lenientCommit && int64(len(data)) != w.opts.Size {
		return fmt.Errorf("fake commit %s: %w", w.path, vfs.ErrSizeMismatch)
	}
	if parent, ok := f.nodes[path.Dir(w.path)]; !ok || !parent.dir {
		return notExist("commit", path.Dir(w.path))
	}
	if old, ok := f.nodes[w.path]; ok && old.dir {
		return fmt.Errorf("fake commit %s: %w", w.path, vfs.ErrExist)
	}
	mode, mtime := w.opts.Mode, w.opts.ModTime
	if mode == 0 {
		mode = 0o644
	}
	if mtime.IsZero() {
		mtime = time.Now()
	}
	f.nodes[w.path] = &fnode{data: data, mode: mode, mtime: mtime}
	f.commits++
	return nil
}

// fakeWriterRF additionally implements io.ReaderFrom.
type fakeWriterRF struct{ *fakeWriter }

func (w *fakeWriterRF) ReadFrom(r io.Reader) (int64, error) {
	w.f.mu.Lock()
	w.f.viaReadFrm++
	w.f.mu.Unlock()
	return pipe(w.fakeWriter, r)
}

func (f *fakeFS) Mkdir(ctx context.Context, p string) error {
	p = path.Clean(p)
	if err := f.pre(ctx, "mkdir", p); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.mkdirLocked(p)
}

func (f *fakeFS) Remove(ctx context.Context, p string) error {
	p = path.Clean(p)
	if err := f.pre(ctx, "remove", p); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.nodes[p]
	if !ok {
		return notExist("remove", p)
	}
	if n.dir {
		prefix := p + "/"
		for q := range f.nodes {
			if strings.HasPrefix(q, prefix) {
				return fmt.Errorf("fake remove %s: directory not empty", p)
			}
		}
	}
	delete(f.nodes, p)
	f.removed = append(f.removed, p)
	return nil
}

func (f *fakeFS) Rename(ctx context.Context, from, to string) error {
	if err := f.pre(ctx, "rename", from); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.nodes[from]
	if !ok {
		return notExist("rename", from)
	}
	if _, exists := f.nodes[to]; exists {
		return fmt.Errorf("fake rename %s: %w", to, vfs.ErrExist)
	}
	f.nodes[to] = n
	delete(f.nodes, from)
	return nil
}
