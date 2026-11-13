package transfer

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/w33ble/portunus/internal/vfs"
)

// wantBottomUp fails unless every path was removed after everything beneath it.
func wantBottomUp(t *testing.T, removed []string) {
	t.Helper()
	index := make(map[string]int, len(removed))
	for i, p := range removed {
		index[p] = i
	}
	for p, i := range index {
		for q, j := range index {
			if strings.HasPrefix(q, p+"/") && j > i {
				t.Fatalf("%s removed before its descendant %s", p, q)
			}
		}
	}
}

func deleteFixture(a *fakeFS) {
	a.addFile("/d/a", "a").addFile("/d/b", "b")
	a.addFile("/d/s1/c", "c").addFile("/d/s1/d", "d")
	a.addFile("/d/s1/s2/e", "e")
	a.addDir("/d/s3")
}

func TestDeleteRemovesTreeBottomUp(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.fs["a"]
	deleteFixture(a)
	a.addFile("/keep", "keep")
	v := h.waitFinal(h.deleteJob("a", "/d"))
	if v.State != StateDone || v.Kind != KindDelete {
		t.Fatalf("view = %+v", v)
	}
	wantTree(t, a, "/", "/keep")
	// 5 files + 4 directories (d, s1, s2, s3).
	if v.TotalFiles != 9 || v.DoneFiles != 9 || v.DstConn != "" || v.DstDir != "" {
		t.Fatalf("counters = %+v", v)
	}
	wantBottomUp(t, a.removedPaths())
}

func TestDeleteIsBottomUpWithManyWorkersAndInterleaving(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(4, 4))
	a := h.fs["a"]
	for d := range 12 {
		for s := range 3 {
			addFiles(a, fmt.Sprintf("/big/d%02d/s%d/deeper", d, s), numbered("f", 5))
			addFiles(a, fmt.Sprintf("/big/d%02d/s%d", d, s), numbered("g", 3))
		}
	}
	a.setLatency(50 * time.Microsecond)
	v := h.waitFinal(h.deleteJob("a", "/big"))
	if v.State != StateDone {
		t.Fatalf("view = %+v", v)
	}
	wantTree(t, a, "/")
	wantBottomUp(t, a.removedPaths())
	h.assertQuiescent()
}

func TestDeleteNeverFollowsSymlinks(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.fs["a"]
	a.addFile("/outside/keep", "keep")
	a.addFile("/d/real", "r")
	a.addLink("/d/to-dir", "/outside")
	a.addLink("/d/to-file", "/outside/keep")
	a.addLink("/d/dangling", "/nowhere")
	v := h.waitFinal(h.deleteJob("a", "/d"))
	if v.State != StateDone {
		t.Fatalf("view = %+v", v)
	}
	wantTree(t, a, "/", "/outside/", "/outside/keep")
	if v.TotalFiles != 5 { // real + three links + d
		t.Fatalf("total = %d", v.TotalFiles)
	}
}

func TestDeletingASymlinkToADirectoryRemovesOnlyTheLink(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.fs["a"]
	a.addFile("/outside/keep", "keep")
	a.addLink("/link", "/outside")
	h.waitState(h.deleteJob("a", "/link"), StateDone)
	wantTree(t, a, "/", "/outside/", "/outside/keep")
}

func TestDeleteTreatsAlreadyGoneAsDeleted(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.fs["a"]
	deleteFixture(a)
	a.failOn("remove", "/d/a", fmt.Errorf("gone: %w", vfs.ErrNotExist))
	v := h.waitFinal(h.deleteJob("a", "/d/a", "/missing"))
	if v.State != StateDone || v.FailedFiles != 0 || v.SkippedFiles != 1 {
		t.Fatalf("view = %+v", v)
	}
}

func TestDeleteOfOnlyMissingPathsIsSkipped(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.waitFinal(h.deleteJob("a", "/missing"))
	if v.State != StateSkipped {
		t.Fatalf("view = %+v", v)
	}
}

func TestDeleteFailureBlocksAncestorsAndRetryFinishesBottomUp(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.fs["a"]
	deleteFixture(a)
	a.failTimes("remove", "/d/s1/c", 1, errors.New("read-only"))
	id := h.deleteJob("a", "/d")
	v := h.waitFinal(id)
	// c fails; s1 and d cannot be removed with something left inside.
	if v.State != StateFailed || v.FailedFiles != 3 || v.DoneFiles != 6 {
		t.Fatalf("view = %+v", v)
	}
	failed := map[string]string{}
	for _, f := range v.Failed {
		failed[f.Src] = f.Error
	}
	if !strings.Contains(failed["/d/s1/c"], "read-only") || !strings.Contains(failed["/d/s1"], "not removed") || !strings.Contains(failed["/d"], "not removed") {
		t.Fatalf("failed = %v", failed)
	}
	if !a.exists("/d/s1/c") || !a.exists("/d/s1") {
		t.Fatal("ancestors of a failed delete were removed")
	}

	if err := h.e.Retry(id); err != nil {
		t.Fatal(err)
	}
	v = h.waitState(id, StateDone)
	if v.FailedFiles != 0 || v.DoneFiles != 9 {
		t.Fatalf("after retry = %+v", v)
	}
	wantTree(t, a, "/")
	wantBottomUp(t, a.removedPaths())
}

func TestDeleteListFailureIsRetriedByWalkingTheDirectoryAgain(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.fs["a"]
	deleteFixture(a)
	a.failTimes("list", "/d/s1", 1, errors.New("permission denied"))
	id := h.deleteJob("a", "/d")
	v := h.waitFinal(id)
	if v.State != StateFailed || !a.exists("/d/s1/c") {
		t.Fatalf("view = %+v", v)
	}
	if err := h.e.Retry(id); err != nil {
		t.Fatal(err)
	}
	h.waitState(id, StateDone)
	wantTree(t, a, "/")
	wantBottomUp(t, a.removedPaths())
}

func TestDeleteWithHostileEntryNameRemovesNothingOutside(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.fs["a"]
	a.addFile("/victim", "v").addFile("/d/ok", "ok")
	a.addEntry("/d", vfs.Entry{Name: "../victim", Path: "/d/../victim"})
	v := h.waitFinal(h.deleteJob("a", "/d"))
	if v.State != StateFailed || !a.exists("/victim") || !a.exists("/d") {
		t.Fatalf("view = %+v", v)
	}
	if err := h.e.Retry(v.ID); !errors.Is(err, ErrNotRetryable) {
		t.Fatalf("Retry = %v", err)
	}
}

func TestDeleteCanBeCanceledMidTree(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(1, 1))
	a := h.fs["a"]
	deleteFixture(a)
	bl := newBlocker("remove")
	a.setHook(bl.hook)
	id := h.deleteJob("a", "/d")
	bl.waitArrived(h, 1)
	if err := h.e.Cancel(id); err != nil {
		t.Fatal(err)
	}
	v := h.waitState(id, StateCanceled)
	if v.FailedFiles != 0 || len(v.Active) != 0 {
		t.Fatalf("view = %+v", v)
	}
	if !a.exists("/d") {
		t.Fatal("directory removed after cancel")
	}
}

func TestDeleteRequestValidation(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for name, req := range map[string]DeleteRequest{
		"no paths":       {Conn: "a"},
		"empty path":     {Conn: "a", Paths: []string{""}},
		"root":           {Conn: "a", Paths: []string{"/"}},
		"normalized":     {Conn: "a", Paths: []string{"/d/.."}},
		"trailing slash": {Conn: "a", Paths: []string{"/d/"}},
		"empty segment":  {Conn: "a", Paths: []string{"/a//d"}},
		"dot segment":    {Conn: "a", Paths: []string{"/a/./d"}},
		"dotdot inside":  {Conn: "a", Paths: []string{"/a/b/../d"}},
		"unknown conn":   {Conn: "zzz", Paths: []string{"/d"}},
	} {
		if _, err := h.e.Delete(req); err == nil {
			t.Errorf("%s: Delete succeeded", name)
		}
	}
	if n := len(h.e.Snapshot()); n != 0 {
		t.Fatalf("%d jobs created by rejected requests", n)
	}
}
