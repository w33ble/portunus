package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteReplacesAndLeavesNoTempFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested")
	path := filepath.Join(dir, "f.json")

	for _, content := range []string{"one", "two"} {
		if err := Write(path, []byte(content)); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != content {
			t.Fatalf("read = %q, %v; want %q", got, err, content)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("dir has %d entries, want only the target file", len(entries))
	}
	if info, _ := os.Stat(path); info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("mode = %v, want owner-only", info.Mode().Perm())
	}
}
