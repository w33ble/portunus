package transfer

import (
	"context"
	"errors"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/w33ble/portunus/internal/vfs"
)

func wantTree(t *testing.T, f *fakeFS, dir string, want ...string) {
	t.Helper()
	got := f.tree(dir)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("tree %s:\n got  %q\n want %q", dir, got, want)
	}
}

func wantContent(t *testing.T, f *fakeFS, p, want string) {
	t.Helper()
	got, ok := f.content(p)
	if !ok || got != want {
		t.Fatalf("content of %s = %q (exists %v), want %q", p, got, ok, want)
	}
}

func TestCopyTreeRecreatesEverythingIncludingEmptyDirs(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.fs["a"]
	a.addFile("/src/top.txt", "top")
	a.addFile("/src/sub/one.txt", "one")
	a.addFile("/src/sub/deep/two.txt", "twotwo")
	a.addFile("/src/zero.txt", "")
	a.addDir("/src/empty")
	a.addDir("/src/sub/emptydeep")
	h.fs["b"].addDir("/dst")

	v := h.waitFinal(h.copyJob("a", []string{"/src"}, "b", "/dst", PolicyAsk))
	if v.State != StateDone {
		t.Fatalf("state = %s (failed %+v, err %q)", v.State, v.Failed, v.Error)
	}
	wantTree(t, h.fs["b"], "/dst",
		"/dst/src/", "/dst/src/top.txt", "/dst/src/zero.txt", "/dst/src/empty/",
		"/dst/src/sub/", "/dst/src/sub/one.txt", "/dst/src/sub/deep/", "/dst/src/sub/deep/two.txt",
		"/dst/src/sub/emptydeep/")
	wantContent(t, h.fs["b"], "/dst/src/sub/deep/two.txt", "twotwo")
	wantContent(t, h.fs["b"], "/dst/src/zero.txt", "")
	// 4 files + 5 directories (src, sub, deep, empty, emptydeep).
	if v.TotalFiles != 9 || v.DoneFiles != 9 || v.FailedFiles != 0 || v.SkippedFiles != 0 {
		t.Fatalf("counters = %+v", v)
	}
	if v.TotalBytes != 3+3+6 || v.DoneBytes != v.TotalBytes {
		t.Fatalf("bytes done/total = %d/%d", v.DoneBytes, v.TotalBytes)
	}
	if v.Walking || v.FinishedAt == nil || len(v.Active) != 0 || v.Title != "src/" {
		t.Fatalf("view = %+v", v)
	}
	h.assertQuiescent()
}

func TestCopyCreatesMissingDestinationDir(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.fs["a"].addFile("/src/x.txt", "x")
	v := h.waitFinal(h.copyJob("a", []string{"/src/x.txt"}, "b", "/new/nested", PolicyAsk))
	if v.State != StateDone {
		t.Fatalf("state = %s %+v", v.State, v.Failed)
	}
	wantContent(t, h.fs["b"], "/new/nested/x.txt", "x")
}

func TestCopyPreservesModeAndModTimeWithoutSpecialBits(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.fs["a"]
	a.addFile("/src/run.sh", "#!/bin/sh")
	a.setMode("/src/run.sh", fs.ModeSetuid|0o755)
	h.waitFinal(h.copyJob("a", []string{"/src/run.sh"}, "b", "/dst", PolicyAsk))
	got := h.fs["b"].node("/dst/run.sh")
	if got.mode != 0o755 {
		t.Fatalf("mode = %v, want 0755 with setuid stripped", got.mode)
	}
	if want := a.node("/src/run.sh").mtime; !got.mtime.Equal(want) {
		t.Fatalf("mtime = %v, want %v", got.mtime, want)
	}
}

func TestTitleNamesFirstSourceAndCountsTheRest(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.fs["a"]
	a.addFile("/s/x", "x")
	a.addFile("/s/y", "y")
	a.addFile("/s/z", "z")
	v := h.waitFinal(h.copyJob("a", joinPaths("/s", []string{"x", "y", "z"}), "b", "/d", PolicyAsk))
	if v.Title != "x and 2 more" {
		t.Fatalf("title = %q", v.Title)
	}
}

func TestWalkIsLazyAndCopiesWhileStillExpanding(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.fs["a"]
	a.addFile("/src/first.txt", "first")
	a.addFile("/src/slow/later.txt", "later")
	b := newBlocker("list")
	a.setHook(func(ctx context.Context, op, p string) error {
		if p == "/src/slow" {
			return b.hook(ctx, op, p)
		}
		return nil
	})
	id := h.copyJob("a", []string{"/src"}, "b", "/dst", PolicyAsk)
	h.eventually("first file copied while walk blocked", func() bool {
		_, ok := h.fs["b"].content("/dst/src/first.txt")
		return ok
	})
	v := h.job(id)
	if !v.Walking || v.State != StateRunning || v.ETASeconds != -1 {
		t.Fatalf("mid-walk view: walking=%v state=%s eta=%d", v.Walking, v.State, v.ETASeconds)
	}
	b.releaseAll()
	v = h.waitFinal(id)
	if v.State != StateDone || v.Walking {
		t.Fatalf("final view = %+v", v)
	}
	wantContent(t, h.fs["b"], "/dst/src/slow/later.txt", "later")
}

func TestWalkQueueIsBounded(t *testing.T) {
	t.Parallel()
	h := newHarness(t, withLimits(1, 1))
	a := h.fs["a"]
	names := numbered("f", 400)
	addFiles(a, "/src", names)
	b := newBlocker("open")
	a.setHook(b.hook)
	id := h.copyJob("a", []string{"/src"}, "b", "/dst", PolicyAsk)
	b.waitArrived(h, 1)
	h.eventually("queue to fill", func() bool { return h.job(id).TotalFiles >= readyQueueCap })
	time.Sleep(40 * time.Millisecond)
	v := h.job(id)
	if v.TotalFiles > readyQueueCap+8 || !v.Walking {
		t.Fatalf("walker ran ahead: total=%d walking=%v (queue cap %d)", v.TotalFiles, v.Walking, readyQueueCap)
	}
	b.releaseAll()
	v = h.waitFinal(id)
	if v.State != StateDone || v.DoneFiles != 401 { // 400 files + /src
		t.Fatalf("final = %+v", v)
	}
}

func TestSymlinksFollowFilesButNeverLinkedDirs(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.fs["a"]
	a.addFile("/src/file.txt", "real")
	a.addLink("/src/to-file", "/src/file.txt")
	a.addFile("/src/sub/inner.txt", "inner")
	a.addLink("/src/to-dir", "/src/sub")
	a.addLink("/src/sub/loop", "/src") // loop back to an ancestor
	v := h.waitFinal(h.copyJob("a", []string{"/src"}, "b", "/dst", PolicyAsk))
	if v.State != StateDone {
		t.Fatalf("state = %s %+v", v.State, v.Failed)
	}
	wantTree(t, h.fs["b"], "/dst",
		"/dst/src/", "/dst/src/file.txt", "/dst/src/to-file", "/dst/src/sub/", "/dst/src/sub/inner.txt")
	wantContent(t, h.fs["b"], "/dst/src/to-file", "real")
	if v.SkippedFiles != 2 {
		t.Fatalf("skipped = %d, want the 2 linked dirs", v.SkippedFiles)
	}
}

func TestTopLevelSymlinkToDirIsSkipped(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.fs["a"]
	a.addFile("/real/x.txt", "x")
	a.addLink("/link", "/real")
	v := h.waitFinal(h.copyJob("a", []string{"/link"}, "b", "/dst", PolicyAsk))
	if v.State != StateSkipped || v.SkippedFiles != 1 {
		t.Fatalf("view = %+v", v)
	}
	wantTree(t, h.fs["b"], "/dst")
}

func TestDanglingSymlinkFailsItsTaskOnly(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.fs["a"]
	a.addFile("/src/ok.txt", "ok")
	a.addLink("/src/dangling", "/nowhere")
	v := h.waitFinal(h.copyJob("a", []string{"/src"}, "b", "/dst", PolicyAsk))
	if v.State != StateFailed || v.FailedFiles != 1 || v.Failed[0].Src != "/src/dangling" {
		t.Fatalf("view = %+v", v)
	}
	wantContent(t, h.fs["b"], "/dst/src/ok.txt", "ok")
}

func TestSpecialFilesAreSkippedNotOpened(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.fs["a"]
	a.addFile("/src/ok.txt", "ok")
	a.addEntry("/src", vfs.Entry{Name: "fifo", Path: "/src/fifo", Mode: fs.ModeNamedPipe | 0o644})
	v := h.waitFinal(h.copyJob("a", []string{"/src"}, "b", "/dst", PolicyAsk))
	if v.State != StateDone || v.SkippedFiles != 1 {
		t.Fatalf("view = %+v", v)
	}
	if n := a.count(a.opens, "/src/fifo"); n != 0 {
		t.Fatalf("fifo opened %d times", n)
	}
}

func TestHostileEntryNamesCannotEscapeTheDestination(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.fs["a"]
	a.addFile("/src/ok.txt", "ok")
	a.addEntry("/src", vfs.Entry{Name: "../../evil", Path: "/src/../../evil", Size: 3})
	a.addEntry("/src", vfs.Entry{Name: "a/b", Path: "/src/a/b", Size: 3})
	v := h.waitFinal(h.copyJob("a", []string{"/src"}, "b", "/dst", PolicyAsk))
	if v.State != StateFailed || v.FailedFiles != 2 {
		t.Fatalf("view = %+v", v)
	}
	wantTree(t, h.fs["b"], "/", "/dst/", "/dst/src/", "/dst/src/ok.txt")
	if err := h.e.Retry(v.ID); !errors.Is(err, ErrNotRetryable) {
		t.Fatalf("Retry of unretryable failures = %v, want ErrNotRetryable", err)
	}
}

func TestSameNamedSourcesCannotOverwriteEachOther(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.fs["a"]
	a.addFile("/one/x", "1")
	a.addFile("/two/x", "2")
	v := h.waitFinal(h.copyJob("a", []string{"/one/x", "/two/x"}, "b", "/dst", PolicyOverwrite))
	if v.State != StateFailed || v.FailedFiles != 1 || v.DoneFiles != 1 {
		t.Fatalf("view = %+v", v)
	}
	wantContent(t, h.fs["b"], "/dst/x", "1")
	if err := h.e.Retry(v.ID); !errors.Is(err, ErrNotRetryable) {
		t.Fatalf("Retry of a duplicate-name failure = %v, want ErrNotRetryable", err)
	}
}

func TestMissingSourceFailsOnlyThatSource(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.fs["a"].addFile("/s/there", "t")
	v := h.waitFinal(h.copyJob("a", []string{"/s/missing", "/s/there"}, "b", "/dst", PolicyAsk))
	if v.State != StateFailed || v.FailedFiles != 1 || v.DoneFiles != 1 {
		t.Fatalf("view = %+v", v)
	}
	if got := v.Failed[0]; got.Src != "/s/missing" || !strings.Contains(got.Error, "does not exist") {
		t.Fatalf("failed task = %+v", got)
	}
	wantContent(t, h.fs["b"], "/dst/there", "t")
}

func TestKindMismatchesFailWithoutTouchingTheDestination(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a, b := h.fs["a"], h.fs["b"]
	a.addFile("/src/name1", "file")
	a.addFile("/src/name2/inner", "inner")
	b.addDir("/dst/src/name1") // a directory where a file should go
	b.addFile("/dst/src/name2", "i am a file")
	v := h.waitFinal(h.copyJob("a", []string{"/src"}, "b", "/dst", PolicyOverwrite))
	if v.State != StateFailed || v.FailedFiles != 2 {
		t.Fatalf("view = %+v", v)
	}
	wantContent(t, b, "/dst/src/name2", "i am a file")
	if b.node("/dst/src/name1").dir != true {
		t.Fatal("directory was replaced by a file")
	}
}

func TestCopyIntoExistingDirectoryMergesWithoutAsking(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a, b := h.fs["a"], h.fs["b"]
	a.addFile("/src/new.txt", "new")
	a.addFile("/src/sub/new2.txt", "new2")
	b.addFile("/dst/src/keep.txt", "keep")
	b.addDir("/dst/src/sub")
	v := h.waitFinal(h.copyJob("a", []string{"/src"}, "b", "/dst", PolicyAsk))
	if v.State != StateDone || h.asker.callCount() != 0 {
		t.Fatalf("state = %s, prompts = %d", v.State, h.asker.callCount())
	}
	wantContent(t, b, "/dst/src/keep.txt", "keep")
	wantContent(t, b, "/dst/src/sub/new2.txt", "new2")
}

func TestConflictsAreDetectedByListingEachDestinationDirOnce(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a, b := h.fs["a"], h.fs["b"]
	names := numbered("f", 40)
	addFiles(a, "/src", names)
	addFiles(a, "/src/sub", names)
	addFiles(b, "/dst/src", names[:10])
	v := h.waitFinal(h.copyJob("a", []string{"/src"}, "b", "/dst", PolicyOverwrite))
	if v.State != StateDone {
		t.Fatalf("state = %s %+v", v.State, v.Failed)
	}
	for _, d := range []string{"/dst", "/dst/src"} {
		if n := b.count(b.listCalls, d); n != 1 {
			t.Errorf("destination %s listed %d times, want once", d, n)
		}
	}
	if n := b.count(b.listCalls, "/dst/src/sub"); n != 0 {
		t.Errorf("fresh directory /dst/src/sub listed %d times, want 0", n)
	}
	for p, n := range b.statCalls {
		t.Errorf("destination Stat(%s) called %d times; conflicts must come from listings", p, n)
	}
}

func TestPolicyOverwriteAndSkip(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		policy    Policy
		want      string
		wantState State
	}{
		{PolicyOverwrite, "new", StateDone},
		{PolicySkip, "old", StateDone},
	} {
		t.Run(string(tc.policy), func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.fs["a"].addFile("/src/x.txt", "new").addFile("/src/y.txt", "y")
			h.fs["b"].addFile("/dst/x.txt", "old")
			v := h.waitFinal(h.copyJob("a", []string{"/src/x.txt", "/src/y.txt"}, "b", "/dst", tc.policy))
			if v.State != tc.wantState {
				t.Fatalf("state = %s", v.State)
			}
			wantContent(t, h.fs["b"], "/dst/x.txt", tc.want)
			wantContent(t, h.fs["b"], "/dst/y.txt", "y")
			if (tc.policy == PolicySkip) != (v.SkippedFiles == 1) {
				t.Fatalf("skipped = %d", v.SkippedFiles)
			}
			if h.asker.callCount() != 0 {
				t.Fatal("non-ask policy prompted")
			}
		})
	}
}

func TestJobWhereEverythingIsSkippedReportsSkipped(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.fs["a"].addFile("/src/x", "n")
	h.fs["b"].addFile("/dst/x", "o")
	v := h.waitFinal(h.copyJob("a", []string{"/src/x"}, "b", "/dst", PolicySkip))
	if v.State != StateSkipped || v.TotalBytes != 0 {
		t.Fatalf("view = %+v", v)
	}
}

func TestCopyLoopPicksWriterToThenReaderFromThenBuffer(t *testing.T) {
	t.Parallel()
	const payload = "payload bytes"
	for _, tc := range []struct {
		name                 string
		readerWT, writerRF   bool
		wantWT, wantRF, want int // want: minimum max Read buffer (0 = don't care)
	}{
		{"WriterTo wins over ReaderFrom", true, true, 1, 0, 0},
		{"ReaderFrom when reader is plain", false, true, 0, 1, 0},
		{"plain copies through a 1MiB buffer", false, false, 0, 0, copyBufSize},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.fs["a"].addFile("/src/f", payload).configure(func(f *fakeFS) { f.readerWriterTo = tc.readerWT })
			h.fs["b"].addDir("/dst")
			h.fs["b"].configure(func(f *fakeFS) { f.writerReaderFrom = tc.writerRF })
			v := h.waitFinal(h.copyJob("a", []string{"/src/f"}, "b", "/dst", PolicyAsk))
			if v.State != StateDone || v.DoneBytes != int64(len(payload)) {
				t.Fatalf("view = %+v", v)
			}
			wantContent(t, h.fs["b"], "/dst/f", payload)
			a, b := h.fs["a"], h.fs["b"]
			if (a.viaWriteTo > 0) != (tc.wantWT > 0) || (b.viaReadFrm > 0) != (tc.wantRF > 0) {
				t.Fatalf("WriteTo used %d, ReadFrom used %d", a.viaWriteTo, b.viaReadFrm)
			}
			if a.maxReadBuf < tc.want {
				t.Fatalf("largest Read buffer = %d, want >= %d", a.maxReadBuf, tc.want)
			}
		})
	}
}

func TestShortWriteIsCaughtBeforeCommitAndAborted(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a, b := h.fs["a"], h.fs["b"]
	a.addFile("/src/f", "0123456789")
	b.addFile("/dst/f", "precious")
	b.configure(func(f *fakeFS) { f.lenientCommit = true }) // only the engine's own check can catch it
	a.setHook(func(_ context.Context, op, p string) error {
		if op == "open" {
			a.mu.Lock()
			a.nodes[p].data = []byte("01234") // shrinks after the listing recorded 10 bytes
			a.mu.Unlock()
		}
		return nil
	})
	v := h.waitFinal(h.copyJob("a", []string{"/src/f"}, "b", "/dst", PolicyOverwrite))
	if v.State != StateFailed || v.FailedFiles != 1 {
		t.Fatalf("view = %+v", v)
	}
	if got := v.Failed[0].Error; !strings.Contains(got, vfs.ErrSizeMismatch.Error()) {
		t.Fatalf("error = %q, want size mismatch", got)
	}
	commits, aborts, _ := b.counters()
	if commits != 0 || aborts != 1 {
		t.Fatalf("commits=%d aborts=%d, want 0 and 1", commits, aborts)
	}
	wantContent(t, b, "/dst/f", "precious")
	if v.DoneBytes != 0 {
		t.Fatalf("DoneBytes = %d after failure, want rollback to 0", v.DoneBytes)
	}
	h.assertQuiescent()
}

func TestFailedCreateAndOpenReleaseEverything(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a, b := h.fs["a"], h.fs["b"]
	a.addFile("/src/openfail", "x").addFile("/src/createfail", "y")
	a.failOn("open", "/src/openfail", errors.New("boom open"))
	b.failOn("create", "/dst/createfail", errors.New("boom create"))
	v := h.waitFinal(h.copyJob("a", []string{"/src/openfail", "/src/createfail"}, "b", "/dst", PolicyAsk))
	if v.State != StateFailed || v.FailedFiles != 2 {
		t.Fatalf("view = %+v", v)
	}
	h.assertQuiescent()
	wantTree(t, b, "/dst")
}
