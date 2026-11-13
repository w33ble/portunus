package transfer

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestCopyRequestValidation(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for _, tc := range []struct {
		name string
		req  CopyRequest
		want error // nil: any error
	}{
		{"no sources", CopyRequest{SrcConn: "a", DstConn: "b", DstDir: "/d"}, ErrNoSources},
		{"empty source path", CopyRequest{SrcConn: "a", Sources: []string{""}, DstConn: "b", DstDir: "/d"}, ErrInvalid},
		{"root source", CopyRequest{SrcConn: "a", Sources: []string{"/"}, DstConn: "b", DstDir: "/d"}, ErrInvalid},
		{"empty destination", CopyRequest{SrcConn: "a", Sources: []string{"/x"}, DstConn: "b"}, ErrInvalid},
		{"unknown policy", CopyRequest{SrcConn: "a", Sources: []string{"/x"}, DstConn: "b", DstDir: "/d", Policy: "maybe"}, ErrInvalid},
		{"dir into itself", CopyRequest{SrcConn: "a", Sources: []string{"/a/b"}, DstConn: "a", DstDir: "/a/b"}, ErrIntoSelf},
		{"dir into its subdirectory", CopyRequest{SrcConn: "a", Sources: []string{"/a/b"}, DstConn: "a", DstDir: "/a/b/c/d"}, ErrIntoSelf},
		{"file onto itself", CopyRequest{SrcConn: "a", Sources: []string{"/a/f"}, DstConn: "a", DstDir: "/a"}, ErrOntoSelf},
		{"dir onto itself", CopyRequest{SrcConn: "a", Sources: []string{"/a/b"}, DstConn: "a", DstDir: "/a"}, ErrOntoSelf},
		{"unclean source: dotdot", CopyRequest{SrcConn: "a", Sources: []string{"/a/../b"}, DstConn: "b", DstDir: "/d"}, ErrInvalid},
		{"unclean source: trailing slash", CopyRequest{SrcConn: "a", Sources: []string{"/a/b/"}, DstConn: "b", DstDir: "/d"}, ErrInvalid},
		{"unclean source: empty segment", CopyRequest{SrcConn: "a", Sources: []string{"/a//b"}, DstConn: "b", DstDir: "/d"}, ErrInvalid},
		{"unclean source: dot", CopyRequest{SrcConn: "a", Sources: []string{"/a/./b"}, DstConn: "b", DstDir: "/d"}, ErrInvalid},
		{"relative-looking dotdot source", CopyRequest{SrcConn: "a", Sources: []string{"/.."}, DstConn: "b", DstDir: "/d"}, ErrInvalid},
		{"unclean destination", CopyRequest{SrcConn: "a", Sources: []string{"/x"}, DstConn: "b", DstDir: "/a/x/../b"}, ErrInvalid},
		{"one bad source among good", CopyRequest{SrcConn: "a", Sources: []string{"/ok", "/a/b"}, DstConn: "a", DstDir: "/a/b/z"}, ErrIntoSelf},
		{"unknown source connection", CopyRequest{SrcConn: "nope", Sources: []string{"/x"}, DstConn: "b", DstDir: "/d"}, nil},
		{"unknown destination connection", CopyRequest{SrcConn: "a", Sources: []string{"/x"}, DstConn: "nope", DstDir: "/d"}, nil},
	} {
		_, err := h.e.Copy(tc.req)
		if err == nil || (tc.want != nil && !errors.Is(err, tc.want)) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
	if n := len(h.e.Snapshot()); n != 0 {
		t.Fatalf("%d jobs created by rejected requests", n)
	}
}

func TestCopyRequestAcceptsLookalikePaths(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.fs["a"].addFile("/a/b/x", "x").addDir("/a/bc")
	// /a/bc merely shares a string prefix with /a/b; and the same paths on another connection are unrelated.
	h.waitFinal(h.copyJob("a", []string{"/a/b"}, "a", "/a/bc", PolicyAsk))
	wantContent(t, h.fs["a"], "/a/bc/b/x", "x")
	h.fs["b"].addDir("/a/b/c")
	h.waitFinal(h.copyJob("a", []string{"/a/b"}, "b", "/a/b/c", PolicyAsk))
	wantContent(t, h.fs["b"], "/a/b/c/b/x", "x")
}

func TestClearDropsOnlyFinishedJobsAndReportsThem(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.fs["a"]
	a.addFile("/ok", "ok").addFile("/slow", "slow")
	done := h.copyJob("a", []string{"/ok"}, "b", "/d", PolicyAsk)
	h.waitState(done, StateDone)
	failing := h.copyJob("a", []string{"/missing"}, "b", "/d", PolicyAsk)
	h.waitState(failing, StateFailed)
	bl := newBlocker("open")
	a.setHook(bl.hook)
	running := h.copyJob("a", []string{"/slow"}, "b", "/d2", PolicyAsk)
	bl.waitArrived(h, 1)

	h.e.Clear()
	snap := h.e.Snapshot()
	if len(snap) != 1 || snap[0].ID != running {
		t.Fatalf("snapshot after Clear = %+v", snap)
	}
	var removed []string
	h.eventually("Removed to be emitted", func() bool {
		removed = nil
		for _, u := range h.allUpdates() {
			removed = append(removed, u.Removed...)
		}
		return len(removed) == 2
	})
	slices.Sort(removed)
	if want := []string{done, failing}; !slices.Equal(removed, want) {
		t.Fatalf("removed = %v, want %v", removed, want)
	}
	if err := h.e.Retry(failing); !errors.Is(err, ErrUnknownJob) {
		t.Fatalf("Retry(cleared) = %v", err)
	}
	bl.releaseAll()
	h.waitState(running, StateDone)
	h.e.Clear()
	if n := len(h.e.Snapshot()); n != 0 {
		t.Fatalf("%d jobs left", n)
	}
}

func TestCloseCancelsEverythingWaitsAndRejectsNewWork(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(2, 2))
	a := h.fs["a"]
	a.addFile("/one", "1").addFile("/two", "2").addFile("/three", "3").addFile("/walk/x", "x")
	bl := newBlocker("open")
	a.setHook(bl.hook)
	ids := []string{
		h.copyJob("a", []string{"/one", "/two", "/three"}, "b", "/d", PolicyAsk),
		h.copyJob("a", []string{"/walk"}, "b", "/d2", PolicyAsk),
	}
	bl.waitArrived(h, 2)

	closed := make(chan struct{})
	go func() { h.e.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(waitTimeout):
		t.Fatal("Close did not return")
	}
	for _, id := range ids {
		if v := h.job(id); v.State != StateCanceled {
			t.Errorf("job %s = %s after Close", id, v.State)
		}
	}
	h.assertQuiescent()
	if _, err := h.e.Copy(CopyRequest{SrcConn: "a", Sources: []string{"/one"}, DstConn: "b", DstDir: "/d"}); !errors.Is(err, ErrClosed) {
		t.Fatalf("Copy after Close = %v", err)
	}
	if _, err := h.e.Delete(DeleteRequest{Conn: "a", Paths: []string{"/one"}}); !errors.Is(err, ErrClosed) {
		t.Fatalf("Delete after Close = %v", err)
	}
	h.e.Close() // idempotent
	h.e.SetLimits(Limits{Global: 9, PerConn: 9})

	last := h.allUpdates()
	final := map[string]State{}
	for _, u := range last {
		for _, v := range u.Jobs {
			final[v.ID] = v.State
		}
	}
	for _, id := range ids {
		if final[id] != StateCanceled {
			t.Errorf("last emitted state of %s = %s, want canceled (Close must flush)", id, final[id])
		}
	}
}

func TestUpdatesAndViewsEncodeToTheDocumentedJSON(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withTick(time.Hour))
	h.fs["a"].addFile("/f", "f")
	id := h.copyJob("a", []string{"/f"}, "b", "/d", PolicyAsk)
	h.waitState(id, StateDone)
	h.eventually("final update", func() bool { return len(h.allUpdates()) > 0 })

	raw, err := json.Marshal(h.job(id))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"id", "kind", "state", "srcConn", "dstConn", "dstDir", "title", "walking",
		"totalFiles", "doneFiles", "failedFiles", "skippedFiles", "totalBytes", "doneBytes",
		"bytesPerSec", "etaSeconds", "active", "failed", "createdAt", "finishedAt"} {
		if _, ok := m[key]; !ok {
			t.Errorf("JobView JSON lacks %q: %s", key, raw)
		}
	}
	if _, ok := m["error"]; ok {
		t.Error("empty error should be omitted")
	}
	if !strings.Contains(string(raw), `"active":[]`) || !strings.Contains(string(raw), `"failed":[]`) {
		t.Errorf("empty task lists must encode as [], not null: %s", raw)
	}
	for _, u := range h.allUpdates() {
		b, _ := json.Marshal(u)
		if strings.Contains(string(b), "null") {
			t.Fatalf("update encodes null: %s", b)
		}
	}
	if b, _ := json.Marshal(CopyRequest{SrcConn: "a", Sources: []string{"/x"}, DstConn: "b", DstDir: "/d", Policy: PolicySkip}); string(b) !=
		`{"srcConn":"a","sources":["/x"],"dstConn":"b","dstDir":"/d","policy":"skip"}` {
		t.Fatalf("CopyRequest JSON = %s", b)
	}
	if b, _ := json.Marshal(Limits{Global: 4, PerConn: 2}); string(b) != `{"global":4,"perConn":2}` {
		t.Fatalf("Limits JSON = %s", b)
	}
}
