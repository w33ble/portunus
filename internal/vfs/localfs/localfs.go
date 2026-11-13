// Package localfs implements vfs.FS over the local filesystem.
package localfs

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/w33ble/portunus/internal/vfs"
)

const (
	pageSize  = 256
	tmpSuffix = ".portunus-tmp"
	maxHops   = 40
)

// FS is the whole local filesystem. Paths are absolute and '/'-separated.
type FS struct{}

var _ vfs.FS = (*FS)(nil)

// New returns a vfs.FS rooted at the local filesystem root.
func New() *FS { return &FS{} }

func (*FS) Kind() vfs.Kind { return vfs.KindLocal }

func (*FS) Close() error { return nil }

func (*FS) Home(context.Context) (string, error) {
	h, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("localfs home: %w", err)
	}
	return filepath.ToSlash(h), nil
}

func osPath(p string) string { return filepath.FromSlash(p) }

func isTemp(name string) bool {
	return strings.HasPrefix(name, ".") && strings.HasSuffix(name, tmpSuffix)
}

// entryFor describes p (an OS path) given its Lstat info; vp is the vfs path.
func entryFor(vp, osp string, li fs.FileInfo) vfs.Entry {
	e := vfs.Entry{
		Name:    path.Base(vp),
		Path:    vp,
		Size:    li.Size(),
		ModTime: li.ModTime(),
		Mode:    li.Mode(),
		IsDir:   li.IsDir(),
	}
	if li.Mode()&fs.ModeSymlink == 0 {
		return e
	}
	target, err := os.Readlink(osp)
	if err != nil {
		target = "?"
	}
	e.Link = filepath.ToSlash(target)
	e.Size, e.IsDir = 0, false
	if ti, err := os.Stat(osp); err == nil {
		e.Size, e.ModTime, e.Mode, e.IsDir = ti.Size(), ti.ModTime(), ti.Mode(), ti.IsDir()
	}
	return e
}

func (*FS) List(ctx context.Context, dir string, page func([]vfs.Entry) error) error {
	d, err := os.Open(osPath(dir))
	if err != nil {
		return fmt.Errorf("localfs list %s: %w", dir, err)
	}
	defer func() { _ = d.Close() }()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		des, err := d.ReadDir(pageSize)
		batch := make([]vfs.Entry, 0, len(des))
		for _, de := range des {
			if isTemp(de.Name()) {
				continue
			}
			vp := path.Join(dir, de.Name())
			li, lerr := de.Info()
			if lerr != nil {
				continue // vanished between readdir and lstat
			}
			batch = append(batch, entryFor(vp, osPath(vp), li))
		}
		if len(batch) > 0 {
			if perr := page(batch); perr != nil {
				return perr
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("localfs list %s: %w", dir, err)
		}
	}
}

func (*FS) Stat(_ context.Context, p string) (vfs.Entry, error) {
	li, err := os.Lstat(osPath(p))
	if err != nil {
		return vfs.Entry{}, fmt.Errorf("localfs stat %s: %w", p, err)
	}
	return entryFor(p, osPath(p), li), nil
}

func (*FS) Open(_ context.Context, p string, offset int64) (io.ReadCloser, error) {
	f, err := os.Open(osPath(p))
	if err != nil {
		return nil, fmt.Errorf("localfs open %s: %w", p, err)
	}
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("localfs seek %s: %w", p, err)
		}
	}
	return f, nil
}

func (*FS) Mkdir(_ context.Context, p string) error {
	if err := os.MkdirAll(osPath(p), 0o777); err != nil { //nolint:gosec // umask governs
		return fmt.Errorf("localfs mkdir %s: %w", p, err)
	}
	return nil
}

func (*FS) Remove(_ context.Context, p string) error {
	if err := os.Remove(osPath(p)); err != nil {
		return fmt.Errorf("localfs remove %s: %w", p, err)
	}
	return nil
}

// Rename refuses to replace an existing target. The Lstat check and the rename are
// not atomic, so a concurrent creator of `to` can still be overwritten.
func (*FS) Rename(_ context.Context, from, to string) error {
	if _, err := os.Lstat(osPath(to)); err == nil {
		return fmt.Errorf("localfs rename %s -> %s: %w", from, to, vfs.ErrExist)
	}
	if err := os.Rename(osPath(from), osPath(to)); err != nil {
		return fmt.Errorf("localfs rename %s -> %s: %w", from, to, err)
	}
	return nil
}

// resolveLinks follows symlinks at p (even dangling ones) so writes go through to the real file.
func resolveLinks(p string) string {
	for range maxHops {
		li, err := os.Lstat(p)
		if err != nil || li.Mode()&fs.ModeSymlink == 0 {
			return p
		}
		t, err := os.Readlink(p)
		if err != nil {
			return p
		}
		if !filepath.IsAbs(t) {
			t = filepath.Join(filepath.Dir(p), t)
		}
		p = t
	}
	return p
}

// Create stages into `.<name>.portunus-tmp` beside the target; concurrent writers to
// the same target are not supported.
func (*FS) Create(_ context.Context, p string, opts vfs.CreateOpts) (vfs.Writer, error) {
	target := resolveLinks(osPath(p))
	tmp := filepath.Join(filepath.Dir(target), "."+filepath.Base(target)+tmpSuffix)
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666) //nolint:gosec // umask governs
	if err != nil {
		return nil, fmt.Errorf("localfs create %s: %w", p, err)
	}
	return &writer{f: f, tmp: tmp, target: target, vpath: p, opts: opts}, nil
}

type writer struct {
	f      *os.File
	tmp    string
	target string
	vpath  string
	opts   vfs.CreateOpts
	n      int64
	done   bool
}

func (w *writer) Write(b []byte) (int, error) {
	n, err := w.f.Write(b)
	w.n += int64(n)
	return n, err
}

func (w *writer) Written() int64 { return w.n }

func (w *writer) discard() {
	_ = w.f.Close()
	_ = os.Remove(w.tmp)
	w.done = true
}

func (w *writer) Commit() error {
	if w.done {
		return fmt.Errorf("localfs commit %s: %w", w.vpath, fs.ErrClosed)
	}
	if w.n != w.opts.Size {
		w.discard()
		return fmt.Errorf("localfs commit %s: wrote %d of %d bytes: %w", w.vpath, w.n, w.opts.Size, vfs.ErrSizeMismatch)
	}
	if err := w.finish(); err != nil {
		w.discard()
		return fmt.Errorf("localfs commit %s: %w", w.vpath, err)
	}
	w.done = true
	return nil
}

func (w *writer) finish() error {
	mode := w.opts.Mode
	if mode == 0 {
		if ti, err := os.Stat(w.target); err == nil {
			mode = ti.Mode()
		}
	}
	if err := w.f.Close(); err != nil {
		return err
	}
	if mode != 0 {
		if err := os.Chmod(w.tmp, mode.Perm()); err != nil {
			return err
		}
	}
	if !w.opts.ModTime.IsZero() {
		if err := os.Chtimes(w.tmp, w.opts.ModTime, w.opts.ModTime); err != nil {
			return err
		}
	}
	return os.Rename(w.tmp, w.target)
}

func (w *writer) Abort() error {
	if w.done {
		return nil
	}
	w.discard()
	return nil
}
