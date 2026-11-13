package transfer

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/w33ble/portunus/internal/prompt"
	"github.com/w33ble/portunus/internal/vfs"
)

var errContentsRemain = errors.New("not removed: some contents could not be deleted")

type taskKind int

const (
	taskFile   taskKind = iota // copy a file, or delete a file or link
	taskDir                    // create a directory and expand it, or remove it after its contents
	taskSource                 // a top-level source that could not be examined
)

type task struct {
	id        string
	seq       int
	kind      taskKind
	src, dst  string
	ent       vfs.Entry // source entry (copy)
	size      int64
	dstEnt    *vfs.Entry // existing destination file that conflicts
	overwrite bool       // conflict resolved in favor of overwriting
	noRetry   bool       // failed for a reason Retry cannot change
	state     State
	err       string
	done      atomic.Int64

	// Delete jobs remove a directory only after its contents.
	parent      *task
	remaining   int  // unfinished children plus one token until the directory is fully listed
	childFailed bool // a child failed, so the directory cannot be removed
	childFinal  bool // ...and one of those failures cannot be retried
	enqueued    bool
	expanded    bool // contents fully enumerated
}

func (t *task) needsAsk() bool { return t.dstEnt != nil && !t.overwrite }

func (t *task) view() TaskView {
	return TaskView{ID: t.id, Src: t.src, Dst: t.dst, Size: t.size, Done: t.done.Load(), State: t.state, Error: t.err}
}

// finishedTask is what a resumed job needs to count a settled path without redoing it.
type finishedTask struct {
	state State // StateDone or StateSkipped
	size  int64
}

type job struct {
	id                    string
	kind                  JobKind
	policy                Policy
	srcConn, dstConn      string
	dstDir                string
	sources               []string
	conns                 []string // distinct connection IDs this job's tasks hold permits on, sorted
	firstIsDir            bool
	src, dst              vfs.FS
	ctx                   context.Context
	cancel                context.CancelFunc
	ready                 chan *task // bounded; fed by producers, drained by the scheduler
	extra                 []*task    // directory deletes whose contents are gone
	state                 State
	started               bool
	createdAt, finishedAt time.Time
	totalFiles, doneFiles int
	skippedFiles          int
	totalBytes            int64
	doneBytes             atomic.Int64
	walking               bool
	producers, running    int // goroutines that can still add work / tasks executing
	pending               int // tasks neither finished nor discarded
	active                map[*task]struct{}
	failed                []*task
	finished              map[string]finishedTask // paths settled done or skipped, kept while the job is resumable
	errMsg                string
	applyAll              prompt.ConflictAction
	taskSeq               int
	meter                 meter
	dirty                 atomic.Bool
}

func (j *job) final() bool { return isFinal(j.state) }

func isFinal(s State) bool {
	switch s {
	case StateDone, StateFailed, StateCanceled, StateSkipped:
		return true
	case StateQueued, StateRunning, StateWaiting:
		return false
	}
	return false
}

func (j *job) title() string {
	base := path.Base(j.sources[0])
	if j.firstIsDir {
		base += "/"
	}
	if n := len(j.sources) - 1; n > 0 {
		return fmt.Sprintf("%s and %d more", base, n)
	}
	return base
}

func (j *job) view() JobView {
	v := JobView{
		ID: j.id, Kind: j.kind, State: j.state,
		SrcConn: j.srcConn, DstConn: j.dstConn, DstDir: j.dstDir,
		Title: j.title(), Walking: j.walking,
		TotalFiles: j.totalFiles, DoneFiles: j.doneFiles, FailedFiles: len(j.failed), SkippedFiles: j.skippedFiles,
		TotalBytes: j.totalBytes, DoneBytes: j.doneBytes.Load(),
		Error: j.errMsg, CreatedAt: j.createdAt,
		Active: []TaskView{}, Failed: []TaskView{},
	}
	if !j.final() {
		v.BytesPerSec = j.meter.speed
		v.ETASeconds = j.eta()
	} else {
		finished := j.finishedAt
		v.FinishedAt = &finished
	}
	tasks := make([]*task, 0, len(j.active))
	for t := range j.active {
		tasks = append(tasks, t)
	}
	slices.SortFunc(tasks, func(a, b *task) int { return cmp.Compare(a.seq, b.seq) })
	for _, t := range tasks {
		v.Active = append(v.Active, t.view())
	}
	for _, t := range j.failed[:min(len(j.failed), maxFailedViews)] {
		v.Failed = append(v.Failed, t.view())
	}
	return v
}

func (j *job) eta() int64 {
	remaining := j.totalBytes - j.doneBytes.Load()
	if j.walking || j.meter.speed <= 0 {
		return -1
	}
	if remaining <= 0 {
		return 0
	}
	return int64(float64(remaining)/j.meter.speed + 0.999)
}

// outcome is the final state of a job whose work all settled without cancellation.
func (j *job) outcome() State {
	switch {
	case len(j.failed) > 0 || j.errMsg != "":
		return StateFailed
	case j.doneFiles == 0 && j.skippedFiles > 0:
		return StateSkipped
	default:
		return StateDone
	}
}

func (e *Engine) newJobLocked(kind JobKind, srcConn, dstConn, dstDir string, policy Policy, sources []string) *job {
	e.seq++
	j := &job{
		id: "job-" + strconv.Itoa(e.seq), kind: kind, policy: policy,
		srcConn: srcConn, dstConn: dstConn, dstDir: dstDir, sources: sources,
		ready:     make(chan *task, readyQueueCap),
		state:     StateQueued,
		active:    map[*task]struct{}{},
		createdAt: time.Now(),
		conns:     []string{srcConn},
	}
	if dstConn != "" && dstConn != srcConn {
		j.conns = []string{srcConn, dstConn}
		slices.Sort(j.conns)
	}
	j.ctx, j.cancel = context.WithCancel(e.ctx)
	e.jobs[j.id] = j
	e.order = append(e.order, j)
	e.live = append(e.live, j)
	j.dirty.Store(true)
	e.notify()
	e.log.InfoContext(j.ctx, "transfer job created", "job", j.id, "kind", string(kind), "sources", len(sources))
	return j
}

// touch marks a job for the next progress batch.
func (e *Engine) touch(j *job) { j.dirty.Store(true) }

// notify asks the aggregator for an immediate flush.
func (e *Engine) notify() {
	select {
	case e.kick <- struct{}{}:
	default:
	}
}

func (e *Engine) setStateLocked(j *job, s State) {
	j.state = s
	j.dirty.Store(true)
	e.notify()
}

func (e *Engine) markStartedLocked(j *job) {
	if j.started {
		return
	}
	j.started = true
	if j.state == StateQueued {
		e.setStateLocked(j, StateRunning)
	}
}

// restoreStateLocked leaves StateWaiting once the conflict gate stops blocking on the user.
func (e *Engine) restoreStateLocked(j *job) {
	if j.state != StateWaiting {
		return
	}
	if j.started {
		e.setStateLocked(j, StateRunning)
		return
	}
	e.setStateLocked(j, StateQueued)
}

// outcomeOf classifies how a task ended: a failure during cancellation is a cancellation.
func outcomeOf(j *job, err error) State {
	switch {
	case err == nil:
		return StateDone
	case j.ctx.Err() != nil:
		return StateCanceled
	default:
		return StateFailed
	}
}

// register adds a new task to the job's counters.
func (e *Engine) register(j *job, t *task) {
	e.mu.Lock()
	defer e.mu.Unlock()
	j.taskSeq++
	t.seq = j.taskSeq
	t.id = j.id + "." + strconv.Itoa(t.seq)
	t.state = StateQueued
	j.totalFiles++
	j.totalBytes += t.size
	j.pending++
	if t.parent != nil {
		t.parent.remaining++
	}
	e.touch(j)
}

// settleLocked moves t to a final state, folds it into the job counters, and wakes
// whatever was waiting on it. Done, Skipped, Failed and Canceled are the only valid s.
func (e *Engine) settleLocked(j *job, t *task, s State, err error) {
	t.state = s
	delete(j.active, t)
	j.pending--
	switch s {
	case StateDone:
		j.doneFiles++
	case StateSkipped:
		j.skippedFiles++
		j.totalBytes -= t.size
	case StateFailed:
		t.err = err.Error()
		j.failed = append(j.failed, t)
		j.doneBytes.Add(-t.done.Swap(0))
		e.log.WarnContext(j.ctx, "transfer task failed", "job", j.id, "src", t.src, "dst", t.dst, "err", err)
	case StateCanceled:
		j.doneBytes.Add(-t.done.Swap(0))
	case StateQueued, StateRunning, StateWaiting:
	}
	j.recordFinishedLocked(t, s)
	e.touch(j)
	e.depDoneLocked(j, t)
	e.finishCheckLocked(j)
}

// settle is settleLocked for callers that do not hold the lock.
func (e *Engine) settle(j *job, t *task, s State, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.settleLocked(j, t, s, err)
}

// recordFinishedLocked remembers files (and, for deletes, directories) that are done or
// skipped, so Resume can count them without walking them again.
func (j *job) recordFinishedLocked(t *task, s State) {
	if s != StateDone && s != StateSkipped {
		return
	}
	key := t.dst
	if j.kind == KindDelete {
		key = t.src
	} else if t.kind != taskFile {
		return
	}
	if key == "" {
		return
	}
	if j.finished == nil {
		j.finished = map[string]finishedTask{}
	}
	j.finished[key] = finishedTask{state: s, size: t.size}
}

// depDoneLocked tells a delete job's parent directory that a child is finished.
func (e *Engine) depDoneLocked(j *job, t *task) {
	p := t.parent
	if p == nil {
		return
	}
	if t.state == StateFailed {
		p.childFailed = true
		p.childFinal = p.childFinal || t.noRetry
	}
	p.remaining--
	e.dirReadyLocked(j, p)
}

// dirReadyLocked queues a directory delete once its contents are gone, or fails it when
// some contents could not be deleted.
func (e *Engine) dirReadyLocked(j *job, p *task) {
	if p.remaining > 0 || p.enqueued || p.state != StateQueued || j.ctx.Err() != nil {
		return
	}
	if p.childFailed {
		p.noRetry = p.childFinal
		e.settleLocked(j, p, StateFailed, errContentsRemain)
		return
	}
	p.enqueued = true
	j.extra = append(j.extra, p)
}

// finishCheckLocked finalizes the job when nothing can add or run work any more.
func (e *Engine) finishCheckLocked(j *job) {
	if j.final() {
		return
	}
	if j.ctx.Err() != nil {
		if j.producers == 0 && j.running == 0 {
			e.finalizeLocked(j, StateCanceled)
		}
		return
	}
	if j.producers == 0 && j.pending == 0 {
		e.finalizeLocked(j, j.outcome())
	}
}

func (e *Engine) finalizeLocked(j *job, s State) {
	for t := range j.active {
		t.state = StateCanceled
		j.doneBytes.Add(-t.done.Swap(0))
	}
	clear(j.active)
	j.walking = false
	if s != StateCanceled {
		j.finished = nil
	}
	j.finishedAt = time.Now()
	j.meter = meter{}
	e.setStateLocked(j, s)
	e.live = slices.DeleteFunc(e.live, func(o *job) bool { return o == j })
	j.cancel()
	e.log.InfoContext(e.ctx, "transfer job finished", "job", j.id, "state", string(s),
		"done", j.doneFiles, "failed", len(j.failed), "skipped", j.skippedFiles)
	e.dispatchLocked()
}
