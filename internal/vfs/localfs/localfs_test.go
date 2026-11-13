package localfs_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/w33ble/portunus/internal/vfs"
	"github.com/w33ble/portunus/internal/vfs/localfs"
	"github.com/w33ble/portunus/internal/vfs/vfstest"
)

func TestConformance(t *testing.T) {
	vfstest.Run(t, vfstest.Harness{
		New: func(t *testing.T) (vfs.FS, string) {
			return localfs.New(), filepath.ToSlash(t.TempDir())
		},
		Caps: vfstest.Caps{Symlinks: true, Modes: true, ModTimes: true, EmptyDirs: true, RenameDirs: true},
		Symlink: func(t *testing.T, target, link string) {
			t.Helper()
			if err := os.Symlink(filepath.FromSlash(target), filepath.FromSlash(link)); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
		},
	})
}

func TestCommitWritesThroughSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.txt")
	link := filepath.Join(dir, "link.txt")
	if err := os.WriteFile(real, []byte("old"), 0o640); err != nil { //nolint:gosec // mode under test
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	f := localfs.New()
	w, err := f.Create(t.Context(), filepath.ToSlash(link), vfs.CreateOpts{Size: 3})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("new")); err != nil {
		t.Fatal(err)
	}
	if err := w.Commit(); err != nil {
		t.Fatal(err)
	}
	if li, err := os.Lstat(link); err != nil || li.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link was replaced: %v %v", li, err)
	}
	if b, _ := os.ReadFile(real); string(b) != "new" {
		t.Fatalf("real file = %q, want new", b)
	}
	if runtime.GOOS != "windows" {
		if li, _ := os.Stat(real); li.Mode().Perm() != 0o640 {
			t.Fatalf("mode = %v, want 0640 preserved", li.Mode().Perm())
		}
	}
}
