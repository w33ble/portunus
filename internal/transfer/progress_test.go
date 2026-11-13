package transfer

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

func TestMeterConvergesDecaysAndIgnoresRollbacks(t *testing.T) {
	t.Parallel()
	var m meter
	start := time.Unix(1000, 0)
	if m.sample(start, 0) || m.speed != 0 {
		t.Fatal("first sample only sets the baseline")
	}
	bytes := int64(0)
	for i := 1; i <= 30; i++ {
		bytes += 1000
		m.sample(start.Add(time.Duration(i)*time.Second), bytes)
	}
	if math.Abs(m.speed-1000) > 10 {
		t.Fatalf("steady 1000 B/s converged to %.1f", m.speed)
	}

	// One slow second moves the estimate only part of the way: that is the smoothing.
	bytes += 200
	m.sample(start.Add(31*time.Second), bytes)
	if m.speed >= 1000 || m.speed <= 200 {
		t.Fatalf("after a slow second speed = %.1f, want between 200 and 1000", m.speed)
	}

	// A failed task rolls DoneBytes back; speed must not go negative.
	m.sample(start.Add(32*time.Second), bytes-5000)
	if m.speed < 0 {
		t.Fatalf("speed = %.1f", m.speed)
	}

	// Idle: decays toward zero and finally reads exactly zero.
	for i := 33; i < 200; i++ {
		m.sample(start.Add(time.Duration(i)*time.Second), bytes-5000)
	}
	if m.speed != 0 {
		t.Fatalf("idle speed = %v, want 0", m.speed)
	}
}

func TestMeterWeightsByElapsedTimeNotSampleCount(t *testing.T) {
	t.Parallel()
	steady := func(step time.Duration) float64 {
		var m meter
		now := time.Unix(0, 0)
		m.sample(now, 0)
		var bytes int64
		for elapsed := time.Duration(0); elapsed < 20*time.Second; elapsed += step {
			now = now.Add(step)
			bytes += int64(500 * step.Seconds())
			m.sample(now, bytes)
		}
		return m.speed
	}
	fine, coarse := steady(50*time.Millisecond), steady(time.Second)
	if math.Abs(fine-coarse) > 15 || math.Abs(fine-500) > 15 {
		t.Fatalf("sampling interval changed the estimate: fine=%.1f coarse=%.1f", fine, coarse)
	}
}

func TestSpeedAndETAAppearOnceBytesFlow(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.fs["a"]
	a.addFile("/src/f", strings.Repeat("z", 60_000))
	a.configure(func(f *fakeFS) { f.chunk = 1000 })
	a.setLatency(2 * time.Millisecond)
	id := h.copyJob("a", []string{"/src/f"}, "b", "/dst", PolicyAsk)
	sawSpeed := false
	h.eventually("speed and ETA while copying", func() bool {
		v := h.job(id)
		if isFinal(v.State) {
			t.Fatalf("finished before speed was reported: %+v", v)
		}
		if v.BytesPerSec > 0 && v.ETASeconds >= 0 && !v.Walking {
			sawSpeed = true
		}
		return sawSpeed
	})
	v := h.waitFinal(id)
	if v.BytesPerSec != 0 || v.ETASeconds != 0 {
		t.Fatalf("finished job reports speed %v eta %d", v.BytesPerSec, v.ETASeconds)
	}
}

func TestStateTransitionsAreEmittedWithoutWaitingForATick(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withTick(time.Hour))
	h.fs["a"].addFile("/f", "f")
	id := h.copyJob("a", []string{"/f"}, "b", "/d", PolicyAsk)
	h.eventually("done update with a one-hour tick", func() bool { return h.sawJobState(id, StateDone) })
	first := h.allUpdates()[0]
	if len(first.Jobs) != 1 || first.Jobs[0].State != StateQueued && first.Jobs[0].State != StateRunning {
		t.Fatalf("job creation was not emitted promptly: %+v", first)
	}
}

func TestUpdatesCarryOnlyJobsThatChanged(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.fs["a"]
	a.addFile("/stuck", "s").addFile("/quick", "q")
	bl := newBlocker("open")
	a.setHook(func(ctx context.Context, op, p string) error {
		if p == "/stuck" {
			return bl.hook(ctx, op, p)
		}
		return nil
	})
	stuck := h.copyJob("a", []string{"/stuck"}, "b", "/d1", PolicyAsk)
	bl.waitArrived(h, 1)
	quick := h.copyJob("a", []string{"/quick"}, "b", "/d2", PolicyAsk)
	h.eventually("quick job's done update", func() bool { return h.sawJobState(quick, StateDone) })
	h.eventually("stuck job's running state to be flushed", func() bool { return h.sawJobState(stuck, StateRunning) })
	time.Sleep(30 * time.Millisecond) // anything still in flight lands before the mark
	mark := len(h.allUpdates())
	time.Sleep(60 * time.Millisecond) // a dozen ticks with nothing changing
	for _, u := range h.allUpdates()[mark:] {
		t.Errorf("idle engine emitted %+v", u)
	}
	bl.releaseAll()
	h.waitState(stuck, StateDone)
}

func TestProgressUpdatesAreBatchedAcrossJobs(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withTick(20*time.Millisecond))
	a := h.fs["a"]
	a.configure(func(f *fakeFS) { f.chunk = 100 })
	a.setLatency(200 * time.Microsecond)
	a.addFile("/one", strings.Repeat("1", 20_000)).addFile("/two", strings.Repeat("2", 20_000))
	i1 := h.copyJob("a", []string{"/one"}, "b", "/d1", PolicyAsk)
	i2 := h.copyJob("a", []string{"/two"}, "c", "/d2", PolicyAsk)
	h.waitState(i1, StateDone)
	h.waitState(i2, StateDone)
	both := 0
	for _, u := range h.allUpdates() {
		if len(u.Jobs) == 2 {
			both++
		}
	}
	if both == 0 {
		t.Fatal("no update batched both jobs' progress")
	}
}

func TestFinishedTasksCollapseIntoCounters(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(4, 4))
	a := h.fs["a"]
	addFiles(a, "/src", numbered("f", 300))
	v := h.waitFinal(h.copyJob("a", []string{"/src"}, "b", "/dst", PolicyAsk))
	if v.DoneFiles != 301 || len(v.Active) != 0 || len(v.Failed) != 0 {
		t.Fatalf("view = %+v", v)
	}
	for _, u := range h.allUpdates() {
		for _, j := range u.Jobs {
			if len(j.Active) > 5 { // 4 workers plus the directory being expanded
				t.Fatalf("update shows %d active rows with 4 workers", len(j.Active))
			}
		}
	}
}

func TestFailedListIsCappedButCountIsComplete(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.fs["a"]
	names := numbered("f", 250)
	addFiles(a, "/src", names)
	a.setHook(func(_ context.Context, op, _ string) error {
		if op == "open" {
			return errors.New("denied")
		}
		return nil
	})
	v := h.waitFinal(h.copyJob("a", []string{"/src"}, "b", "/dst", PolicyAsk))
	if v.State != StateFailed || v.FailedFiles != 250 || len(v.Failed) != maxFailedViews {
		t.Fatalf("failed=%d listed=%d state=%s", v.FailedFiles, len(v.Failed), v.State)
	}
	if f := v.Failed[0]; f.State != StateFailed || !strings.Contains(f.Error, "denied") || f.Dst == "" {
		t.Fatalf("task view = %+v", f)
	}
}

func TestActiveTasksReportLiveByteProgress(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.fs["a"]
	a.addFile("/f", strings.Repeat("x", 5000))
	a.configure(func(f *fakeFS) { f.chunk = 1000 })
	bl := newBlocker("read")
	a.setHook(bl.hook)
	bl.release(3)
	id := h.copyJob("a", []string{"/f"}, "b", "/d", PolicyAsk)
	bl.waitArrived(h, 4)
	h.eventually("task progress", func() bool {
		v := h.job(id)
		return len(v.Active) == 1 && v.Active[0].Done == 3000
	})
	v := h.job(id)
	if got := v.Active[0]; got.Size != 5000 || got.State != StateRunning || got.Src != "/f" || got.Dst != "/d/f" || v.DoneBytes != 3000 {
		t.Fatalf("active = %+v job = %+v", got, v)
	}
	bl.releaseAll()
	h.waitState(id, StateDone)
}
