package sftpfs_test

import (
	"os"
	"testing"

	"github.com/w33ble/portunus/internal/vfs"
	"github.com/w33ble/portunus/internal/vfs/vfstest"
)

// Paths are real local paths: sshd runs as the test user on this machine, so t.TempDir()
// is visible to both sides and symlinks can be made with os.Symlink.
func TestConformance(t *testing.T) {
	r := newRig(t)
	vfstest.Run(t, vfstest.Harness{
		New: func(t *testing.T) (vfs.FS, string) {
			return r.connect(t), t.TempDir()
		},
		Caps: vfstest.Caps{Symlinks: true, Modes: true, ModTimes: true, EmptyDirs: true, RenameDirs: true},
		Symlink: func(t *testing.T, target, link string) {
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
		},
	})
}
