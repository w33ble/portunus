package transfer

import (
	"context"
	"fmt"

	"github.com/w33ble/portunus/internal/vfs"
)

// dispatchLocked starts queued tasks, oldest job first, while the global limit and each
// job's per-connection limits allow. A task takes its permits (global plus one per
// distinct connection, visited in connection-ID order) all at once or not at all, so no
// task ever holds one permit while waiting for another and remote-to-remote jobs in
// opposite directions cannot deadlock. Jobs that are out of permits are skipped, so one
// saturated connection never blocks tasks bound for another.
func (e *Engine) dispatchLocked() {
	if e.closed {
		return
	}
	for _, j := range e.live {
		if e.running >= e.limits.Global {
			return
		}
		if j.ctx.Err() != nil {
			continue
		}
		for e.running < e.limits.Global && e.permitsFreeLocked(j) {
			t := j.popReadyLocked()
			if t == nil {
				break
			}
			e.startLocked(j, t)
		}
	}
}

func (e *Engine) permitsFreeLocked(j *job) bool {
	for _, c := range j.conns {
		if e.conns[c] >= e.limits.PerConn {
			return false
		}
	}
	return true
}

func (j *job) popReadyLocked() *task {
	if n := len(j.extra); n > 0 {
		t := j.extra[n-1]
		j.extra[n-1] = nil
		j.extra = j.extra[:n-1]
		return t
	}
	select {
	case t := <-j.ready:
		return t
	default:
		return nil
	}
}

func (e *Engine) startLocked(j *job, t *task) {
	e.running++
	for _, c := range j.conns {
		e.conns[c]++
	}
	j.running++
	t.state = StateRunning
	j.active[t] = struct{}{}
	e.markStartedLocked(j)
	e.touch(j)
	e.wg.Go(func() { e.runTask(j, t) })
}

func (e *Engine) runTask(j *job, t *task) {
	err := e.execute(j, t)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.running--
	for _, c := range j.conns {
		if e.conns[c]--; e.conns[c] == 0 {
			delete(e.conns, c)
		}
	}
	j.running--
	e.settleLocked(j, t, outcomeOf(j, err), err)
	e.dispatchLocked()
}

func (e *Engine) execute(j *job, t *task) error {
	switch j.kind {
	case KindCopy:
		return e.copyFile(j, t)
	case KindDelete:
		return e.remove(j, t)
	}
	return fmt.Errorf("transfer: unknown job kind %q", j.kind)
}

// startRunLocked launches the producer (and, for ask-policy copies, the conflict gate)
// for one run of j: its first, or a retry. Tasks in requeue go back to the scheduler
// before items are walked.
func (e *Engine) startRunLocked(j *job, items []walkItem, requeue []*task) {
	j.walking = true
	j.producers++
	var conflicts chan *task
	if j.kind == KindCopy && j.policy == PolicyAsk {
		conflicts = make(chan *task, conflictQueueCap)
		j.producers++
		e.wg.Go(func() { e.runGate(j, conflicts) })
	}
	e.wg.Go(func() { e.produce(j, items, requeue, conflicts) })
}

func (e *Engine) produce(j *job, items []walkItem, requeue []*task, conflicts chan *task) {
	defer func() {
		if conflicts != nil {
			close(conflicts)
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		j.producers--
		j.walking = false
		e.touch(j)
		e.notify()
		e.finishCheckLocked(j)
	}()
	for _, t := range requeue {
		if !e.requeue(j, t, conflicts) {
			return
		}
	}
	switch j.kind {
	case KindCopy:
		w := &copyWalker{e: e, j: j, ctx: j.ctx, conflicts: conflicts, stack: items}
		w.run()
	case KindDelete:
		w := &deleteWalker{e: e, j: j, ctx: j.ctx, stack: items}
		w.run()
	}
}

// requeue hands a failed task from an earlier run back to the gate or the scheduler.
func (e *Engine) requeue(j *job, t *task, conflicts chan *task) bool {
	if t.needsAsk() && conflicts != nil {
		e.markWaiting(j, t)
		return send(j.ctx, conflicts, t)
	}
	return e.enqueueReady(j, t)
}

// enqueueReady blocks while the job's queue is full, which is what keeps a walk lazy.
// It reports false when the job was canceled first.
func (e *Engine) enqueueReady(j *job, t *task) bool {
	if !send(j.ctx, j.ready, t) {
		return false
	}
	e.mu.Lock()
	e.dispatchLocked()
	e.mu.Unlock()
	return true
}

func send(ctx context.Context, ch chan<- *task, t *task) bool {
	select {
	case ch <- t:
		return true
	case <-ctx.Done():
		return false
	}
}

func (e *Engine) markWaiting(j *job, t *task) {
	e.mu.Lock()
	defer e.mu.Unlock()
	t.state = StateWaiting
	j.active[t] = struct{}{}
	e.touch(j)
}

// beginInline marks a task the walker executes itself (copy directories) as running.
func (e *Engine) beginInline(j *job, t *task) {
	e.mu.Lock()
	defer e.mu.Unlock()
	t.state = StateRunning
	j.active[t] = struct{}{}
	e.markStartedLocked(j)
	e.touch(j)
}

// endInline settles a task the walker executed itself.
func (e *Engine) endInline(j *job, t *task, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	t.expanded = err == nil
	e.settleLocked(j, t, outcomeOf(j, err), err)
}

// firstSourceKnown records whether the first source is a directory, for the title.
func (e *Engine) firstSourceKnown(j *job, src string, ent vfs.Entry) {
	if src != j.sources[0] {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	j.firstIsDir = ent.IsDir
	e.touch(j)
}
