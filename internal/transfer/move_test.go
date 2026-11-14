package transfer

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/w33ble/portunus/internal/prompt"
	"github.com/w33ble/portunus/internal/vfs"
)

func (h *harness) moveJob(srcConn string, sources []string, dstConn, dstDir string, p Policy) string {
	h.t.Helper()
	id, err := h.e.Move(MoveRequest{SrcConn: srcConn, Sources: sources, DstConn: dstConn, DstDir: dstDir, Policy: p})
	if err != nil {
		h.t.Fatalf("Move: %v", err)
	}
	return id
}

func TestMoveOnOneConnectionRenamesWithoutCopyingData(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.fs["a"]
	a.addFile("/src/f.txt", "hello")
	a.addFile("/src/dir/x", "x").addFile("/src/dir/deep/y", "y")
	a.addDir("/dst")

	v := h.waitFinal(h.moveJob("a", []string{"/src/f.txt", "/src/dir"}, "a", "/dst", PolicyAsk))
	if v.State != StateDone || v.Kind != KindMove {
		t.Fatalf("view = %+v", v)
	}
	wantTree(t, a, "/", "/src/", "/dst/", "/dst/f.txt", "/dst/dir/", "/dst/dir/x", "/dst/dir/deep/", "/dst/dir/deep/y")
	wantContent(t, a, "/dst/f.txt", "hello")
	if got := a.renamedPaths(); !slices.Equal(got, []string{"/src/f.txt -> /dst/f.txt", "/src/dir -> /dst/dir"}) && !slices.Equal(got, []string{"/src/dir -> /dst/dir", "/src/f.txt -> /dst/f.txt"}) {
		t.Fatalf("renames = %q, want one per source", got)
	}
	commits, _, _ := a.counters()
	if commits != 0 || len(a.opens) != 0 || len(a.creates) != 0 {
		t.Fatalf("data was copied: commits=%d opens=%v creates=%v", commits, a.opens, a.creates)
	}
	// One task per source: the renamed file and the renamed directory.
	if v.TotalFiles != 2 || v.DoneFiles != 2 || v.DoneBytes != v.TotalBytes || v.TotalBytes != 5 {
		t.Fatalf("counters = %+v", v)
	}
	h.assertQuiescent()
}

func TestMoveFallsBackToCopyThenDeleteWhenRenameFails(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		failRenames []string
		wantRenamed []string
		wantOpened  []string
	}{
		{
			name:        "prefix cannot be renamed but its objects can",
			failRenames: []string{"/src/dir"},
			wantRenamed: []string{"/src/dir/deep -> /dst/dir/deep", "/src/dir/empty -> /dst/dir/empty", "/src/dir/x -> /dst/dir/x"},
		},
		{
			name:        "nothing can be renamed",
			failRenames: []string{"/src/dir", "/src/dir/x", "/src/dir/deep", "/src/dir/deep/y", "/src/dir/empty"},
			wantOpened:  []string{"/src/dir/deep/y", "/src/dir/x"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			a := h.fs["a"]
			a.addFile("/src/dir/x", "x").addFile("/src/dir/deep/y", "yy").addDir("/src/dir/empty")
			for _, p := range tc.failRenames {
				err := errors.New("invalid cross-device link")
				if p == "/src/dir" {
					err = vfs.ErrUnsupported
				}
				a.failOn("rename", p, err)
			}

			v := h.waitFinal(h.moveJob("a", []string{"/src/dir"}, "a", "/dst", PolicyAsk))
			if v.State != StateDone || v.FailedFiles != 0 {
				t.Fatalf("view = %+v", v)
			}
			wantTree(t, a, "/", "/src/", "/dst/", "/dst/dir/", "/dst/dir/x", "/dst/dir/deep/", "/dst/dir/deep/y", "/dst/dir/empty/")
			wantContent(t, a, "/dst/dir/deep/y", "yy")
			renamed := a.renamedPaths()
			slices.Sort(renamed)
			if !slices.Equal(renamed, tc.wantRenamed) {
				t.Fatalf("renames = %q, want %q", renamed, tc.wantRenamed)
			}
			for _, p := range tc.wantOpened {
				if a.count(a.opens, p) != 1 {
					t.Fatalf("%s opened %d times, want 1", p, a.count(a.opens, p))
				}
			}
			wantBottomUp(t, a.removedPaths())
			h.assertQuiescent()
		})
	}
}

func TestMoveAcrossConnectionsCopiesThenDeletesTheTree(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a, b := h.fs["a"], h.fs["b"]
	a.addFile("/src/top.txt", "top").addFile("/src/sub/one.txt", "one").addFile("/src/sub/deep/two.txt", "twotwo")
	a.addDir("/src/empty")
	a.addFile("/keep", "keep")
	b.addDir("/dst")

	v := h.waitFinal(h.moveJob("a", []string{"/src"}, "b", "/dst", PolicyAsk))
	if v.State != StateDone || v.FailedFiles != 0 || v.SkippedFiles != 0 {
		t.Fatalf("view = %+v", v)
	}
	wantTree(t, a, "/", "/keep")
	wantTree(t, b, "/dst", "/dst/src/", "/dst/src/top.txt", "/dst/src/empty/", "/dst/src/sub/",
		"/dst/src/sub/one.txt", "/dst/src/sub/deep/", "/dst/src/sub/deep/two.txt")
	wantContent(t, b, "/dst/src/sub/deep/two.txt", "twotwo")
	// 3 files + 5 directories (src, sub, deep, empty and the top-level src).
	if v.TotalFiles != 7 || v.DoneFiles != 7 || v.DoneBytes != v.TotalBytes || v.TotalBytes != 12 {
		t.Fatalf("counters = %+v", v)
	}
	wantBottomUp(t, a.removedPaths())
	if len(a.renamedPaths()) != 0 {
		t.Fatalf("renamed across connections: %q", a.renamedPaths())
	}
	h.assertQuiescent()
}

func TestMoveNeverDeletesAFileWhoseCopyDidNotCommit(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a, b := h.fs["a"], h.fs["b"]
	a.addFile("/src/ok", "ok").addFile("/src/bad", "bad")
	b.addDir("/dst")
	b.failOn("commit", "/dst/bad", errors.New("disk full"))

	v := h.waitFinal(h.moveJob("a", []string{"/src/ok", "/src/bad"}, "b", "/dst", PolicyAsk))
	if v.State != StateFailed || v.FailedFiles != 1 {
		t.Fatalf("view = %+v", v)
	}
	wantTree(t, a, "/", "/src/", "/src/bad")
	wantContent(t, a, "/src/bad", "bad")
	wantTree(t, b, "/dst", "/dst/ok")
	h.assertQuiescent()
}

func TestMoveReportsFailureAndKeepsTheSourceWhenOnlyTheDeleteFails(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a, b := h.fs["a"], h.fs["b"]
	a.addFile("/src/d/f", "payload").addFile("/src/d/g", "g")
	b.addDir("/dst")
	a.failTimes("remove", "/src/d/f", 1, errors.New("permission denied"))

	id := h.moveJob("a", []string{"/src/d"}, "b", "/dst", PolicyAsk)
	v := h.waitFinal(id)
	if v.State != StateFailed || v.FailedFiles != 2 { // the file, then its directory
		t.Fatalf("view = %+v", v)
	}
	if !strings.Contains(v.Failed[0].Error, "removing the source failed") {
		t.Fatalf("failure = %+v", v.Failed)
	}
	wantContent(t, a, "/src/d/f", "payload")
	wantContent(t, b, "/dst/d/f", "payload")
	wantTree(t, a, "/src", "/src/d/", "/src/d/f")

	if err := h.e.Retry(id); err != nil {
		t.Fatal(err)
	}
	v = h.waitState(id, StateDone)
	if v.FailedFiles != 0 {
		t.Fatalf("retried view = %+v", v)
	}
	wantTree(t, a, "/", "/src/")
	wantContent(t, b, "/dst/d/f", "payload")
	wantContent(t, b, "/dst/d/g", "g")
	h.assertQuiescent()
}

func TestMoveSkippedConflictStaysAtTheSource(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		policy Policy
		answer prompt.ConflictAction
		dst    string
	}{
		{"skip policy", PolicySkip, "", "b"},
		{"asked and skipped", PolicyAsk, prompt.ConflictSkip, "b"},
		{"skip policy on one connection", PolicySkip, "", "a"},
		{"asked and skipped on one connection", PolicyAsk, prompt.ConflictSkip, "a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.asker.set(func(_ context.Context, _ prompt.ConflictRequest) (prompt.ConflictAnswer, error) {
				return prompt.ConflictAnswer{Action: tc.answer}, nil
			})
			a := h.fs["a"]
			dst := h.fs[tc.dst]
			a.addFile("/src/f", "new")
			dst.addFile("/dst/f", "old")

			v := h.waitFinal(h.moveJob("a", []string{"/src/f"}, tc.dst, "/dst", tc.policy))
			if v.State != StateSkipped || v.SkippedFiles != 1 || v.DoneFiles != 0 {
				t.Fatalf("view = %+v", v)
			}
			wantContent(t, a, "/src/f", "new")
			wantContent(t, dst, "/dst/f", "old")
		})
	}
}

func TestMoveOverwriteReplacesTheTargetThenDeletesTheSource(t *testing.T) {
	t.Parallel()
	for _, dstConn := range []string{"a", "b"} {
		t.Run("to "+dstConn, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.asker.set(func(_ context.Context, _ prompt.ConflictRequest) (prompt.ConflictAnswer, error) {
				return prompt.ConflictAnswer{Action: prompt.ConflictOverwrite}, nil
			})
			a, dst := h.fs["a"], h.fs[dstConn]
			a.addFile("/src/f", "new")
			dst.addFile("/dst/f", "old")

			v := h.waitFinal(h.moveJob("a", []string{"/src/f"}, dstConn, "/dst", PolicyAsk))
			if v.State != StateDone {
				t.Fatalf("view = %+v", v)
			}
			wantContent(t, dst, "/dst/f", "new")
			if a.exists("/src/f") {
				t.Fatal("source survived an overwriting move")
			}
			if h.asker.callCount() != 1 {
				t.Fatalf("asked %d times", h.asker.callCount())
			}
			// Replacing is a copy: Rename refuses an existing target, and removing the old file
			// first would lose it if the new one then failed to arrive.
			if len(a.renamedPaths()) != 0 {
				t.Fatalf("renamed over an existing file: %q", a.renamedPaths())
			}
		})
	}
}

func TestMoveKeepsADirectoryAtTheSourceWhileAChildStays(t *testing.T) {
	t.Parallel()
	for _, dstConn := range []string{"a", "b"} {
		t.Run("to "+dstConn, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			a, dst := h.fs["a"], h.fs[dstConn]
			a.addFile("/src/d/keep", "src-keep").addFile("/src/d/go", "go").addFile("/src/d/inner/also", "also")
			a.addFile("/src/d/inner/stay", "src-stay")
			dst.addFile("/dst/d/keep", "dst-keep").addFile("/dst/d/inner/stay", "dst-stay")

			v := h.waitFinal(h.moveJob("a", []string{"/src/d"}, dstConn, "/dst", PolicySkip))
			if v.State != StateDone || v.SkippedFiles != 4 || v.FailedFiles != 0 { // keep, stay, and both directories that hold them
				t.Fatalf("view = %+v", v)
			}
			wantTree(t, a, "/src", "/src/d/", "/src/d/keep", "/src/d/inner/", "/src/d/inner/stay")
			wantContent(t, a, "/src/d/keep", "src-keep")
			wantContent(t, dst, "/dst/d/keep", "dst-keep")
			wantContent(t, dst, "/dst/d/go", "go")
			wantContent(t, dst, "/dst/d/inner/also", "also")
			wantContent(t, dst, "/dst/d/inner/stay", "dst-stay")
			h.assertQuiescent()
		})
	}
}

func TestMoveMergesIntoAnExistingDirectoryByRenamingWhatIsFree(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.fs["a"]
	a.addFile("/src/d/new", "new").addFile("/src/d/sub/n", "n")
	a.addFile("/dst/d/old", "old")

	v := h.waitFinal(h.moveJob("a", []string{"/src/d"}, "a", "/dst", PolicyAsk))
	if v.State != StateDone {
		t.Fatalf("view = %+v", v)
	}
	wantTree(t, a, "/", "/src/", "/dst/", "/dst/d/", "/dst/d/old", "/dst/d/new", "/dst/d/sub/", "/dst/d/sub/n")
	renamed := a.renamedPaths()
	slices.Sort(renamed)
	if want := []string{"/src/d/new -> /dst/d/new", "/src/d/sub -> /dst/d/sub"}; !slices.Equal(renamed, want) {
		t.Fatalf("renames = %q, want %q", renamed, want)
	}
	if len(a.opens) != 0 {
		t.Fatalf("data was copied: %v", a.opens)
	}
}

func TestMoveCancelMidwayLosesNothingAndResumeFinishes(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(1, 1))
	a, b := h.fs["a"], h.fs["b"]
	big := strings.Repeat("x", 10_000)
	a.addFile("/src/d/a-small", "small").addFile("/src/d/z-big", big).addFile("/src/later", "later")
	a.configure(func(f *fakeFS) { f.chunk = 1000 })
	b.addDir("/dst")
	bl := newBlocker("read")
	a.setHook(onPath(bl, "/src/d/z-big"))
	bl.release(2)

	id := h.moveJob("a", []string{"/src/d", "/src/later"}, "b", "/dst", PolicyAsk)
	bl.waitArrived(h, 3)
	v := h.cancelAndWait(id)
	if v.FailedFiles != 0 {
		t.Fatalf("view = %+v", v)
	}
	// Every item is at the source or the destination; the half-copied one only at the source.
	wantContent(t, a, "/src/d/z-big", big)
	wantTree(t, a, "/src", "/src/d/", "/src/d/z-big")
	wantTree(t, b, "/dst", "/dst/later", "/dst/d/", "/dst/d/a-small")
	wantContent(t, b, "/dst/d/a-small", "small")
	h.assertQuiescent()

	bl.releaseAll()
	h.resume(id)
	v = h.waitState(id, StateDone)
	if v.FailedFiles != 0 || v.SkippedFiles != 0 {
		t.Fatalf("resumed view = %+v", v)
	}
	wantTree(t, a, "/", "/src/")
	wantContent(t, b, "/dst/d/z-big", big)
	wantContent(t, b, "/dst/d/a-small", "small")
	wantContent(t, b, "/dst/later", "later")
	h.assertQuiescent()
}

func TestResumeOfAMoveSkipsSourcesThatAlreadyArrived(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(1, 1))
	a, b := h.fs["a"], h.fs["b"]
	a.addFile("/s/one", "1").addFile("/s/two", "22")
	b.addDir("/d")
	bl := newBlocker("open")
	a.setHook(onPath(bl, "/s/two"))
	id := h.moveJob("a", []string{"/s/one", "/s/two"}, "b", "/d", PolicyAsk)
	bl.waitArrived(h, 1)
	h.cancelAndWait(id)
	if a.exists("/s/one") || !a.exists("/s/two") {
		t.Fatalf("source after cancel: %q", a.tree("/s"))
	}

	bl.releaseAll()
	h.resume(id)
	v := h.waitState(id, StateDone)
	if v.FailedFiles != 0 || v.DoneFiles != 2 || v.TotalFiles != 2 {
		t.Fatalf("resumed view = %+v", v)
	}
	wantTree(t, a, "/s")
	wantTree(t, b, "/d", "/d/one", "/d/two")
}

func TestCancelWhileDeletingACommittedSourceStillDeletesIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a, b := h.fs["a"], h.fs["b"]
	a.addFile("/src/f", "data")
	b.addDir("/dst")
	bl := newBlocker("remove")
	a.setHook(bl.hook)

	id := h.moveJob("a", []string{"/src/f"}, "b", "/dst", PolicyAsk)
	bl.waitArrived(h, 1)
	if err := h.e.Cancel(id); err != nil {
		t.Fatal(err)
	}
	bl.releaseAll()
	h.waitState(id, StateCanceled)
	// The copy committed, so leaving the source behind would put the file in both places.
	wantContent(t, b, "/dst/f", "data")
	if a.exists("/src/f") {
		t.Fatal("source of a committed copy survived the cancel")
	}
}

func TestMoveRejectsTargetsInsideTheSource(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for _, tc := range []struct {
		name string
		req  MoveRequest
		want error
	}{
		{"dir into itself", MoveRequest{SrcConn: "a", Sources: []string{"/a/b"}, DstConn: "a", DstDir: "/a/b"}, ErrIntoSelf},
		{"dir into its subdirectory", MoveRequest{SrcConn: "a", Sources: []string{"/a/b"}, DstConn: "a", DstDir: "/a/b/c"}, ErrIntoSelf},
		{"file onto itself", MoveRequest{SrcConn: "a", Sources: []string{"/a/f"}, DstConn: "a", DstDir: "/a"}, ErrOntoSelf},
		{"unclean source", MoveRequest{SrcConn: "a", Sources: []string{"/a/../f"}, DstConn: "b", DstDir: "/d"}, ErrInvalid},
		{"root source", MoveRequest{SrcConn: "a", Sources: []string{"/"}, DstConn: "b", DstDir: "/d"}, ErrInvalid},
		{"no sources", MoveRequest{SrcConn: "a", DstConn: "b", DstDir: "/d"}, ErrNoSources},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := h.e.Move(tc.req); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
	// Different connections may use the same paths.
	h.fs["a"].addFile("/a/b/x", "x")
	h.waitFinal(h.moveJob("a", []string{"/a/b"}, "b", "/a/b", PolicyAsk))
}
