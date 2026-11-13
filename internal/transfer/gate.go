package transfer

import (
	"errors"
	"fmt"

	"github.com/w33ble/portunus/internal/prompt"
)

var errNoAsker = errors.New("destination exists and no conflict handler is configured")

// runGate is a job's conflict gate. The walker parks every conflicting task here in state
// waiting; the gate asks about one at a time, so a job never stacks dialogs, and parked
// tasks hold no worker slot while non-conflicting files keep copying. ApplyToAll makes the
// gate (and the walker, for later conflicts) answer without asking.
func (e *Engine) runGate(j *job, in <-chan *task) {
	defer e.producerDone(j)
	for t := range in {
		if j.ctx.Err() != nil {
			return
		}
		e.resolveConflict(j, t)
	}
}

func (e *Engine) producerDone(j *job) {
	e.mu.Lock()
	defer e.mu.Unlock()
	j.producers--
	e.finishCheckLocked(j)
}

func (e *Engine) resolveConflict(j *job, t *task) {
	act, err := e.decide(j, t)
	if err != nil {
		e.settle(j, t, outcomeOf(j, err), fmt.Errorf("resolve conflict at %s: %w", t.dst, err))
		return
	}
	switch act {
	case prompt.ConflictOverwrite:
		e.release(j, t)
	case prompt.ConflictSkip:
		e.settle(j, t, StateSkipped, nil)
	case prompt.ConflictCancel:
		e.mu.Lock()
		j.cancel()
		e.settleLocked(j, t, StateCanceled, nil)
		e.mu.Unlock()
	default:
		e.settle(j, t, StateFailed, fmt.Errorf("unknown conflict action %q", act))
	}
}

// decide returns what to do with a parked task, asking the user unless an earlier
// "apply to all" answer already covers it.
func (e *Engine) decide(j *job, t *task) (prompt.ConflictAction, error) {
	e.mu.Lock()
	if act := j.applyAll; act != "" {
		e.mu.Unlock()
		return act, nil
	}
	if e.asker == nil {
		e.mu.Unlock()
		return "", errNoAsker
	}
	e.setStateLocked(j, StateWaiting)
	req := prompt.ConflictRequest{JobID: j.id, Src: t.ent, Dst: *t.dstEnt}
	e.mu.Unlock()

	ans, err := e.asker.AskConflict(j.ctx, req)

	e.mu.Lock()
	defer e.mu.Unlock()
	e.restoreStateLocked(j)
	if err != nil {
		return "", err
	}
	if ans.ApplyToAll && (ans.Action == prompt.ConflictOverwrite || ans.Action == prompt.ConflictSkip) {
		j.applyAll = ans.Action
	}
	return ans.Action, nil
}

// release sends a task whose conflict was resolved as "overwrite" on to the scheduler.
func (e *Engine) release(j *job, t *task) {
	e.mu.Lock()
	t.overwrite = true
	t.state = StateQueued
	delete(j.active, t)
	e.touch(j)
	e.mu.Unlock()
	e.enqueueReady(j, t)
}
