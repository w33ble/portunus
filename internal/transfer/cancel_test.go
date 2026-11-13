package transfer

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCancelMidFileLeavesNoTargetAndClosesBothEnds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		blockOn  string // "read" stalls the source reader, "write" the destination writer
		existing bool
	}{
		{"stalled reader", "read", false},
		{"stalled writer", "write", false},
		{"overwrite keeps old target", "read", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			a, b := h.fs["a"], h.fs["b"]
			a.addFile("/src/big", strings.Repeat("x", 10_000))
			a.configure(func(f *fakeFS) { f.chunk = 1000 })
			b.addDir("/dst")
			policy := PolicyAsk
			if tc.existing {
				b.addFile("/dst/big", "old")
				policy = PolicyOverwrite
			}
			bl := newBlocker(tc.blockOn)
			target := a
			if tc.blockOn == "write" {
				target = b
			}
			target.setHook(bl.hook)
			bl.release(2) // two chunks flow, the third stalls mid-file
			id := h.copyJob("a", []string{"/src/big"}, "b", "/dst", policy)
			bl.waitArrived(h, 3)
			h.eventually("partial progress to be reported", func() bool { return h.job(id).DoneBytes > 0 })

			if err := h.e.Cancel(id); err != nil {
				t.Fatal(err)
			}
			v := h.waitState(id, StateCanceled)
			if v.FailedFiles != 0 || v.DoneBytes != 0 || len(v.Active) != 0 || v.FinishedAt == nil {
				t.Fatalf("view = %+v", v)
			}
			commits, aborts, _ := b.counters()
			if commits != 0 || aborts != 1 {
				t.Fatalf("commits=%d aborts=%d, want 0 and 1", commits, aborts)
			}
			if tc.existing {
				wantContent(t, b, "/dst/big", "old")
			} else {
				wantTree(t, b, "/dst")
			}
			h.assertQuiescent()
		})
	}
}

func TestCancelQueuedJobDoesNotDisturbTheRunningOne(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(1, 1))
	a := h.fs["a"]
	a.addFile("/s/one", "1").addFile("/s/two", "2")
	bl := newBlocker("open")
	a.setHook(bl.hook)
	first := h.copyJob("a", []string{"/s/one"}, "b", "/d1", PolicyAsk)
	bl.waitArrived(h, 1)
	second := h.copyJob("a", []string{"/s/two"}, "b", "/d2", PolicyAsk)
	h.eventually("second job queued", func() bool { return h.job(second).TotalFiles == 1 })
	if st := h.job(second).State; st != StateQueued {
		t.Fatalf("second job state = %s, want queued", st)
	}
	if err := h.e.Cancel(second); err != nil {
		t.Fatal(err)
	}
	h.waitState(second, StateCanceled)
	if st := h.job(first).State; st != StateRunning {
		t.Fatalf("first job state = %s, want still running", st)
	}
	bl.releaseAll()
	h.waitState(first, StateDone)
	if h.fs["b"].exists("/d2/two") {
		t.Fatal("canceled job copied its file")
	}
	bl.stays(t, 1)
}

func TestCancelWhileWalkIsBlocked(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.fs["a"].addFile("/src/x/y", "y")
	bl := newBlocker("list")
	h.fs["a"].setHook(bl.hook)
	id := h.copyJob("a", []string{"/src"}, "b", "/dst", PolicyAsk)
	bl.waitArrived(h, 1)
	if err := h.e.Cancel(id); err != nil {
		t.Fatal(err)
	}
	v := h.waitState(id, StateCanceled)
	if v.Walking {
		t.Fatal("canceled job still walking")
	}
}

func TestCancelOfFinishedOrUnknownJob(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.fs["a"].addFile("/f", "f")
	id := h.copyJob("a", []string{"/f"}, "b", "/d", PolicyAsk)
	h.waitState(id, StateDone)
	if err := h.e.Cancel(id); err != nil {
		t.Fatalf("Cancel(finished) = %v, want nil", err)
	}
	if st := h.job(id).State; st != StateDone {
		t.Fatalf("finished job changed to %s", st)
	}
	if err := h.e.Cancel("nope"); !errors.Is(err, ErrUnknownJob) {
		t.Fatalf("Cancel(unknown) = %v", err)
	}
}

func TestCancelUnderLoadSettlesEveryJob(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(4, 2))
	a := h.fs["a"]
	a.configure(func(f *fakeFS) { f.chunk = 64 })
	a.setLatency(100 * time.Microsecond)
	var ids []string
	for i := range 6 {
		dir := "/s" + string(rune('0'+i))
		addFiles(a, dir, numbered("f", 30))
		ids = append(ids, h.copyJob("a", []string{dir}, "b", "/d", PolicyOverwrite))
	}
	for i, id := range ids {
		if i%2 == 0 {
			if err := h.e.Cancel(id); err != nil {
				t.Fatal(err)
			}
		}
	}
	for i, id := range ids {
		v := h.waitFinal(id)
		if i%2 == 0 && v.State != StateCanceled {
			t.Errorf("job %d state = %s, want canceled", i, v.State)
		}
		if i%2 == 1 && v.State != StateDone {
			t.Errorf("job %d state = %s, want done (%+v)", i, v.State, v.Failed)
		}
	}
	h.assertQuiescent()
	for _, p := range h.fs["b"].tree("/") {
		if strings.Contains(p, "portunus-tmp") {
			t.Errorf("leftover temp file %s", p)
		}
	}
}
