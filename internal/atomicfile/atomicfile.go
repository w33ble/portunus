// Package atomicfile replaces files without ever exposing a half-written one.
package atomicfile

import (
	"fmt"
	"os"
	"path/filepath"
)

// Write stores data at path via a temp file in the same directory and a rename, creating the
// directory (0700) when needed. The file is readable by the owner only.
func Write(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	// CreateTemp creates the file with mode 0600.
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
