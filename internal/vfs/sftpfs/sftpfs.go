// Package sftpfs implements vfs.FS over SFTP on an established SSH connection.
//
// It opens two SFTP sessions on the connection: one for browsing (List, Stat, Mkdir,
// Remove, Rename, Home) and one for transfers (Open, Create), so a large transfer never
// delays a directory listing.
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

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"

	"github.com/w33ble/portunus/internal/vfs"
)

const (
	pageSize        = 256
	symlinkParallel = 8
	maxLinkHops     = 16
	tmpSuffix       = ".portunus-tmp"
	posixRenameExt  = "posix-rename@openssh.com"
)

// FS is a vfs.FS over SFTP.
type FS struct {
	ssh    *ssh.Client
	browse *sftp.Client
	xfer   *sftp.Client

	closeOnce sync.Once
	closeErr  error
}

var _ vfs.FS = (*FS)(nil)

// New opens the browse and transfer SFTP sessions on client. Close closes both and client.
// On error client is left open.
func New(client *ssh.Client) (*FS, error) {
	browse, err := sftp.NewClient(client)
	if err != nil {
		return nil, fmt.Errorf("sftp browse session: %w", err)
	}
	xfer, err := sftp.NewClient(client, sftp.UseConcurrentWrites(true))
	if err != nil {
		_ = browse.Close()
		return nil, fmt.Errorf("sftp transfer session: %w", err)
	}
	return &FS{ssh: client, browse: browse, xfer: xfer}, nil
}

func (f *FS) Kind() vfs.Kind { return vfs.KindSFTP }

func (f *FS) Close() error {
	f.closeOnce.Do(func() {
		f.closeErr = errors.Join(f.browse.Close(), f.xfer.Close(), f.ssh.Close())
	})
	return f.closeErr
}

// wrap adds context and maps status codes the sftp package leaves raw to vfs sentinels
// (not-exist and permission are already mapped by pkg/sftp).
func wrap(op, p string, err error) error {
	if err == nil {
		return nil
	}
	var se *sftp.StatusError
	if errors.As(err, &se) && se.FxCode() == sftp.ErrSSHFxOpUnsupported && !errors.Is(err, vfs.ErrUnsupported) {
		return fmt.Errorf("sftp %s %s: %w: %w", op, p, vfs.ErrUnsupported, err)
	}
	return fmt.Errorf("sftp %s %s: %w", op, p, err)
}

func (f *FS) Home(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	h, err := f.browse.Getwd()
	if err != nil {
		return "", wrap("realpath", ".", err)
	}
	return h, nil
}

func (f *FS) List(ctx context.Context, dir string, page func([]vfs.Entry) error) error {
	dir = path.Clean(dir)
	infos, err := f.browse.ReadDirContext(ctx, dir)
	if err != nil {
		return wrap("readdir", dir, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for start := 0; start < len(infos); start += pageSize {
		batch := infos[start:min(start+pageSize, len(infos))]
		entries := make([]vfs.Entry, len(batch))
		var g errgroup.Group
		g.SetLimit(symlinkParallel)
		for i, fi := range batch {
			g.Go(func() error {
				entries[i] = f.entry(path.Join(dir, fi.Name()), fi)
				return nil
			})
		}
		_ = g.Wait()
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := page(entries); err != nil {
			return err
		}
	}
	return nil
}

// entry builds an Entry from lstat info, resolving symlinks like `ls -L` (best effort:
// a dangling link reports IsDir=false, Size=0).
func (f *FS) entry(p string, lfi os.FileInfo) vfs.Entry {
	e := vfs.Entry{
		Name:    lfi.Name(),
		Path:    p,
		Size:    lfi.Size(),
		ModTime: lfi.ModTime(),
		Mode:    lfi.Mode(),
		IsDir:   lfi.IsDir(),
	}
	if lfi.Mode()&fs.ModeSymlink == 0 {
		return e
	}
	e.Size, e.IsDir = 0, false
	if target, err := f.browse.ReadLink(p); err == nil {
		e.Link = target
	} else {
		e.Link = "?"
	}
	if tfi, err := f.browse.Stat(p); err == nil {
		e.Size, e.ModTime, e.IsDir = tfi.Size(), tfi.ModTime(), tfi.IsDir()
	}
	return e
}

func (f *FS) Stat(ctx context.Context, p string) (vfs.Entry, error) {
	if err := ctx.Err(); err != nil {
		return vfs.Entry{}, err
	}
	p = path.Clean(p)
	fi, err := f.browse.Lstat(p)
	if err != nil {
		return vfs.Entry{}, wrap("stat", p, err)
	}
	return f.entry(p, fi), nil
}

func (f *FS) Open(ctx context.Context, p string, offset int64) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p = path.Clean(p)
	file, err := f.xfer.Open(p)
	if err != nil {
		return nil, wrap("open", p, err)
	}
	if offset > 0 {
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			_ = file.Close()
			return nil, wrap("seek", p, err)
		}
	}
	r := &reader{f: file, ctx: ctx}
	return r, nil
}

func (f *FS) Mkdir(ctx context.Context, p string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p = path.Clean(p)
	err := f.browse.MkdirAll(p)
	if err == nil {
		return nil
	}
	if fi, serr := f.browse.Stat(p); serr == nil && !fi.IsDir() {
		return fmt.Errorf("sftp mkdir %s: not a directory: %w", p, vfs.ErrExist)
	}
	return wrap("mkdir", p, err)
}

func (f *FS) Remove(ctx context.Context, p string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p = path.Clean(p)
	return wrap("remove", p, f.browse.Remove(p))
}

// Rename refuses existing targets. The Lstat check races with other writers; the
// server's own rename (link-based on OpenSSH) is the backstop.
func (f *FS) Rename(ctx context.Context, from, to string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	from, to = path.Clean(from), path.Clean(to)
	if _, err := f.browse.Lstat(to); err == nil {
		return fmt.Errorf("sftp rename %s: target exists: %w", to, vfs.ErrExist)
	}
	if err := f.browse.Rename(from, to); err != nil {
		if _, lerr := f.browse.Lstat(to); lerr == nil {
			return fmt.Errorf("sftp rename %s: target exists: %w", to, vfs.ErrExist)
		}
		return wrap("rename", from, err)
	}
	return nil
}
