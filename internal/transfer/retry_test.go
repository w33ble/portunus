package transfer

import (
	"errors"
	"testing"
)

func TestRetryReEnqueuesOnlyTheFailedTasks(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a, b := h.fs["a"], h.fs["b"]
	names := numbered("f", 6)
	addFiles(a, "/src", names)
	a.failTimes("open", "/src/f001", 1, errors.New("flaky"))
	a.failTimes("open", "/src/f004", 1, errors.New("flaky"))
	id := h.copyJob("a", joinPaths("/src", names), "b", "/dst", PolicyAsk)
	v := h.waitFinal(id)
	if v.State != StateFailed || v.FailedFiles != 2 || v.DoneFiles != 4 {
		t.Fatalf("view = %+v", v)
	}
	before := h.resolves.Load()
	if err := h.e.Retry(id); err != nil {
		t.Fatal(err)
	}
	if got := h.resolves.Load() - before; got != 2 {
		t.Fatalf("Retry resolved %d connections, want 2 (it must pick up reconnects)", got)
	}
	v = h.waitState(id, StateDone)
	if v.FailedFiles != 0 || v.DoneFiles != 6 || v.TotalFiles != 6 || v.DoneBytes != v.TotalBytes {
		t.Fatalf("after retry = %+v", v)
	}
	for _, n := range names {
		want := 1
		if n == "f001" || n == "f004" {
			want = 2 // failed once, retried once
		}
		if got := a.count(a.opens, "/src/"+n); got != want {
			t.Errorf("%s opened %d times, want %d", n, got, want)
		}
		wantContent(t, b, "/dst/"+n, "content of "+n)
	}
	h.assertQuiescent()
}

func TestRetryCanFailAgainAndBeRetriedAgain(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.fs["a"].addFile("/src/x", "x")
	h.fs["a"].failTimes("open", "/src/x", 2, errors.New("still down"))
	id := h.copyJob("a", []string{"/src/x"}, "b", "/dst", PolicyAsk)
	h.waitState(id, StateFailed)
	if err := h.e.Retry(id); err != nil {
		t.Fatal(err)
	}
	v := h.waitState(id, StateFailed)
	if v.FailedFiles != 1 || v.FinishedAt == nil {
		t.Fatalf("second failure = %+v", v)
	}
	if err := h.e.Retry(id); err != nil {
		t.Fatal(err)
	}
	h.waitState(id, StateDone)
	wantContent(t, h.fs["b"], "/dst/x", "x")
}

func TestRetryRewalksADirectoryWhoseListingFailed(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a, b := h.fs["a"], h.fs["b"]
	a.addFile("/src/top", "t").addFile("/src/sub/inner", "i").addDir("/src/sub/empty")
	a.failTimes("list", "/src/sub", 1, errors.New("permission denied"))
	id := h.copyJob("a", []string{"/src"}, "b", "/dst", PolicyAsk)
	v := h.waitFinal(id)
	if v.State != StateFailed || v.FailedFiles != 1 || v.Failed[0].Src != "/src/sub" {
		t.Fatalf("view = %+v", v)
	}
	if err := h.e.Retry(id); err != nil {
		t.Fatal(err)
	}
	v = h.waitState(id, StateDone)
	wantTree(t, b, "/dst", "/dst/src/", "/dst/src/top", "/dst/src/sub/", "/dst/src/sub/inner", "/dst/src/sub/empty/")
	if v.FailedFiles != 0 || v.DoneFiles != v.TotalFiles {
		t.Fatalf("after retry = %+v", v)
	}
}

func TestRetryRewalksAFailedMkdir(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.fs["a"].addFile("/src/sub/inner", "i")
	h.fs["b"].failTimes("mkdir", "/dst/src/sub", 1, errors.New("disk full"))
	id := h.copyJob("a", []string{"/src"}, "b", "/dst", PolicyAsk)
	v := h.waitFinal(id)
	if v.State != StateFailed || v.FailedFiles != 1 {
		t.Fatalf("view = %+v", v)
	}
	if err := h.e.Retry(id); err != nil {
		t.Fatal(err)
	}
	h.waitState(id, StateDone)
	wantContent(t, h.fs["b"], "/dst/src/sub/inner", "i")
}

func TestRetryAfterDestinationCouldNotBePrepared(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a, b := h.fs["a"], h.fs["b"]
	a.addFile("/s/one", "1").addFile("/s/two", "2")
	b.failTimes("list", "/dst", 1, errors.New("permission denied"))
	id := h.copyJob("a", []string{"/s/one", "/s/two"}, "b", "/dst", PolicyAsk)
	v := h.waitFinal(id)
	if v.State != StateFailed || v.FailedFiles != 2 || v.Error == "" {
		t.Fatalf("view = %+v", v)
	}
	if err := h.e.Retry(id); err != nil {
		t.Fatal(err)
	}
	v = h.waitState(id, StateDone)
	if v.TotalFiles != 2 || v.DoneFiles != 2 || v.FailedFiles != 0 || v.Error != "" {
		t.Fatalf("after retry = %+v", v)
	}
	wantContent(t, b, "/dst/two", "2")
}

func TestRetryRejectsJobsWithNothingToRetry(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.fs["a"]
	a.addFile("/ok", "ok")
	done := h.copyJob("a", []string{"/ok"}, "b", "/d", PolicyAsk)
	h.waitState(done, StateDone)
	if err := h.e.Retry(done); !errors.Is(err, ErrNotRetryable) {
		t.Fatalf("Retry(done) = %v", err)
	}
	if err := h.e.Retry("nope"); !errors.Is(err, ErrUnknownJob) {
		t.Fatalf("Retry(unknown) = %v", err)
	}

	bl := newBlocker("open")
	a.setHook(bl.hook)
	running := h.copyJob("a", []string{"/ok"}, "b", "/d2", PolicyAsk)
	bl.waitArrived(h, 1)
	if err := h.e.Retry(running); !errors.Is(err, ErrNotRetryable) {
		t.Fatalf("Retry(running) = %v", err)
	}
	if err := h.e.Cancel(running); err != nil {
		t.Fatal(err)
	}
	h.waitState(running, StateCanceled)
	if err := h.e.Retry(running); !errors.Is(err, ErrNotRetryable) {
		t.Fatalf("Retry(canceled) = %v", err)
	}
}

func TestRetryTwiceInARaceStartsOnlyOneRun(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.fs["a"].addFile("/x", "x")
	h.fs["a"].failTimes("open", "/x", 1, errors.New("flaky"))
	id := h.copyJob("a", []string{"/x"}, "b", "/d", PolicyAsk)
	h.waitState(id, StateFailed)
	bl := newBlocker("open")
	h.fs["a"].setHook(bl.hook)
	if err := h.e.Retry(id); err != nil {
		t.Fatal(err)
	}
	if err := h.e.Retry(id); !errors.Is(err, ErrNotRetryable) {
		t.Fatalf("second Retry = %v, want ErrNotRetryable", err)
	}
	bl.releaseAll()
	h.waitState(id, StateDone)
}

func TestRetryFailsWhenConnectionIsGone(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.fs["a"].addFile("/x", "x")
	h.fs["a"].failTimes("open", "/x", 1, errors.New("flaky"))
	id := h.copyJob("a", []string{"/x"}, "b", "/d", PolicyAsk)
	h.waitState(id, StateFailed)
	h.down.Store(true)
	if err := h.e.Retry(id); err == nil {
		t.Fatal("Retry succeeded without a connection")
	}
	if st := h.job(id).State; st != StateFailed {
		t.Fatalf("job state = %s after failed Retry", st)
	}
	h.down.Store(false)
	if err := h.e.Retry(id); err != nil {
		t.Fatalf("Retry after reconnect: %v", err)
	}
	h.waitState(id, StateDone)
}
