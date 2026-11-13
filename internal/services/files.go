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

// List streams dir's entries as files:page events tagged with requestID and returns the
// total count, so the caller knows when every page has arrived. Cancel the promise to stop.
func (s *FileService) List(ctx context.Context, connID, dir, requestID string) (int, error) {
	fs, err := s.app.reg.FS(ctx, connID)
	if err != nil {
		return 0, err
	}

	total := 0
	err = fs.List(ctx, dir, func(page []vfs.Entry) error {
		total += len(page)
		s.app.emit(EventFilesPage, FilePage{RequestID: requestID, Entries: page})
		return nil
	})
	return total, err
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
