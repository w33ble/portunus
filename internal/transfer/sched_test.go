package transfer

import (
	"fmt"
	"slices"
	"testing"
	"time"
)

// trackTotal makes every fake count open readers in one shared gauge: each running copy
// task holds exactly one reader, so the gauge is the number of tasks running.
func trackTotal(h *harness) *gauge {
	g := &gauge{}
	for _, f := range h.fs {
		f.configure(func(f *fakeFS) { f.total = g })
	}
	return g
}

func TestGlobalLimitCapsConcurrentTasks(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(4, 8))
	total := trackTotal(h)
	names := numbered("f", 12)
	addFiles(h.fs["a"], "/src", names)
	bl := newBlocker("open")
	h.fs["a"].setHook(bl.hook)
	id := h.copyJob("a", joinPaths("/src", names), "b", "/dst", PolicyAsk)
	bl.waitArrived(h, 4)
	bl.stays(t, 4)
	if cur := total.cur.Load(); cur != 4 {
		t.Fatalf("running = %d, want 4", cur)
	}
	bl.releaseAll()
	if v := h.waitFinal(id); v.State != StateDone {
		t.Fatalf("view = %+v", v)
	}
	if peak := total.peak.Load(); peak != 4 {
		t.Fatalf("peak concurrency = %d, want 4", peak)
	}
}

func TestPerConnectionLimitCapsTasksTouchingOneConnection(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(8, 2))
	names := numbered("f", 8)
	addFiles(h.fs["a"], "/src", names)
	bl := newBlocker("open")
	h.fs["a"].setHook(bl.hook)
	id := h.copyJob("a", joinPaths("/src", names), "b", "/dst", PolicyAsk)
	bl.waitArrived(h, 2)
	bl.stays(t, 2)
	bl.releaseAll()
	h.waitState(id, StateDone)
	for _, c := range []string{"a", "b"} {
		if peak := h.fs[c].active.peak.Load(); peak > 2 {
			t.Errorf("conn %s: %d tasks at once, limit 2", c, peak)
		}
	}
}

func TestSaturatedConnectionDoesNotBlockOtherConnections(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(4, 1))
	addFiles(h.fs["a"], "/s", numbered("f", 3))
	h.fs["c"].addFile("/s/x", "x")
	bl := newBlocker("open")
	h.fs["a"].setHook(bl.hook)
	blocked := h.copyJob("a", joinPaths("/s", numbered("f", 3)), "b", "/d", PolicyAsk)
	bl.waitArrived(h, 1)
	free := h.copyJob("c", []string{"/s/x"}, "c", "/d", PolicyAsk) // first job is older and still has queued work
	h.waitState(free, StateDone)
	bl.stays(t, 1)
	bl.releaseAll()
	h.waitState(blocked, StateDone)
}

func TestOpposingRemoteToRemoteJobsDoNotDeadlock(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(4, 1))
	names := numbered("f", 25)
	for _, c := range []string{"a", "b", "c"} {
		addFiles(h.fs[c], "/s"+c, names)
		h.fs[c].configure(func(f *fakeFS) { f.chunk = 4 })
		h.fs[c].setLatency(20 * time.Microsecond)
	}
	routes := [][2]string{{"a", "b"}, {"b", "a"}, {"b", "c"}, {"c", "b"}, {"c", "a"}, {"a", "c"}}
	var ids []string
	for _, r := range routes {
		ids = append(ids, h.copyJob(r[0], []string{"/s" + r[0]}, r[1], "/in-"+r[0], PolicyOverwrite))
	}
	for i, id := range ids {
		if v := h.waitFinal(id); v.State != StateDone {
			t.Fatalf("route %v: %+v", routes[i], v)
		}
	}
	for _, c := range []string{"a", "b", "c"} {
		if peak := h.fs[c].active.peak.Load(); peak > 1 {
			t.Errorf("conn %s: %d tasks at once, limit 1", c, peak)
		}
	}
	h.assertQuiescent()
}

func TestSameConnectionCopyNeedsOnlyOnePermit(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(4, 1)) // two permits on one connection would deadlock here
	total := trackTotal(h)
	addFiles(h.fs["a"], "/src", numbered("f", 6))
	v := h.waitFinal(h.copyJob("a", []string{"/src"}, "a", "/copy", PolicyAsk))
	if v.State != StateDone {
		t.Fatalf("view = %+v", v)
	}
	if peak := total.peak.Load(); peak != 1 {
		t.Fatalf("peak = %d, want 1", peak)
	}
	wantContent(t, h.fs["a"], "/copy/src/f005", "content of f005")
}

func TestSameConnectionTaskUsesOnePermitNotTwo(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(4, 2))
	names := numbered("f", 6)
	addFiles(h.fs["a"], "/src", names)
	bl := newBlocker("open")
	h.fs["a"].setHook(bl.hook)
	id := h.copyJob("a", joinPaths("/src", names), "a", "/copy", PolicyAsk)
	bl.waitArrived(h, 2) // two permits per task would allow only one
	bl.stays(t, 2)
	bl.releaseAll()
	h.waitState(id, StateDone)
}

func TestLimitsBelowOneMeanOne(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(0, -3))
	total := trackTotal(h)
	addFiles(h.fs["a"], "/src", numbered("f", 6))
	h.waitFinal(h.copyJob("a", []string{"/src"}, "b", "/dst", PolicyAsk))
	if peak := total.peak.Load(); peak != 1 {
		t.Fatalf("peak = %d, want 1", peak)
	}
	h.e.SetLimits(Limits{Global: -1, PerConn: 0})
	addFiles(h.fs["a"], "/src2", numbered("g", 6))
	h.waitFinal(h.copyJob("a", []string{"/src2"}, "b", "/dst", PolicyAsk))
	if peak := total.peak.Load(); peak != 1 {
		t.Fatalf("peak after SetLimits = %d, want 1", peak)
	}
}

func TestRaisingLimitsMidRunStartsMoreTasks(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(1, 1))
	names := numbered("f", 10)
	addFiles(h.fs["a"], "/src", names)
	bl := newBlocker("open")
	h.fs["a"].setHook(bl.hook)
	id := h.copyJob("a", joinPaths("/src", names), "b", "/dst", PolicyAsk)
	bl.waitArrived(h, 1)
	bl.stays(t, 1)
	h.e.SetLimits(Limits{Global: 5, PerConn: 3})
	bl.waitArrived(h, 3)
	bl.stays(t, 3) // per-connection limit now binds
	h.e.SetLimits(Limits{Global: 5, PerConn: 5})
	bl.waitArrived(h, 5)
	bl.stays(t, 5)
	bl.releaseAll()
	h.waitState(id, StateDone)
}

func TestLoweringLimitsLetsRunningTasksFinishAndHoldsNewStarts(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(4, 4))
	names := numbered("f", 12)
	addFiles(h.fs["a"], "/src", names)
	bl := newBlocker("open")
	h.fs["a"].setHook(bl.hook)
	id := h.copyJob("a", joinPaths("/src", names), "b", "/dst", PolicyAsk)
	bl.waitArrived(h, 4)

	h.e.SetLimits(Limits{Global: 1, PerConn: 1})
	if v := h.job(id); len(v.Active) != 4 || v.State != StateRunning {
		t.Fatalf("lowering disturbed running tasks: %+v", v)
	}
	// Three of the four running tasks finish; nothing new may start while any is running.
	bl.release(3)
	h.eventually("three tasks to finish", func() bool { return h.job(id).DoneFiles == 3 })
	bl.stays(t, 4)
	// The last one finishes: now exactly one new task starts, not four.
	bl.release(1)
	bl.waitArrived(h, 5)
	bl.stays(t, 5)
	bl.releaseAll()
	if v := h.waitFinal(id); v.State != StateDone || v.DoneFiles != 12 {
		t.Fatalf("final = %+v", v)
	}
}

func TestStressAtGlobalFourKeepsEveryLimit(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(4, 2), withTick(2*time.Millisecond))
	total := trackTotal(h)
	for _, c := range []string{"a", "b", "c"} {
		for d := range 4 {
			addFiles(h.fs[c], fmt.Sprintf("/s/%s/d%d", c, d), numbered("f", 15))
		}
		h.fs[c].addDir("/s/" + c + "/empty")
		h.fs[c].configure(func(f *fakeFS) { f.chunk = 8; f.readerWriterTo = c == "a"; f.writerReaderFrom = c == "b" })
		h.fs[c].setLatency(30 * time.Microsecond)
	}
	routes := [][2]string{{"a", "b"}, {"b", "c"}, {"c", "a"}, {"a", "c"}, {"b", "a"}, {"c", "b"}}
	var ids []string
	for _, r := range routes {
		ids = append(ids, h.copyJob(r[0], []string{"/s/" + r[0]}, r[1], "/out/"+r[0]+r[1], PolicyOverwrite))
	}
	for i, id := range ids {
		v := h.waitFinal(id)
		if v.State != StateDone || v.DoneFiles != 4*15+4+1+1 {
			t.Fatalf("route %v: %+v", routes[i], v)
		}
		got := h.fs[routes[i][1]].tree("/out/" + routes[i][0] + routes[i][1])
		want := h.fs[routes[i][0]].tree("/s")
		if len(got) != len(want) {
			t.Fatalf("route %v: %d paths copied, want %d", routes[i], len(got), len(want))
		}
	}
	if peak := total.peak.Load(); peak > 4 {
		t.Fatalf("global peak = %d, limit 4", peak)
	}
	for c, f := range h.fs {
		if peak := f.active.peak.Load(); peak > 2 {
			t.Errorf("conn %s: %d tasks at once, limit 2", c, peak)
		}
	}
	h.assertQuiescent()
	if !slices.IsSortedFunc(h.e.Snapshot(), func(a, b JobView) int { return a.CreatedAt.Compare(b.CreatedAt) }) {
		t.Error("Snapshot is not in creation order")
	}
}
