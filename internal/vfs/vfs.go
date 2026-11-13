// Package vfs defines the filesystem abstraction every backend (local, SFTP, S3) implements.
//
// All paths are absolute and '/'-separated on every backend, including local on Windows
// ("C:/Users/x"), so callers use package path, never filepath.
package vfs

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"time"
)

// Kind identifies a backend.
type Kind string

const (
	KindLocal Kind = "local"
	KindSFTP  Kind = "sftp"
	KindS3    Kind = "s3"
)

// Sentinel errors. Backends wrap their native errors so errors.Is works against these.
var (
	ErrNotExist    = fs.ErrNotExist
	ErrExist       = fs.ErrExist
	ErrPermission  = fs.ErrPermission
	ErrUnsupported = errors.ErrUnsupported
	// ErrSizeMismatch is returned by Writer.Commit when Written() differs from CreateOpts.Size.
	ErrSizeMismatch = errors.New("vfs: written size does not match expected size")
)

// Entry describes one file, directory, or symlink.
//
// For symlinks, Size/ModTime/IsDir describe the link target when it resolves (like `ls -L`),
// and Link holds the raw link target. A dangling link has IsDir false and Size 0.
type Entry struct {
	Name    string      `json:"name"`
	Path    string      `json:"path"`
	Size    int64       `json:"size"`
	ModTime time.Time   `json:"modTime"`
	Mode    fs.FileMode `json:"mode"`
	IsDir   bool        `json:"isDir"`
	Link    string      `json:"link,omitempty"`
}

// IsSymlink reports whether the entry is a symbolic link.
func (e Entry) IsSymlink() bool { return e.Link != "" }

// CreateOpts carries metadata for a new file. Mode and ModTime are applied at Commit
// where the backend supports them; zero values mean "backend default".
type CreateOpts struct {
	Size    int64
	Mode    fs.FileMode
	ModTime time.Time
}

// FS is a connected filesystem. Implementations must be safe for concurrent use.
type FS interface {
	Kind() Kind

	// Home returns the directory a new pane should open at.
	Home(ctx context.Context) (string, error)

	// List calls page with batches of the directory's entries, in backend order.
	// Returning an error from page stops listing and List returns that error.
	List(ctx context.Context, dir string, page func([]Entry) error) error

	Stat(ctx context.Context, p string) (Entry, error)

	// Open returns a reader positioned at offset. Closing it aborts in-flight reads,
	// which is how cancellation reaches backends that ignore ctx.
	// The reader may implement io.WriterTo for pipelined reads.
	Open(ctx context.Context, p string, offset int64) (io.ReadCloser, error)

	// Create returns a Writer whose content becomes visible at p only on Commit.
	// The Writer may implement io.ReaderFrom for pipelined writes.
	Create(ctx context.Context, p string, opts CreateOpts) (Writer, error)

	// Mkdir creates p and any missing parents. Existing directories are not an error.
	Mkdir(ctx context.Context, p string) error

	// Remove deletes a single file, symlink, object, or empty directory. Recursion is
	// the transfer engine's job.
	Remove(ctx context.Context, p string) error

	// Rename moves from to to. It fails with ErrExist if to exists, and with
	// ErrUnsupported where the backend cannot do it (e.g. S3 prefixes).
	Rename(ctx context.Context, from, to string) error

	Close() error
}

// Writer stages a file. Nothing is visible at the target path until Commit succeeds;
// Abort (or a failed Commit) leaves no trace. Exactly one of Commit or Abort must be called.
type Writer interface {
	io.Writer
	Written() int64
	Commit() error
	Abort() error
}

// CaseInsensitive is optionally implemented by filesystems whose name lookups ignore case
// (macOS APFS/HFS+ defaults, Windows), so "README" and "readme" are the same entry.
type CaseInsensitive interface {
	CaseInsensitive() bool
}

// IsCaseInsensitive reports whether fsys matches names without regard to case.
func IsCaseInsensitive(fsys FS) bool {
	ci, ok := fsys.(CaseInsensitive)
	return ok && ci.CaseInsensitive()
}
