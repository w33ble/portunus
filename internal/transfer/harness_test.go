package transfer

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/w33ble/portunus/internal/prompt"
	"github.com/w33ble/portunus/internal/vfs"
)

const waitTimeout = 15 * time.Second

// scriptAsker is a ConflictAsker whose answers the test controls.
type scriptAsker struct {
	mu          sync.Mutex
	fn          func(ctx context.Context, req prompt.ConflictRequest) (prompt.ConflictAnswer, error)
	calls       []prompt.ConflictRequest
	inflight    int
	maxInflight int
}

func (a *scriptAsker) AskConflict(ctx context.Context, req prompt.ConflictRequest) (prompt.ConflictAnswer, error) {
	a.mu.Lock()
	a.calls = append(a.calls, req)
	a.inflight++
	a.maxInflight = max(a.maxInflight, a.inflight)
	fn := a.fn
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.inflight--
		a.mu.Unlock()
	}()
	if fn == nil {
		return prompt.ConflictAnswer{Action: prompt.ConflictOverwrite}, nil
	}
	return fn(ctx, req)
}

func (a *scriptAsker) set(fn func(ctx context.Context, req prompt.ConflictRequest) (prompt.ConflictAnswer, error)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.fn = fn
}

func (a *scriptAsker) callCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.calls)
}

func (a *scriptAsker) maxConcurrent() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.maxInflight
}

// harness wires an Engine to three fake connections ("a", "b", "c").
type harness struct {
	t     *testing.T
	e     *Engine
	fs    map[string]*fakeFS
	asker *scriptAsker

	resolves atomic.Int64
	down     atomic.Bool // makes every Resolve fail, like a dropped connection
	emitting atomic.Bool
	mu       sync.Mutex
	updates  []Update
}

func newHarness(t *testing.T, mutate ...func(*Options)) *harness {
	t.Helper()
	h := &harness{t: t, fs: map[string]*fakeFS{}, asker: &scriptAsker{}}
	for _, id := range []string{"a", "b", "c"} {
		h.fs[id] = newFake(id)
	}
	opts := Options{
		Resolve: func(id string) (vfs.FS, error) {
			h.resolves.Add(1)
			if h.down.Load() {
				return nil, errors.New("connection closed")
			}
			f, ok := h.fs[id]
			if !ok {
				return nil, fmt.Errorf("no connection %q", id)
			}
			return f, nil
		},
		Conflicts: h.asker,
		Emit:      h.record,
		Limits:    Limits{Global: 4, PerConn: 4},
		Tick:      5 * time.Millisecond,
	}
	for _, m := range mutate {
		m(&opts)
	}
	h.e = New(opts)
	t.Cleanup(h.e.Close)
	return h
}

func withLimits(global, perConn int) func(*Options) {
	return func(o *Options) { o.Limits = Limits{Global: global, PerConn: perConn} }
}

func withTick(d time.Duration) func(*Options) { return func(o *Options) { o.Tick = d } }

// record is Emit; it fails the test if the engine ever calls it concurrently.
func (h *harness) record(u Update) {
	if !h.emitting.CompareAndSwap(false, true) {
		h.t.Error("Emit called concurrently")
	}
	time.Sleep(50 * time.Microsecond) // widen the window a concurrent call would hit
	h.mu.Lock()
	h.updates = append(h.updates, u)
	h.mu.Unlock()
	h.emitting.Store(false)
}

// sawJobState reports whether any emitted update showed the job in state s.
func (h *harness) sawJobState(id string, s State) bool {
	for _, u := range h.allUpdates() {
		for _, v := range u.Jobs {
			if v.ID == id && v.State == s {
				return true
			}
		}
	}
	return false
}

func (h *harness) allUpdates() []Update {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.updates)
}

func (h *harness) eventually(what string, cond func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for !cond() {
		if time.Now().After(deadline) {
			h.t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func (h *harness) find(id string) (JobView, bool) {
	for _, v := range h.e.Snapshot() {
		if v.ID == id {
			return v, true
		}
	}
	return JobView{}, false
}

func (h *harness) job(id string) JobView {
	h.t.Helper()
	v, ok := h.find(id)
	if !ok {
		h.t.Fatalf("job %s not found", id)
	}
	return v
}

func (h *harness) waitState(id string, want State) JobView {
	h.t.Helper()
	h.eventually(fmt.Sprintf("job %s to be %s", id, want), func() bool {
		v, ok := h.find(id)
		return ok && v.State == want
	})
	return h.job(id)
}

func (h *harness) waitFinal(id string) JobView {
	h.t.Helper()
	h.eventually(fmt.Sprintf("job %s to finish", id), func() bool {
		v, ok := h.find(id)
		return ok && isFinal(v.State)
	})
	return h.job(id)
}

func (h *harness) copyJob(srcConn string, sources []string, dstConn, dstDir string, p Policy) string {
	h.t.Helper()
	id, err := h.e.Copy(CopyRequest{SrcConn: srcConn, Sources: sources, DstConn: dstConn, DstDir: dstDir, Policy: p})
	if err != nil {
		h.t.Fatalf("Copy: %v", err)
	}
	return id
}

func (h *harness) deleteJob(conn string, paths ...string) string {
	h.t.Helper()
	id, err := h.e.Delete(DeleteRequest{Conn: conn, Paths: paths})
	if err != nil {
		h.t.Fatalf("Delete: %v", err)
	}
	return id
}

// assertQuiescent checks no reader or writer is left open and no writer was settled twice.
func (h *harness) assertQuiescent() {
	h.t.Helper()
	for id, f := range h.fs {
		if n := f.readers.cur.Load(); n != 0 {
			h.t.Errorf("conn %s: %d readers still open", id, n)
		}
		if n := f.writers.cur.Load(); n != 0 {
			h.t.Errorf("conn %s: %d writers still unsettled", id, n)
		}
		if _, _, misuse := f.counters(); len(misuse) > 0 {
			h.t.Errorf("conn %s: writer misuse: %v", id, misuse)
		}
	}
}

// blocker stalls matching operations until released, counting how many arrived.
type blocker struct {
	op      string
	arrived atomic.Int64
	tokens  chan struct{}
}

func newBlocker(op string) *blocker { return &blocker{op: op, tokens: make(chan struct{}, 4096)} }

func (b *blocker) hook(ctx context.Context, op, _ string) error {
	if op != b.op {
		return nil
	}
	b.arrived.Add(1)
	select {
	case <-b.tokens:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *blocker) release(n int) {
	for range n {
		b.tokens <- struct{}{}
	}
}

// releaseAll lets every current and future arrival through.
func (b *blocker) releaseAll() { b.release(cap(b.tokens)) }

func (b *blocker) waitArrived(h *harness, n int64) {
	h.t.Helper()
	h.eventually(fmt.Sprintf("%d %s arrivals (have %d)", n, b.op, b.arrived.Load()), func() bool { return b.arrived.Load() >= n })
}

// stays asserts the arrival count does not grow past n for a short while.
func (b *blocker) stays(t *testing.T, n int64) {
	t.Helper()
	time.Sleep(40 * time.Millisecond)
	if got := b.arrived.Load(); got != n {
		t.Fatalf("%s arrivals = %d, want exactly %d", b.op, got, n)
	}
}

func numbered(prefix string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s%03d", prefix, i)
	}
	return out
}

func addFiles(f *fakeFS, dir string, names []string) {
	for _, n := range names {
		f.addFile(dir+"/"+n, "content of "+n)
	}
}

func joinPaths(dir string, names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = dir + "/" + n
	}
	return out
}
