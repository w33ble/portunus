// Package transfer runs copy and delete jobs between vfs filesystems.
//
// A job is one user action. Directories expand lazily on a producer goroutine into
// tasks (one per file or directory); a scheduler starts tasks under a global limit plus
// a per-connection limit; an aggregator batches progress into Updates.
package transfer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/w33ble/portunus/internal/prompt"
	"github.com/w33ble/portunus/internal/vfs"
)

const (
	defaultTick      = 250 * time.Millisecond
	readyQueueCap    = 64 // bounds how far a walk runs ahead of the workers
	conflictQueueCap = 32 // bounds tasks parked on the conflict gate
	maxFailedViews   = 200
	copyBufSize      = 1 << 20
	// writeBatchSize is how much WriteTo output is gathered before one Write: pkg/sftp only
	// pipelines writes larger than a packet, and a deeper batch keeps its queue full.
	writeBatchSize = 4 << 20
)

var (
	ErrClosed       = errors.New("transfer: engine closed")
	ErrUnknownJob   = errors.New("transfer: unknown job")
	ErrNotRetryable = errors.New("transfer: job has no failed tasks to retry")
	ErrNotResumable = errors.New("transfer: only canceled jobs can be resumed")
	ErrNoSources    = errors.New("transfer: no sources")
	ErrIntoSelf     = errors.New("transfer: cannot copy a directory into itself")
	ErrOntoSelf     = errors.New("transfer: cannot copy an item onto itself")
	// ErrInvalid wraps requests with empty or root paths, or an unknown policy.
	ErrInvalid = errors.New("transfer: invalid request")
)

// Resolver returns the live filesystem for a connection ID.
type Resolver func(connID string) (vfs.FS, error)

// ConflictAsker asks the user what to do about an existing destination file.
type ConflictAsker interface {
	AskConflict(ctx context.Context, req prompt.ConflictRequest) (prompt.ConflictAnswer, error)
}

// Limits bounds concurrent tasks. Values <= 0 mean 1.
type Limits struct {
	Global  int `json:"global"`
	PerConn int `json:"perConn"`
}

func (l Limits) normalized() Limits {
	l.Global = max(l.Global, 1)
	l.PerConn = max(l.PerConn, 1)
	return l
}

// Policy decides what happens when a destination file already exists.
type Policy string

const (
	PolicyAsk       Policy = "ask"
	PolicyOverwrite Policy = "overwrite"
	PolicySkip      Policy = "skip"
)

// Options configures an Engine.
type Options struct {
	Resolve   Resolver
	Conflicts ConflictAsker
	Emit      func(Update) // called from one goroutine, never concurrently; must not call Close
	Limits    Limits
	Tick      time.Duration // progress batch interval, default 250ms
	Logger    *slog.Logger
}

// CopyRequest copies Sources (absolute paths on SrcConn) into DstDir on DstConn.
type CopyRequest struct {
	SrcConn string   `json:"srcConn"`
	Sources []string `json:"sources"`
	DstConn string   `json:"dstConn"`
	DstDir  string   `json:"dstDir"`
	Policy  Policy   `json:"policy"` // empty means ask
}

// DeleteRequest recursively deletes Paths on Conn. Symlinks are removed, never followed.
type DeleteRequest struct {
	Conn  string   `json:"conn"`
	Paths []string `json:"paths"`
}

// JobKind is "copy" or "delete".
type JobKind string

const (
	KindCopy   JobKind = "copy"
	KindDelete JobKind = "delete"
)

// State is a job or task state.
type State string

const (
	StateQueued   State = "queued"
	StateRunning  State = "running"
	StateWaiting  State = "waiting" // parked on the conflict gate
	StateDone     State = "done"
	StateFailed   State = "failed"
	StateCanceled State = "canceled"
	StateSkipped  State = "skipped"
)

// JobView is a point-in-time copy of a job. File counters count tasks, so they include
// directories and skipped links.
type JobView struct {
	ID      string  `json:"id"`
	Kind    JobKind `json:"kind"`
	State   State   `json:"state"`
	SrcConn string  `json:"srcConn"`
	DstConn string  `json:"dstConn"` // empty for delete
	DstDir  string  `json:"dstDir"`  // empty for delete
	Title   string  `json:"title"`
	// Walking is true while directories are still expanding; totals are provisional.
	Walking      bool       `json:"walking"`
	TotalFiles   int        `json:"totalFiles"`
	DoneFiles    int        `json:"doneFiles"`
	FailedFiles  int        `json:"failedFiles"`
	SkippedFiles int        `json:"skippedFiles"`
	TotalBytes   int64      `json:"totalBytes"`
	DoneBytes    int64      `json:"doneBytes"`
	BytesPerSec  float64    `json:"bytesPerSec"`
	ETASeconds   int64      `json:"etaSeconds"` // -1 unknown; 0 once finished
	Active       []TaskView `json:"active"`     // running and waiting tasks
	Failed       []TaskView `json:"failed"`     // first 200 failures; FailedFiles has the count
	Error        string     `json:"error,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	FinishedAt   *time.Time `json:"finishedAt,omitempty"`
}

// TaskView is a point-in-time copy of one task.
type TaskView struct {
	ID    string `json:"id"`
	Src   string `json:"src"`
	Dst   string `json:"dst"`
	Size  int64  `json:"size"`
	Done  int64  `json:"done"`
	State State  `json:"state"`
	Error string `json:"error,omitempty"`
}

// Update is one batch of changed jobs plus IDs dropped by Clear.
type Update struct {
	Jobs    []JobView `json:"jobs"`
	Removed []string  `json:"removed"`
}

// Engine schedules jobs. Safe for concurrent use.
type Engine struct {
	resolve Resolver
	asker   ConflictAsker
	emit    func(Update)
	tick    time.Duration
	log     *slog.Logger
	bufs    sync.Pool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	kick      chan struct{} // urgent flush request
	stop      chan struct{}
	aggDone   chan struct{}
	closeOnce sync.Once

	mu      sync.Mutex // guards everything below and all job/task fields except atomics
	closed  bool
	limits  Limits
	running int
	conns   map[string]int
	jobs    map[string]*job
	order   []*job // creation order, includes finished jobs
	live    []*job // unfinished jobs in creation order; the scheduler's work list
	removed []string
	seq     int
}

// New starts an Engine and its progress aggregator.
func New(o Options) *Engine {
	log := o.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	tick := o.Tick
	if tick <= 0 {
		tick = defaultTick
	}
	ctx, cancel := context.WithCancel(context.Background())
	e := &Engine{
		resolve: o.Resolve,
		asker:   o.Conflicts,
		emit:    o.Emit,
		tick:    tick,
		log:     log,
		bufs: sync.Pool{New: func() any {
			b := make([]byte, copyBufSize)
			return &b
		}},
		ctx:     ctx,
		cancel:  cancel,
		kick:    make(chan struct{}, 1),
		stop:    make(chan struct{}),
		aggDone: make(chan struct{}),
		limits:  o.Limits.normalized(),
		conns:   map[string]int{},
		jobs:    map[string]*job{},
	}
	go e.aggregate()
	return e
}

// Copy validates req and starts a copy job, returning its ID.
func (e *Engine) Copy(req CopyRequest) (string, error) {
	req, err := req.normalize()
	if err != nil {
		return "", err
	}
	src, dst, err := e.resolvePair(req.SrcConn, req.DstConn)
	if err != nil {
		return "", err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return "", ErrClosed
	}
	j := e.newJobLocked(KindCopy, req.SrcConn, req.DstConn, req.DstDir, req.Policy, req.Sources)
	j.src, j.dst = src, dst
	e.startRunLocked(j, []walkItem{{root: true, sources: req.Sources}}, nil)
	return j.id, nil
}

// Delete validates req and starts a recursive delete job, returning its ID.
func (e *Engine) Delete(req DeleteRequest) (string, error) {
	paths, err := cleanPaths(req.Paths)
	if err != nil {
		return "", err
	}
	src, _, err := e.resolvePair(req.Conn, "")
	if err != nil {
		return "", err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return "", ErrClosed
	}
	j := e.newJobLocked(KindDelete, req.Conn, "", "", "", paths)
	j.src = src
	e.startRunLocked(j, []walkItem{{root: true, sources: paths}}, nil)
	return j.id, nil
}

// Cancel stops a job; it reports canceled once running tasks have aborted. Canceling a
// finished job is a no-op.
func (e *Engine) Cancel(jobID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	j, ok := e.jobs[jobID]
	if !ok {
		return ErrUnknownJob
	}
	if j.final() {
		return nil
	}
	j.cancel()
	e.finishCheckLocked(j)
	return nil
}

// Clear drops finished jobs and reports their IDs in the next Update.
func (e *Engine) Clear() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.order = slices.DeleteFunc(e.order, func(j *job) bool {
		if !j.final() {
			return false
		}
		delete(e.jobs, j.id)
		e.removed = append(e.removed, j.id)
		return true
	})
	if len(e.removed) > 0 {
		e.notify()
	}
}

// Snapshot returns every job in creation order.
func (e *Engine) Snapshot() []JobView {
	e.mu.Lock()
	defer e.mu.Unlock()
	views := make([]JobView, 0, len(e.order))
	for _, j := range e.order {
		views = append(views, j.view())
	}
	return views
}

// SetLimits changes the limits. Lowering below the running count lets running tasks
// finish; nothing new starts until the count drops under the limit.
func (e *Engine) SetLimits(l Limits) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.limits = l.normalized()
	e.dispatchLocked()
}

// Close cancels every job, waits for all workers, flushes a final Update, and stops the
// aggregator. The engine rejects new work afterwards.
func (e *Engine) Close() {
	e.closeOnce.Do(func() {
		e.mu.Lock()
		e.closed = true
		for _, j := range e.live {
			j.cancel()
		}
		for _, j := range slices.Clone(e.live) {
			e.finishCheckLocked(j)
		}
		e.mu.Unlock()
		e.cancel()
		e.wg.Wait()
		close(e.stop)
		<-e.aggDone
	})
}

func (e *Engine) resolvePair(srcConn, dstConn string) (src, dst vfs.FS, err error) {
	if e.resolve == nil {
		return nil, nil, errors.New("transfer: no resolver configured")
	}
	if src, err = e.resolveOne(srcConn); err != nil {
		return nil, nil, err
	}
	switch dstConn {
	case "":
		return src, nil, nil
	case srcConn:
		return src, src, nil
	}
	if dst, err = e.resolveOne(dstConn); err != nil {
		return nil, nil, err
	}
	return src, dst, nil
}

func (e *Engine) resolveOne(conn string) (vfs.FS, error) {
	fsys, err := e.resolve(conn)
	if err != nil {
		return nil, fmt.Errorf("resolve connection %q: %w", conn, err)
	}
	if fsys == nil {
		return nil, fmt.Errorf("resolve connection %q: no filesystem", conn)
	}
	return fsys, nil
}

func (r CopyRequest) normalize() (CopyRequest, error) {
	if len(r.Sources) == 0 {
		return r, ErrNoSources
	}
	switch r.Policy {
	case "":
		r.Policy = PolicyAsk
	case PolicyAsk, PolicyOverwrite, PolicySkip:
	default:
		return r, fmt.Errorf("%w: policy %q", ErrInvalid, r.Policy)
	}
	if r.DstDir == "" {
		return r, fmt.Errorf("%w: empty destination", ErrInvalid)
	}
	if !isCanonical(r.DstDir) {
		return r, fmt.Errorf("%w: destination %q is not a clean path", ErrInvalid, r.DstDir)
	}
	srcs, err := cleanPaths(r.Sources)
	if err != nil {
		return r, err
	}
	if r.SrcConn == r.DstConn {
		for _, s := range srcs {
			if path.Dir(s) == r.DstDir {
				return r, fmt.Errorf("%w: %s into %s", ErrOntoSelf, s, r.DstDir)
			}
			if within(r.DstDir, s) {
				return r, fmt.Errorf("%w: %s into %s", ErrIntoSelf, s, r.DstDir)
			}
		}
	}
	r.Sources = srcs
	return r, nil
}

// isCanonical reports whether p is already in path.Clean form. Paths that differ (empty,
// ".", ".." or doubled-slash segments, trailing slash) could name a different item than the
// one the caller meant, so they are refused rather than silently rewritten.
func isCanonical(p string) bool { return p != "" && p == path.Clean(p) }

// CheckPath accepts p only if it is a canonical, non-root path: the rule every source path follows.
func CheckPath(p string) error {
	switch {
	case p == "":
		return fmt.Errorf("%w: empty path", ErrInvalid)
	case !isCanonical(p):
		return fmt.Errorf("%w: %q is not a clean path", ErrInvalid, p)
	case path.Dir(p) == p:
		return fmt.Errorf("%w: %q is a root", ErrInvalid, p)
	default:
		return nil
	}
}

// cleanPaths returns ps if every entry passes CheckPath.
func cleanPaths(ps []string) ([]string, error) {
	if len(ps) == 0 {
		return nil, ErrNoSources
	}
	for _, p := range ps {
		if err := CheckPath(p); err != nil {
			return nil, err
		}
	}
	return slices.Clone(ps), nil
}

// within reports whether p is dir or lies beneath it.
func within(p, dir string) bool {
	return p == dir || strings.HasPrefix(p, strings.TrimSuffix(dir, "/")+"/")
}

// validName rejects entry names a hostile server could use to escape the destination.
func validName(n string) bool {
	return n != "" && n != "." && n != ".." && !strings.ContainsAny(n, "/\x00")
}
