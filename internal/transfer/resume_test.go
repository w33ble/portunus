package transfer

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// onPath limits a blocker to one path.
func onPath(bl *blocker, target string) func(ctx context.Context, op, p string) error {
	return func(ctx context.Context, op, p string) error {
		if p != target {
			return nil
		}
		return bl.hook(ctx, op, p)
	}
}

func (h *harness) cancelAndWait(id string) JobView {
	h.t.Helper()
	if err := h.e.Cancel(id); err != nil {
		h.t.Fatal(err)
	}
	return h.waitState(id, StateCanceled)
}

func (h *harness) resume(id string) {
	h.t.Helper()
	if err := h.e.Resume(id); err != nil {
		h.t.Fatalf("Resume: %v", err)
	}
}

func TestResumeAfterCancelDuringWalk(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(1, 1))
	a, b := h.fs["a"], h.fs["b"]
	names := numbered("f", 5)
	addFiles(a, "/src", names)
	a.addFile("/src/sub/inner", "inner")
	bl := newBlocker("list")
	a.setHook(onPath(bl, "/src/sub"))
	id := h.copyJob("a", []string{"/src"}, "b", "/dst", PolicyAsk)
	bl.waitArrived(h, 1)
	h.eventually("top-level files to finish", func() bool { return h.job(id).DoneFiles >= 6 })
	v := h.cancelAndWait(id)
	if v.DoneFiles != 6 || b.exists("/dst/src/sub/inner") {
		t.Fatalf("canceled view = %+v", v)
	}

	bl.releaseAll()
	h.resume(id)
	v = h.waitState(id, StateDone)
	// src, sub, five files and inner
	if v.TotalFiles != 8 || v.DoneFiles != 8 || v.DoneBytes != v.TotalBytes || v.FailedFiles != 0 || v.SkippedFiles != 0 {
		t.Fatalf("resumed view = %+v", v)
	}
	wantContent(t, b, "/dst/src/sub/inner", "inner")
	for _, n := range names {
		wantContent(t, b, "/dst/src/"+n, "content of "+n)
		if got := a.count(a.opens, "/src/"+n); got != 1 {
			t.Errorf("%s opened %d times, want 1", n, got)
		}
	}
	h.assertQuiescent()
}

func TestResumeRestartsTheInFlightFileAndSkipsFinishedOnes(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(1, 1))
	a, b := h.fs["a"], h.fs["b"]
	a.configure(func(f *fakeFS) { f.chunk = 1000 })
	body := func(n string) string { return strings.Repeat(n, 3000) }
	for _, n := range []string{"0", "1", "2", "3"} {
		a.addFile("/src/f"+n, body(n))
	}
	bl := newBlocker("read")
	a.setHook(onPath(bl, "/src/f2"))
	bl.release(1) // one chunk of f2 flows, the next stalls
	id := h.copyJob("a", []string{"/src/f0", "/src/f1", "/src/f2", "/src/f3"}, "b", "/dst", PolicyAsk)
	bl.waitArrived(h, 2)
	h.eventually("partial progress on f2", func() bool { return h.job(id).DoneBytes > 6000 })
	v := h.cancelAndWait(id)
	if v.DoneFiles != 2 || v.DoneBytes != 6000 {
		t.Fatalf("canceled view = %+v", v)
	}
	wantTree(t, b, "/dst", "/dst/f0", "/dst/f1")

	bl.releaseAll()
	h.resume(id)
	v = h.waitState(id, StateDone)
	if v.TotalFiles != 4 || v.DoneFiles != 4 || v.TotalBytes != 12000 || v.DoneBytes != 12000 {
		t.Fatalf("resumed view = %+v", v)
	}
	for _, n := range []string{"0", "1", "2", "3"} {
		wantContent(t, b, "/dst/f"+n, body(n))
		want := 1
		if n == "2" {
			want = 2 // canceled mid-file, restarted from byte 0
		}
		if got := a.count(a.opens, "/src/f"+n); got != want {
			t.Errorf("f%s opened %d times, want %d", n, got, want)
		}
	}
	if commits, _, _ := b.counters(); commits != 4 {
		t.Fatalf("commits = %d, want 4", commits)
	}
	h.assertQuiescent()
}

func TestResumeAppliesTheOriginalConflictPolicy(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(1, 1))
	a, b := h.fs["a"], h.fs["b"]
	addFiles(a, "/src", numbered("f", 3))
	b.addFile("/dst/f002", "old")
	bl := newBlocker("open")
	a.setHook(onPath(bl, "/src/f001"))
	id := h.copyJob("a", joinPaths("/src", numbered("f", 3)), "b", "/dst", PolicySkip)
	bl.waitArrived(h, 1)
	h.cancelAndWait(id)

	bl.releaseAll()
	h.resume(id)
	v := h.waitState(id, StateDone)
	if v.DoneFiles != 2 || v.SkippedFiles != 1 || v.TotalFiles != 3 {
		t.Fatalf("resumed view = %+v", v)
	}
	wantContent(t, b, "/dst/f002", "old")
	wantContent(t, b, "/dst/f001", "content of f001")
	if got := a.count(a.opens, "/src/f000"); got != 1 {
		t.Fatalf("f000 opened %d times, want 1", got)
	}
}

func TestResumeFinishesACanceledDelete(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(1, 1))
	a := h.fs["a"]
	deleteFixture(a)
	a.addFile("/keep", "keep")
	bl := newBlocker("remove")
	a.setHook(bl.hook)
	bl.release(3)
	id := h.deleteJob("a", "/d")
	bl.waitArrived(h, 4)
	h.cancelAndWait(id)
	if n := len(a.removedPaths()); n != 3 {
		t.Fatalf("removed %d paths before cancel, want 3", n)
	}

	bl.releaseAll()
	h.resume(id)
	v := h.waitState(id, StateDone)
	// 5 files + 4 directories
	if v.TotalFiles != 9 || v.DoneFiles != 9 || v.SkippedFiles != 0 {
		t.Fatalf("resumed view = %+v", v)
	}
	wantTree(t, a, "/", "/keep")
	wantBottomUp(t, a.removedPaths())
}

func TestResumeCountsDeletedRootsThatAreGone(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(1, 1))
	a := h.fs["a"]
	a.addFile("/x", "x").addFile("/y", "y").addFile("/z", "z")
	bl := newBlocker("remove")
	a.setHook(bl.hook)
	bl.release(2)
	id := h.deleteJob("a", "/x", "/y", "/z")
	bl.waitArrived(h, 3)
	h.cancelAndWait(id)

	bl.releaseAll()
	h.resume(id)
	v := h.waitState(id, StateDone)
	if v.TotalFiles != 3 || v.DoneFiles != 3 || v.SkippedFiles != 0 {
		t.Fatalf("resumed view = %+v", v)
	}
	wantTree(t, a, "/")
}

func TestCancelResumeCancelResume(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(1, 1))
	a, b := h.fs["a"], h.fs["b"]
	names := numbered("f", 4)
	addFiles(a, "/src", names)
	bl := newBlocker("open")
	a.setHook(onPath(bl, "/src/f002"))
	id := h.copyJob("a", joinPaths("/src", names), "b", "/dst", PolicyAsk)
	bl.waitArrived(h, 1)
	h.cancelAndWait(id)

	before := h.resolves.Load()
	h.resume(id)
	if got := h.resolves.Load() - before; got != 2 {
		t.Fatalf("Resume resolved %d connections, want 2 (it must pick up reconnects)", got)
	}
	bl.waitArrived(h, 2)
	v := h.cancelAndWait(id)
	if v.DoneFiles != 2 || v.FinishedAt == nil {
		t.Fatalf("second cancel = %+v", v)
	}

	bl.releaseAll()
	h.resume(id)
	v = h.waitState(id, StateDone)
	if v.TotalFiles != 4 || v.DoneFiles != 4 || v.DoneBytes != v.TotalBytes {
		t.Fatalf("final view = %+v", v)
	}
	for _, n := range names {
		wantContent(t, b, "/dst/"+n, "content of "+n)
		want := 1
		if n == "f002" {
			want = 3
		}
		if got := a.count(a.opens, "/src/"+n); got != want {
			t.Errorf("%s opened %d times, want %d", n, got, want)
		}
	}
	h.assertQuiescent()
}

func TestResumeRefusesWhatIsNotCanceled(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.fs["a"].addFile("/f", "f")
	id := h.copyJob("a", []string{"/f"}, "b", "/d", PolicyAsk)
	h.waitState(id, StateDone)
	if err := h.e.Resume(id); !errors.Is(err, ErrNotResumable) {
		t.Fatalf("Resume(done) = %v, want ErrNotResumable", err)
	}
	if err := h.e.Resume("nope"); !errors.Is(err, ErrUnknownJob) {
		t.Fatalf("Resume(unknown) = %v, want ErrUnknownJob", err)
	}
}

func TestResumeNeedsALiveConnectionAndKeepsTheJobResumable(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(1, 1))
	a := h.fs["a"]
	a.addFile("/src/x", "x")
	bl := newBlocker("open")
	a.setHook(bl.hook)
	id := h.copyJob("a", []string{"/src/x"}, "b", "/dst", PolicyAsk)
	bl.waitArrived(h, 1)
	h.cancelAndWait(id)

	h.down.Store(true)
	if err := h.e.Resume(id); err == nil {
		t.Fatal("Resume with the connection down succeeded")
	}
	if st := h.job(id).State; st != StateCanceled {
		t.Fatalf("state = %s, want still canceled", st)
	}
	h.down.Store(false)
	bl.releaseAll()
	h.resume(id)
	h.waitState(id, StateDone)
	wantContent(t, h.fs["b"], "/dst/x", "x")
}

func TestClearDropsCanceledJobs(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.fs["a"].addFile("/src/x", "x")
	bl := newBlocker("open")
	h.fs["a"].setHook(bl.hook)
	id := h.copyJob("a", []string{"/src/x"}, "b", "/dst", PolicyAsk)
	bl.waitArrived(h, 1)
	h.cancelAndWait(id)
	h.e.Clear()
	if _, ok := h.find(id); ok {
		t.Fatal("canceled job survived Clear")
	}
	if err := h.e.Resume(id); !errors.Is(err, ErrUnknownJob) {
		t.Fatalf("Resume(cleared) = %v, want ErrUnknownJob", err)
	}
}
