package transfer

import (
	"context"
	"time"
)

// Resume restarts a canceled job against its original sources and destination. The sources
// are walked again, because cancellation may have interrupted the walk. Paths that finished
// before the cancel are counted but not copied or deleted again; files that were in flight
// start over from byte 0. The job's conflict policy applies to anything new at the
// destination. Connections are resolved again, so a reconnect in between is picked up.
func (e *Engine) Resume(jobID string) error {
	e.mu.Lock()
	j, err := e.resumableLocked(jobID)
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
	if j, err = e.resumableLocked(jobID); err != nil {
		return err
	}
	j.src, j.dst = src, dst
	j.ctx, j.cancel = context.WithCancel(e.ctx)
	j.ready = make(chan *task, readyQueueCap)
	j.extra, j.failed, j.errMsg, j.pending = nil, nil, "", 0
	j.started = false
	j.finishedAt = time.Time{}
	j.seedFinishedLocked()
	e.setStateLocked(j, StateQueued)
	e.live = append(e.live, j)
	e.startRunLocked(j, []walkItem{{root: true, sources: j.sources}}, nil)
	e.dispatchLocked()
	return nil
}

func (e *Engine) resumableLocked(jobID string) (*job, error) {
	j, ok := e.jobs[jobID]
	switch {
	case !ok:
		return nil, ErrUnknownJob
	case e.closed:
		return nil, ErrClosed
	case j.state != StateCanceled:
		return nil, ErrNotResumable
	}
	return j, nil
}

// seedFinishedLocked resets the counters to what the previous runs settled; the walk then
// adds everything that is still to do.
func (j *job) seedFinishedLocked() {
	j.totalFiles, j.doneFiles, j.skippedFiles, j.totalBytes = 0, 0, 0, 0
	j.doneBytes.Store(0)
	for _, f := range j.finished {
		j.totalFiles++
		switch f.state {
		case StateDone:
			j.doneFiles++
			j.totalBytes += f.size
			j.doneBytes.Add(f.size)
		case StateSkipped:
			j.skippedFiles++
		case StateQueued, StateRunning, StateWaiting, StateFailed, StateCanceled:
		}
	}
}

// alreadyFinished reports whether a resumed run counted key from an earlier run, so the
// walk must not register it again.
func (e *Engine) alreadyFinished(j *job, key string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := j.finished[key]
	return ok
}
