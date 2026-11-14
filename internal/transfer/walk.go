package transfer

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/w33ble/portunus/internal/prompt"
	"github.com/w33ble/portunus/internal/vfs"
)

var (
	errDstIsDir  = errors.New("destination is a directory")
	errDstNotDir = errors.New("destination exists and is not a directory")
)

// walkItem is one unit of producer work.
type walkItem struct {
	root    bool
	sources []string // root: top-level paths to examine
	task    *task    // directory to expand
	listDst bool     // copy: destination may exist, so list it for conflicts instead of creating it
}

type entryClass int

const (
	classFile entryClass = iota
	classDir
	classLinkedDir // symlink to a directory: never followed
	classSpecial   // fifo, socket, device: opening could block forever
)

const specialModes = fs.ModeNamedPipe | fs.ModeSocket | fs.ModeDevice | fs.ModeCharDevice | fs.ModeIrregular

func classify(ent vfs.Entry) entryClass {
	switch {
	case ent.IsDir && ent.IsSymlink():
		return classLinkedDir
	case ent.IsDir:
		return classDir
	case ent.Mode&specialModes != 0:
		return classSpecial
	default:
		return classFile
	}
}

// pushReversed stacks items so the first one found is the next one walked.
func pushReversed(stack []walkItem, found []walkItem) []walkItem {
	for _, it := range slices.Backward(found) {
		stack = append(stack, it)
	}
	return stack
}

// copyWalker expands a copy or move job depth-first. Directories are created and listed
// here, on the producer goroutine, before any of their files are queued; file tasks go to
// the scheduler (or the conflict gate) as they are found.
//
// A move walks the same way, and differs in three places: a task whose destination is free
// on a single connection is renamed instead of copied, every task is linked to the directory
// that holds it (see parent), and that directory stays pending until its contents are gone.
type copyWalker struct {
	e         *Engine
	j         *job
	ctx       context.Context
	conflicts chan<- *task
	stack     []walkItem
	cur       *task // the directory being expanded; nil while walking the top-level sources
}

// parent is the directory task that new tasks hang from. Only a move needs it: it removes
// the source directory once everything inside has moved.
func (w *copyWalker) parent() *task {
	if w.j.kind == KindMove {
		return w.cur
	}
	return nil
}

// sameConn reports whether this is a move that never leaves its connection, so a rename can
// stand in for a copy.
func (w *copyWalker) sameConn() bool {
	return w.j.kind == KindMove && w.j.srcConn == w.j.dstConn
}

// key is the name under which conflicts and duplicate sources are matched: folded to lower
// case when the destination treats names case-insensitively.
func (w *copyWalker) key(name string) string {
	if vfs.IsCaseInsensitive(w.j.dst) {
		return strings.ToLower(name)
	}
	return name
}

func (w *copyWalker) run() {
	for len(w.stack) > 0 && w.ctx.Err() == nil {
		it := w.stack[len(w.stack)-1]
		w.stack = w.stack[:len(w.stack)-1]
		if it.root {
			w.root(it.sources)
		} else {
			w.dir(it)
		}
	}
}

func (w *copyWalker) root(sources []string) {
	w.cur = nil
	existing, err := w.loadDst(w.j.dstDir)
	if err != nil {
		w.failRoot(sources, fmt.Errorf("prepare destination %s: %w", w.j.dstDir, err))
		return
	}
	var found []walkItem
	seen := make(map[string]struct{}, len(sources))
	for _, src := range sources {
		if w.ctx.Err() != nil {
			break
		}
		name := path.Base(src)
		dst := path.Join(w.j.dstDir, name)
		ent, err := w.j.src.Stat(w.ctx, src)
		if err != nil {
			if w.j.kind == KindMove && errors.Is(err, vfs.ErrNotExist) && w.e.alreadyFinished(w.j, dst) {
				continue // moved by an earlier run of this job
			}
			w.fail(taskSource, src, dst, fmt.Errorf("stat %s: %w", src, err))
			continue
		}
		w.e.firstSourceKnown(w.j, src, ent)
		if _, dup := seen[w.key(name)]; dup {
			w.fail(taskFile, src, dst, fmt.Errorf("another source is also named %q", name))
			continue
		}
		seen[w.key(name)] = struct{}{}
		ent.Name, ent.Path = name, src
		if err := w.entry(src, ent, w.j.dstDir, existing, &found); err != nil {
			break
		}
	}
	w.stack = pushReversed(w.stack, found)
}

// failRoot records that the destination could not be prepared: every source fails with
// the same error, and Retry walks the root again.
func (w *copyWalker) failRoot(sources []string, err error) {
	w.e.mu.Lock()
	w.j.errMsg = err.Error()
	w.e.mu.Unlock()
	if w.ctx.Err() != nil {
		return
	}
	for _, src := range sources {
		w.fail(taskSource, src, path.Join(w.j.dstDir, path.Base(src)), err)
	}
}

func (w *copyWalker) dir(it walkItem) {
	t := it.task
	w.cur = t
	w.e.beginInline(w.j, t)
	found, err := w.expand(it)
	w.stack = pushReversed(w.stack, found)
	if w.j.kind == KindMove && err == nil {
		w.e.endExpand(w.j, t)
		return
	}
	w.e.endInline(w.j, t, err)
}

// expand creates (or lists) the destination directory and queues what the source holds.
// Subdirectories it found are returned even on error: they are already counted and must be
// walked or the job never finishes.
func (w *copyWalker) expand(it walkItem) ([]walkItem, error) {
	t := it.task
	existing := map[string]vfs.Entry{}
	if it.listDst {
		var err error
		if existing, err = w.loadDst(t.dst); err != nil {
			return nil, err
		}
	} else if err := w.j.dst.Mkdir(w.ctx, t.dst); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", t.dst, err)
	}
	var found []walkItem
	err := w.j.src.List(w.ctx, t.src, func(page []vfs.Entry) error {
		for _, ent := range page {
			if err := w.entry(path.Join(t.src, ent.Name), ent, t.dst, existing, &found); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return found, fmt.Errorf("list %s: %w", t.src, err)
	}
	return found, nil
}

// loadDst lists a destination directory once so conflicts are name lookups, not per-file
// Stats. A missing directory is created and has no conflicts.
func (w *copyWalker) loadDst(dir string) (map[string]vfs.Entry, error) {
	existing := map[string]vfs.Entry{}
	err := w.j.dst.List(w.ctx, dir, func(page []vfs.Entry) error {
		for _, ent := range page {
			existing[w.key(ent.Name)] = ent
		}
		return nil
	})
	switch {
	case errors.Is(err, vfs.ErrNotExist):
		if err := w.j.dst.Mkdir(w.ctx, dir); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", dir, err)
		}
		return map[string]vfs.Entry{}, nil
	case err != nil:
		return nil, fmt.Errorf("list %s: %w", dir, err)
	}
	return existing, nil
}

// entry turns one source entry into a task. A non-nil error means the job was canceled.
func (w *copyWalker) entry(src string, ent vfs.Entry, dstDir string, existing map[string]vfs.Entry, found *[]walkItem) error {
	if !validName(ent.Name) {
		w.fail(taskFile, src, "", fmt.Errorf("invalid entry name %q", ent.Name))
		return nil
	}
	dst := path.Join(dstDir, ent.Name)
	old, exists := existing[w.key(ent.Name)]
	switch classify(ent) {
	case classLinkedDir, classSpecial:
		if !exists && w.tryRename(taskFile, src, dst, ent) {
			break
		}
		w.skip(taskFile, src, dst)
	case classDir:
		w.dirEntry(src, dst, ent, old, exists, found)
	case classFile:
		return w.fileEntry(src, dst, ent, old, exists)
	}
	return w.ctx.Err()
}

func (w *copyWalker) dirEntry(src, dst string, ent, old vfs.Entry, exists bool, found *[]walkItem) {
	if exists && !old.IsDir {
		w.fail(taskDir, src, dst, errDstNotDir)
		return
	}
	if !exists && w.tryRename(taskDir, src, dst, ent) {
		return
	}
	t := &task{kind: taskDir, src: src, dst: dst, ent: ent, parent: w.parent()}
	if w.j.kind == KindMove {
		t.remaining = 1 // released once the directory is fully listed
	}
	w.e.register(w.j, t)
	// A rename that failed may have left something at dst, so look before writing into it.
	*found = append(*found, walkItem{task: t, listDst: exists || w.sameConn()})
}

// tryRename moves src to dst with one Rename when the job stays on one connection and dst
// is free. It records the item as done and reports true on success; any failure (S3
// prefixes, another filesystem, a target that appeared) leaves src untouched and reports
// false, and the item is then copied and deleted like a cross-connection move.
func (w *copyWalker) tryRename(kind taskKind, src, dst string, ent vfs.Entry) bool {
	if !w.sameConn() {
		return false
	}
	if err := w.j.src.Rename(w.ctx, src, dst); err != nil {
		w.e.log.DebugContext(w.ctx, "rename failed, falling back to copy", "job", w.j.id, "src", src, "dst", dst, "err", err)
		return false
	}
	t := &task{kind: kind, src: src, dst: dst, ent: ent, parent: w.parent()}
	w.e.register(w.j, t)
	w.e.settle(w.j, t, StateDone, nil)
	return true
}

type resolution int

const (
	resolveAsk resolution = iota
	resolveOverwrite
	resolveSkip
)

func (w *copyWalker) resolution() resolution {
	switch w.j.policy {
	case PolicyOverwrite:
		return resolveOverwrite
	case PolicySkip:
		return resolveSkip
	case PolicyAsk:
		return w.e.applyAllResolution(w.j)
	}
	return resolveAsk
}

func (w *copyWalker) fileEntry(src, dst string, ent, old vfs.Entry, exists bool) error {
	if w.j.kind == KindCopy && w.e.alreadyFinished(w.j, dst) {
		return nil
	}
	if exists && old.IsDir {
		w.fail(taskFile, src, dst, errDstIsDir)
		return nil
	}
	t := &task{kind: taskFile, src: src, dst: dst, ent: ent, size: ent.Size, parent: w.parent()}
	if !exists {
		t.rename = w.sameConn()
		w.e.register(w.j, t)
		return w.deliver(w.e.enqueueReady(w.j, t))
	}
	switch w.resolution() {
	case resolveSkip:
		w.e.register(w.j, t)
		w.e.settle(w.j, t, StateSkipped, nil)
		return nil
	case resolveOverwrite:
		t.overwrite = true
		w.e.register(w.j, t)
		return w.deliver(w.e.enqueueReady(w.j, t))
	case resolveAsk:
		t.dstEnt = &old
		w.e.register(w.j, t)
		w.e.markWaiting(w.j, t)
		return w.deliver(send(w.ctx, w.conflicts, t))
	}
	return nil
}

func (w *copyWalker) deliver(ok bool) error {
	if ok {
		return nil
	}
	return w.ctx.Err()
}

// fail and skip record a task that never reaches a worker.
func (w *copyWalker) fail(kind taskKind, src, dst string, err error) {
	t := &task{kind: kind, src: src, dst: dst, noRetry: kind != taskSource, parent: w.parent()}
	w.e.register(w.j, t)
	w.e.settle(w.j, t, outcomeOf(w.j, err), err)
}

func (w *copyWalker) skip(kind taskKind, src, dst string) {
	t := &task{kind: kind, src: src, dst: dst, parent: w.parent()}
	w.e.register(w.j, t)
	w.e.settle(w.j, t, StateSkipped, nil)
}

// applyAllResolution returns the job's remembered "apply to all" answer, if any.
func (e *Engine) applyAllResolution(j *job) resolution {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch j.applyAll {
	case prompt.ConflictOverwrite:
		return resolveOverwrite
	case prompt.ConflictSkip:
		return resolveSkip
	case prompt.ConflictCancel:
	}
	return resolveAsk
}

// deleteWalker expands a delete job. File and link tasks queue as they are found;
// a directory task waits (see dirReadyLocked) until everything inside it is gone.
type deleteWalker struct {
	e     *Engine
	j     *job
	ctx   context.Context
	stack []walkItem
}

func (w *deleteWalker) run() {
	for len(w.stack) > 0 && w.ctx.Err() == nil {
		it := w.stack[len(w.stack)-1]
		w.stack = w.stack[:len(w.stack)-1]
		if it.root {
			w.root(it.sources)
		} else {
			w.dir(it.task)
		}
	}
}

func (w *deleteWalker) root(sources []string) {
	var found []walkItem
	for _, src := range sources {
		if w.ctx.Err() != nil {
			break
		}
		ent, err := w.j.src.Stat(w.ctx, src)
		switch {
		case errors.Is(err, vfs.ErrNotExist):
			if !w.e.alreadyFinished(w.j, src) {
				w.gone(src)
			}
			continue
		case err != nil:
			w.fail(taskSource, src, fmt.Errorf("stat %s: %w", src, err))
			continue
		}
		w.e.firstSourceKnown(w.j, src, ent)
		if err := w.entry(src, ent, nil, &found); err != nil {
			break
		}
	}
	w.stack = pushReversed(w.stack, found)
}

// gone records a source that no longer exists; there is nothing to delete.
func (w *deleteWalker) gone(src string) {
	t := &task{kind: taskFile, src: src}
	w.e.register(w.j, t)
	w.e.settle(w.j, t, StateSkipped, nil)
}

func (w *deleteWalker) fail(kind taskKind, src string, err error) {
	t := &task{kind: kind, src: src, noRetry: kind != taskSource}
	w.e.register(w.j, t)
	w.e.settle(w.j, t, outcomeOf(w.j, err), err)
}

func (w *deleteWalker) entry(src string, ent vfs.Entry, parent *task, found *[]walkItem) error {
	t := &task{kind: taskFile, src: src, parent: parent}
	if classify(ent) == classDir {
		t.kind = taskDir
		t.remaining = 1 // released once the directory is fully listed
		w.e.register(w.j, t)
		*found = append(*found, walkItem{task: t})
		return nil
	}
	w.e.register(w.j, t)
	if !w.e.enqueueReady(w.j, t) {
		return w.ctx.Err()
	}
	return nil
}

func (w *deleteWalker) dir(t *task) {
	var found []walkItem
	err := w.j.src.List(w.ctx, t.src, func(page []vfs.Entry) error {
		for _, ent := range page {
			if !validName(ent.Name) {
				w.failChild(t, path.Join(t.src, ent.Name), fmt.Errorf("invalid entry name %q", ent.Name))
				continue
			}
			if err := w.entry(path.Join(t.src, ent.Name), ent, t, &found); err != nil {
				return err
			}
		}
		return nil
	})
	w.stack = pushReversed(w.stack, found)
	switch {
	case err == nil:
		w.e.expanded(w.j, t)
	case errors.Is(err, vfs.ErrNotExist):
		w.e.settle(w.j, t, StateDone, nil)
	default:
		w.e.settle(w.j, t, outcomeOf(w.j, err), fmt.Errorf("list %s: %w", t.src, err))
	}
}

func (w *deleteWalker) failChild(parent *task, src string, err error) {
	t := &task{kind: taskFile, src: src, parent: parent, noRetry: true}
	w.e.register(w.j, t)
	w.e.settle(w.j, t, StateFailed, err)
}

// expanded releases the directory's listing token; the directory is queued for removal
// once its last child is gone.
func (e *Engine) expanded(j *job, t *task) {
	e.mu.Lock()
	defer e.mu.Unlock()
	t.expanded = true
	t.remaining--
	e.dirReadyLocked(j, t)
	e.dispatchLocked()
}
