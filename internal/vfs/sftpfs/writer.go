package sftpfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"sync"
	"sync/atomic"

	"github.com/pkg/sftp"

	"github.com/w33ble/portunus/internal/vfs"
)

// Create stages content in ".<name>.portunus-tmp" next to the target and renames it over
// the target on Commit (posix-rename, so existing files are replaced atomically). If the
// target is a symlink the link's destination is replaced. If the directory refuses a temp
// file but the existing target is writable, content is written in place instead; that
// path is not atomic and Abort cannot restore the old content.
func (f *FS) Create(ctx context.Context, p string, opts vfs.CreateOpts) (vfs.Writer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	target, err := f.resolveLink(path.Clean(p))
	if err != nil {
		return nil, wrap("create", p, err)
	}
	mode := opts.Mode.Perm()
	exists := false
	if fi, err := f.xfer.Stat(target); err == nil {
		if fi.IsDir() {
			return nil, fmt.Errorf("sftp create %s: is a directory: %w", target, vfs.ErrExist)
		}
		exists = true
		if mode == 0 {
			mode = fi.Mode().Perm()
		}
	}

	dir, base := path.Split(target)
	tmp := path.Join(dir, "."+base+tmpSuffix)
	file, err := f.xfer.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	direct := false
	if err != nil && exists && errors.Is(err, os.ErrPermission) {
		if file, err = f.xfer.OpenFile(target, os.O_WRONLY|os.O_TRUNC); err == nil {
			direct, tmp = true, target
		}
	}
	if err != nil {
		return nil, wrap("create", tmp, err)
	}
	return &writer{fs: f, ctx: ctx, file: file, target: target, tmp: tmp, opts: opts, mode: mode, direct: direct}, nil
}

// resolveLink follows symlinks at p so a commit replaces the link's destination.
func (f *FS) resolveLink(p string) (string, error) {
	for range maxLinkHops {
		fi, err := f.browse.Lstat(p)
		if err != nil || fi.Mode()&fs.ModeSymlink == 0 {
			return p, nil //nolint:nilerr // missing or unreadable paths are created as-is
		}
		dest, err := f.browse.ReadLink(p)
		if err != nil {
			return "", err
		}
		if !path.IsAbs(dest) {
			dest = path.Join(path.Dir(p), dest)
		}
		p = path.Clean(dest)
	}
	return "", errors.New("too many levels of symbolic links")
}

type writer struct {
	fs     *FS
	ctx    context.Context
	file   *sftp.File
	target string
	tmp    string
	opts   vfs.CreateOpts
	mode   fs.FileMode // 0 = leave to server umask
	direct bool

	mu      sync.Mutex // held for the duration of Write/ReadFrom/Commit/Abort
	aborted atomic.Bool
	done    bool
	written atomic.Int64
}

var _ io.ReaderFrom = (*writer)(nil)

func (w *writer) Written() int64 { return w.written.Load() }

func (w *writer) check() error {
	if w.aborted.Load() {
		return os.ErrClosed
	}
	return w.ctx.Err()
}

func (w *writer) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.done {
		return 0, os.ErrClosed
	}
	if err := w.check(); err != nil {
		return 0, err
	}
	n, err := w.file.Write(b)
	w.written.Add(int64(n))
	return n, wrap("write", w.tmp, err)
}

// ReadFrom uses pkg/sftp's pipelined concurrent writes; the reader wrapper reports the
// remaining size so the pipeline can size itself, and stops once the writer is aborted.
func (w *writer) ReadFrom(r io.Reader) (int64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.done {
		return 0, os.ErrClosed
	}
	if err := w.check(); err != nil {
		return 0, err
	}
	n, err := w.file.ReadFrom(&guardReader{r: r, w: w})
	w.written.Add(n)
	if errors.Is(err, os.ErrClosed) || errors.Is(err, context.Canceled) {
		return n, err
	}
	return n, wrap("write", w.tmp, err)
}

type guardReader struct {
	r io.Reader
	w *writer
}

func (g *guardReader) Read(b []byte) (int, error) {
	if err := g.w.check(); err != nil {
		return 0, err
	}
	return g.r.Read(b)
}

func (g *guardReader) Size() int64 {
	if rem := g.w.opts.Size - g.w.written.Load(); rem > 0 {
		return rem
	}
	return 0
}

func (w *writer) Commit() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.done {
		return os.ErrClosed
	}
	if err := w.check(); err != nil {
		w.cleanup()
		return err
	}
	if got := w.written.Load(); got != w.opts.Size {
		w.cleanup()
		return fmt.Errorf("sftp commit %s: wrote %d bytes, expected %d: %w", w.target, got, w.opts.Size, vfs.ErrSizeMismatch)
	}
	if err := w.file.Close(); err != nil {
		w.done = true
		if !w.direct {
			_ = w.fs.xfer.Remove(w.tmp)
		}
		return wrap("close", w.tmp, err)
	}
	w.done = true

	if err := w.applyMetadata(); err != nil {
		w.removeTemp()
		return err
	}
	if w.direct {
		return nil
	}
	if err := w.fs.replace(w.tmp, w.target); err != nil {
		w.removeTemp()
		return err
	}
	return nil
}

// applyMetadata sets mode and mtime. Servers that refuse setstat leave defaults; any
// other failure is an error.
func (w *writer) applyMetadata() error {
	c := w.fs.xfer
	soft := func(err error) bool {
		var se *sftp.StatusError
		return errors.Is(err, os.ErrPermission) || (errors.As(err, &se) && se.FxCode() == sftp.ErrSSHFxOpUnsupported)
	}
	if w.mode != 0 && (!w.direct || w.opts.Mode != 0) {
		if err := c.Chmod(w.tmp, w.mode); err != nil && !soft(err) {
			return wrap("chmod", w.tmp, err)
		}
	}
	if !w.opts.ModTime.IsZero() {
		if err := c.Chtimes(w.tmp, w.opts.ModTime, w.opts.ModTime); err != nil && !soft(err) {
			return wrap("chtimes", w.tmp, err)
		}
	}
	return nil
}

// replace renames tmp over target: atomically with posix-rename, else remove + rename.
func (f *FS) replace(tmp, target string) error {
	if _, ok := f.xfer.HasExtension(posixRenameExt); ok {
		return wrap("rename", target, f.xfer.PosixRename(tmp, target))
	}
	if err := f.xfer.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return wrap("remove", target, err)
	}
	return wrap("rename", target, f.xfer.Rename(tmp, target))
}

func (w *writer) removeTemp() {
	if !w.direct {
		_ = w.fs.xfer.Remove(w.tmp)
	}
}

func (w *writer) cleanup() {
	w.done = true
	_ = w.file.Close()
	w.removeTemp()
}

// Abort stops an in-flight write (waiting for the current chunk) and removes the temp file.
func (w *writer) Abort() error {
	w.aborted.Store(true)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.done {
		return nil
	}
	w.cleanup()
	return nil
}
