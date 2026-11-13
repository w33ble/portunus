package transfer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/w33ble/portunus/internal/prompt"
)

// conflictFixture puts n source files named c000.. in srcDir on a; same-named files
// already exist (as "old") in dstDir on b.
func conflictFixture(h *harness, srcDir, dstDir string, n int) []string {
	names := numbered("c", n)
	for _, name := range names {
		h.fs["a"].addFile(srcDir+"/"+name, "new "+name)
		h.fs["b"].addFile(dstDir+"/"+name, "old")
	}
	return names
}

func answerWith(a prompt.ConflictAnswer) func(context.Context, prompt.ConflictRequest) (prompt.ConflictAnswer, error) {
	return func(context.Context, prompt.ConflictRequest) (prompt.ConflictAnswer, error) { return a, nil }
}

func TestGateAsksOneAtATimeAndParkedTasksHoldNoWorkerSlot(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(1, 1)) // a single slot: parked tasks must not occupy it
	a, b := h.fs["a"], h.fs["b"]
	names := conflictFixture(h, "/src", "/dst", 3)
	a.addFile("/src/z-free", "free")
	b.addDir("/dst")
	answers := make(chan prompt.ConflictAnswer)
	h.asker.set(func(ctx context.Context, _ prompt.ConflictRequest) (prompt.ConflictAnswer, error) {
		select {
		case ans := <-answers:
			return ans, nil
		case <-ctx.Done():
			return prompt.ConflictAnswer{}, ctx.Err()
		}
	})
	id := h.copyJob("a", joinPaths("/src", append(names, "z-free")), "b", "/dst", PolicyAsk)

	h.eventually("the free file to copy while a prompt is open", func() bool {
		_, ok := b.content("/dst/z-free")
		return ok
	})
	h.eventually("first prompt", func() bool { return h.asker.callCount() == 1 })
	v := h.job(id)
	if v.State != StateWaiting || len(v.Active) != 3 {
		t.Fatalf("state=%s active=%d, want waiting with 3 parked tasks", v.State, len(v.Active))
	}
	for _, task := range v.Active {
		if task.State != StateWaiting {
			t.Fatalf("task %s is %s, want waiting", task.Src, task.State)
		}
	}

	answers <- prompt.ConflictAnswer{Action: prompt.ConflictOverwrite}
	h.eventually("second prompt only after the first answer", func() bool { return h.asker.callCount() == 2 })
	answers <- prompt.ConflictAnswer{Action: prompt.ConflictSkip}
	h.eventually("third prompt", func() bool { return h.asker.callCount() == 3 })
	answers <- prompt.ConflictAnswer{Action: prompt.ConflictOverwrite}

	v = h.waitFinal(id)
	if v.State != StateDone || v.SkippedFiles != 1 || v.DoneFiles != 3 {
		t.Fatalf("final = %+v", v)
	}
	wantContent(t, b, "/dst/c000", "new c000")
	wantContent(t, b, "/dst/c001", "old")
	wantContent(t, b, "/dst/c002", "new c002")
	if h.asker.maxConcurrent() != 1 {
		t.Fatalf("%d prompts were open at once", h.asker.maxConcurrent())
	}
	req := h.asker.calls[0]
	if req.JobID != id || req.Src.Path != "/src/c000" || req.Dst.Size != 3 {
		t.Fatalf("first prompt = %+v", req)
	}
}

func TestApplyToAllResolvesWaitingAndFutureConflicts(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		action prompt.ConflictAction
		want   string
	}{
		{prompt.ConflictOverwrite, "new "},
		{prompt.ConflictSkip, "old"},
	} {
		t.Run(string(tc.action), func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			names := conflictFixture(h, "/src", "/dst/src", 40) // more than the walker emits before the first answer
			h.fs["b"].addDir("/dst")
			h.asker.set(answerWith(prompt.ConflictAnswer{Action: tc.action, ApplyToAll: true}))
			v := h.waitFinal(h.copyJob("a", []string{"/src"}, "b", "/dst", PolicyAsk))
			if v.State != StateDone {
				t.Fatalf("state = %s %+v", v.State, v.Failed)
			}
			if n := h.asker.callCount(); n != 1 {
				t.Fatalf("prompted %d times, want 1", n)
			}
			for _, n := range names {
				want := tc.want
				if tc.action == prompt.ConflictOverwrite {
					want += n
				}
				wantContent(t, h.fs["b"], "/dst/src/"+n, want)
			}
			if tc.action == prompt.ConflictSkip && v.SkippedFiles != 40 {
				t.Fatalf("skipped = %d", v.SkippedFiles)
			}
		})
	}
}

func TestConflictCancelCancelsTheWholeJob(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(1, 1))
	names := conflictFixture(h, "/src", "/dst", 5)
	h.fs["b"].addDir("/dst")
	h.asker.set(answerWith(prompt.ConflictAnswer{Action: prompt.ConflictCancel}))
	v := h.waitFinal(h.copyJob("a", joinPaths("/src", names), "b", "/dst", PolicyAsk))
	if v.State != StateCanceled || v.FailedFiles != 0 || len(v.Active) != 0 {
		t.Fatalf("view = %+v", v)
	}
	if n := h.asker.callCount(); n != 1 {
		t.Fatalf("prompted %d times after cancel, want 1", n)
	}
	for _, n := range names {
		wantContent(t, h.fs["b"], "/dst/"+n, "old")
	}
	h.assertQuiescent()
}

func TestCancelWhilePromptIsOpenUnblocksIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	names := conflictFixture(h, "/src", "/dst", 2)
	h.fs["b"].addDir("/dst")
	h.asker.set(func(ctx context.Context, _ prompt.ConflictRequest) (prompt.ConflictAnswer, error) {
		<-ctx.Done()
		return prompt.ConflictAnswer{}, ctx.Err()
	})
	id := h.copyJob("a", joinPaths("/src", names), "b", "/dst", PolicyAsk)
	h.eventually("prompt", func() bool { return h.asker.callCount() == 1 })
	if err := h.e.Cancel(id); err != nil {
		t.Fatal(err)
	}
	v := h.waitState(id, StateCanceled)
	if v.FailedFiles != 0 || h.asker.callCount() != 1 {
		t.Fatalf("view = %+v prompts=%d", v, h.asker.callCount())
	}
}

func TestAskerErrorFailsTheTaskAndRetryAsksAgain(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	names := conflictFixture(h, "/src", "/dst", 3)
	h.fs["b"].addDir("/dst")
	h.asker.set(func(context.Context, prompt.ConflictRequest) (prompt.ConflictAnswer, error) {
		return prompt.ConflictAnswer{}, errors.New("dialog closed")
	})
	id := h.copyJob("a", joinPaths("/src", names), "b", "/dst", PolicyAsk)
	v := h.waitFinal(id)
	if v.State != StateFailed || v.FailedFiles != 3 || !strings.Contains(v.Failed[0].Error, "dialog closed") {
		t.Fatalf("view = %+v", v)
	}
	h.asker.set(answerWith(prompt.ConflictAnswer{Action: prompt.ConflictOverwrite, ApplyToAll: true}))
	if err := h.e.Retry(id); err != nil {
		t.Fatal(err)
	}
	v = h.waitState(id, StateDone)
	if v.FailedFiles != 0 || v.DoneFiles != 3 {
		t.Fatalf("after retry = %+v", v)
	}
	wantContent(t, h.fs["b"], "/dst/c002", "new c002")
}

func TestAskPolicyWithoutAskerFailsConflicts(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(o *Options) { o.Conflicts = nil })
	conflictFixture(h, "/src", "/dst", 1)
	h.fs["b"].addDir("/dst")
	v := h.waitFinal(h.copyJob("a", []string{"/src/c000"}, "b", "/dst", PolicyAsk))
	if v.State != StateFailed || !strings.Contains(v.Failed[0].Error, "no conflict handler") {
		t.Fatalf("view = %+v", v)
	}
	wantContent(t, h.fs["b"], "/dst/c000", "old")
}
