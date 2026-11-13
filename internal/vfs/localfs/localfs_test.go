package localfs_test

import (
	"errors"
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

func TestCaseInsensitiveByPlatform(t *testing.T) {
	want := runtime.GOOS == "darwin" || runtime.GOOS == "windows"
	if got := vfs.IsCaseInsensitive(localfs.New()); got != want {
		t.Fatalf("IsCaseInsensitive = %v on %s, want %v", got, runtime.GOOS, want)
	}
}

// A rename whose target resolves to the source itself (foo -> Foo on a case-insensitive
// volume) must not be reported as an existing-target conflict. A hard link reproduces
// the same-file condition on any platform.
func TestRenameOntoSameFileAllowed(t *testing.T) {
	dir := t.TempDir()
	from, to := filepath.Join(dir, "foo"), filepath.Join(dir, "Foo2")
	if err := os.WriteFile(from, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(from, to); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	if err := localfs.New().Rename(t.Context(), filepath.ToSlash(from), filepath.ToSlash(to)); err != nil {
		t.Fatalf("Rename onto same file: %v", err)
	}
}

func TestRenameOntoDifferentFileRefused(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := localfs.New().Rename(t.Context(), filepath.ToSlash(a), filepath.ToSlash(b)); !errors.Is(err, vfs.ErrExist) {
		t.Fatalf("err = %v, want ErrExist", err)
	}
}
