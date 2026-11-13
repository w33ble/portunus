package transfer

import (
	"context"
	"slices"
	"time"
)

// Retry re-enqueues the failed tasks of a job that finished in state failed. Connections
// are resolved again, so a reconnect between the failure and the retry is picked up.
// Failures that cannot be retried (hostile entry names, a file where a directory belongs)
// stay in the failed list.
func (e *Engine) Retry(jobID string) error {
	e.mu.Lock()
	j, err := e.retryableLocked(jobID)
	if err != nil {
		e.mu.Unlock()
		return err
	}
	srcConn, dstConn := j.srcConn, j.dstConn
	e.mu.Unlock()

	src, dst, err := e.resolvePair(srcConn, dstConn)
	if err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if j, err = e.retryableLocked(jobID); err != nil {
		return err
	}
	j.src, j.dst = src, dst
	j.ctx, j.cancel = context.WithCancel(e.ctx)
	items, requeue := e.prepareRetryLocked(j)
	j.started = false
	j.finishedAt = time.Time{}
	e.setStateLocked(j, StateQueued)
	e.live = append(e.live, j)
	e.startRunLocked(j, items, requeue)
	e.dispatchLocked()
	return nil
}

func (e *Engine) retryableLocked(jobID string) (*job, error) {
	j, ok := e.jobs[jobID]
	switch {
	case !ok:
		return nil, ErrUnknownJob
	case e.closed:
		return nil, ErrClosed
	case j.state != StateFailed || !slices.ContainsFunc(j.failed, func(t *task) bool { return !t.noRetry }):
		return nil, ErrNotRetryable
	}
	return j, nil
}

// prepareRetryLocked takes the retryable failed tasks out of the failed list and sorts them
// into what the producer must walk again and what goes straight back to the scheduler.
// Delete directories get their dependencies rebuilt: a directory waits for the failed
// children being retried, and for its own listing if that was what failed.
func (e *Engine) prepareRetryLocked(j *job) (items []walkItem, requeue []*task) {
	var retry []*task
	j.failed = slices.DeleteFunc(j.failed, func(t *task) bool {
		if t.noRetry {
			return false
		}
		retry = append(retry, t)
		return true
	})
	j.errMsg = ""
	inSet := make(map[*task]struct{}, len(retry))
	for _, t := range retry {
		inSet[t] = struct{}{}
	}

	var roots []string
	for _, t := range retry {
		t.err, t.state, t.remaining, t.enqueued, t.childFailed, t.childFinal = "", StateQueued, 0, false, false, false
		if t.kind == taskSource {
			j.totalFiles-- // the real tasks are counted again when the source is walked
			roots = append(roots, t.src)
			continue
		}
		j.pending++
	}
	if len(roots) > 0 {
		items = append(items, walkItem{root: true, sources: roots})
	}
	for _, t := range retry {
		if _, ok := inSet[t.parent]; ok && t.kind != taskSource {
			t.parent.remaining++
		}
	}
	for _, t := range retry {
		switch {
		case t.kind == taskSource:
		case t.kind == taskDir && !t.expanded:
			t.remaining++ // listing token, released by deleteWalker.dir
			items = append(items, walkItem{task: t, listDst: true})
		case t.kind == taskDir:
			if t.remaining == 0 {
				t.enqueued = true
				j.extra = append(j.extra, t)
			}
		default:
			requeue = append(requeue, t)
		}
	}
	return items, requeue
}
