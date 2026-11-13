package services

import (
	"context"
	"errors"
	"path"
	"strings"

	"github.com/w33ble/portunus/internal/vfs"
)

// FileService browses and edits a connection's filesystem. Deletes are transfer jobs.
type FileService struct {
	app *App
}

// ListPage is one batch of a directory listing. The final page, which can be empty, has no Cursor.
type ListPage struct {
	Entries []vfs.Entry `json:"entries"`
	// Cursor is passed to ListNext for the following page; "" means the listing is complete.
	Cursor string `json:"cursor"`
}

// List starts listing dir and returns its first page. While the Cursor is non-empty, call
// ListNext for more or ListClose to give up. Canceling the promise abandons the listing.
func (s *FileService) List(ctx context.Context, connID, dir string) (ListPage, error) {
	fs, err := s.app.reg.FS(ctx, connID)
	if err != nil {
		return ListPage{}, err
	}

	l, err := s.app.lists.start(fs, dir)
	if err != nil {
		return ListPage{}, err
	}
	return s.app.lists.pull(ctx, l)
}

// ListNext returns the page after the one that issued cursor. A listing that fails midway
// reports its error here; the listing is then over.
func (s *FileService) ListNext(ctx context.Context, cursor string) (ListPage, error) {
	l, err := s.app.lists.get(cursor)
	if err != nil {
		return ListPage{}, err
	}
	return s.app.lists.pull(ctx, l)
}

// ListClose releases a listing the caller no longer wants.
func (s *FileService) ListClose(cursor string) {
	s.app.lists.close(cursor)
}

func (s *FileService) Stat(ctx context.Context, connID, p string) (vfs.Entry, error) {
	fs, err := s.app.reg.FS(ctx, connID)
	if err != nil {
		return vfs.Entry{}, err
	}
	return fs.Stat(ctx, p)
}

// Mkdir creates name inside dir and returns the new directory's entry.
func (s *FileService) Mkdir(ctx context.Context, connID, dir, name string) (vfs.Entry, error) {
	if err := validName(name); err != nil {
		return vfs.Entry{}, err
	}

	fs, err := s.app.reg.FS(ctx, connID)
	if err != nil {
		return vfs.Entry{}, err
	}

	p := path.Join(dir, name)
	if _, err := fs.Stat(ctx, p); err == nil {
		return vfs.Entry{}, vfs.ErrExist
	}
	if err := fs.Mkdir(ctx, p); err != nil {
		return vfs.Entry{}, err
	}
	return fs.Stat(ctx, p)
}

// Rename renames p within its directory. It never overwrites.
func (s *FileService) Rename(ctx context.Context, connID, p, newName string) (vfs.Entry, error) {
	if err := validName(newName); err != nil {
		return vfs.Entry{}, err
	}

	fs, err := s.app.reg.FS(ctx, connID)
	if err != nil {
		return vfs.Entry{}, err
	}

	to := path.Join(path.Dir(p), newName)
	if err := fs.Rename(ctx, p, to); err != nil {
		return vfs.Entry{}, err
	}
	return fs.Stat(ctx, to)
}

func validName(name string) error {
	switch {
	case name == "", name == ".", name == "..":
		return errors.New("enter a name")
	case strings.ContainsAny(name, "/\x00"):
		return errors.New("names can't contain / or NUL")
	default:
		return nil
	}
}
