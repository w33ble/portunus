package transfer

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/w33ble/portunus/internal/prompt"
	"github.com/w33ble/portunus/internal/vfs"
)

func TestCancelDuringCommitDoesNotAbortTheCommit(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.fs["a"].addFile("/f", "data")
	bl := newBlocker("commit")
	h.fs["b"].setHook(bl.hook)
	id := h.copyJob("a", []string{"/f"}, "b", "/dst", PolicyAsk)
	bl.waitArrived(h, 1)
	if err := h.e.Cancel(id); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond) // the cancellation callback has run by now
	if st := h.job(id).State; st != StateRunning {
		t.Fatalf("job = %s while Commit is still in flight", st)
	}
	bl.releaseAll()
	v := h.waitState(id, StateCanceled)
	wantContent(t, h.fs["b"], "/dst/f", "data")
	commits, aborts, misuse := h.fs["b"].counters()
	if commits != 1 || aborts != 0 || len(misuse) != 0 || v.FailedFiles != 0 || v.DoneFiles != 1 {
		t.Fatalf("commits=%d aborts=%d misuse=%v view=%+v", commits, aborts, misuse, v)
	}
}

func TestCommitFailureFailsTheTaskAndIsNotFollowedByAbort(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.fs["a"].addFile("/x", "x")
	h.fs["b"].failOn("commit", "/dst/x", errors.New("rename failed"))
	v := h.waitFinal(h.copyJob("a", []string{"/x"}, "b", "/dst", PolicyAsk))
	if v.State != StateFailed || !strings.Contains(v.Failed[0].Error, "commit /dst/x") || !strings.Contains(v.Failed[0].Error, "rename failed") {
		t.Fatalf("view = %+v", v)
	}
	if h.fs["b"].exists("/dst/x") {
		t.Fatal("failed commit left a target")
	}
	h.assertQuiescent() // a stray Abort after the failed Commit would show up as misuse
}

func TestDeleteSourceThatCannotBeExaminedIsRetriable(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.fs["a"]
	deleteFixture(a)
	a.failTimes("stat", "/d", 1, errors.New("denied"))
	id := h.deleteJob("a", "/d")
	v := h.waitFinal(id)
	if v.State != StateFailed || v.FailedFiles != 1 || !a.exists("/d/a") {
		t.Fatalf("view = %+v", v)
	}
	if err := h.e.Retry(id); err != nil {
		t.Fatal(err)
	}
	v = h.waitState(id, StateDone)
	wantTree(t, a, "/")
	if v.FailedFiles != 0 || v.DoneFiles != v.TotalFiles {
		t.Fatalf("after retry = %+v", v)
	}
}

func TestCancelUnblocksAWalkerStalledOnAFullQueue(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(1, 1))
	addFiles(h.fs["a"], "/src", numbered("f", 300))
	bl := newBlocker("open")
	h.fs["a"].setHook(bl.hook)
	id := h.copyJob("a", []string{"/src"}, "b", "/dst", PolicyAsk)
	bl.waitArrived(h, 1)
	h.eventually("walker to fill the queue", func() bool { return h.job(id).TotalFiles > readyQueueCap })
	if err := h.e.Cancel(id); err != nil {
		t.Fatal(err)
	}
	v := h.waitState(id, StateCanceled)
	if v.Walking || len(v.Active) != 0 {
		t.Fatalf("view = %+v", v)
	}
	h.assertQuiescent()
}

func TestResolverReturningNoFilesystemIsRejected(t *testing.T) {
	t.Parallel()
	e := New(Options{Resolve: func(string) (vfs.FS, error) { return nil, nil }})
	t.Cleanup(e.Close)
	if _, err := e.Copy(CopyRequest{SrcConn: "a", Sources: []string{"/x"}, DstConn: "b", DstDir: "/d"}); err == nil {
		t.Fatal("Copy succeeded with a nil filesystem")
	}
	bare := New(Options{})
	t.Cleanup(bare.Close)
	if _, err := bare.Delete(DeleteRequest{Conn: "a", Paths: []string{"/x"}}); err == nil {
		t.Fatal("Delete succeeded without a resolver")
	}
}

func TestSkippingEveryConflictWithoutAnyTaskStartedReportsSkipped(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	names := conflictFixture(h, "/src", "/dst", 3)
	h.fs["b"].addDir("/dst")
	h.asker.set(func(context.Context, prompt.ConflictRequest) (prompt.ConflictAnswer, error) {
		return prompt.ConflictAnswer{Action: prompt.ConflictSkip, ApplyToAll: true}, nil
	})
	v := h.waitFinal(h.copyJob("a", joinPaths("/src", names), "b", "/dst", PolicyAsk))
	if v.State != StateSkipped || v.SkippedFiles != 3 || v.TotalBytes != 0 {
		t.Fatalf("view = %+v", v)
	}
}
