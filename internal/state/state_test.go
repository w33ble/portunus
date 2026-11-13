package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenMissingFileHasNoWindow(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if w, ok := s.Window(); ok {
		t.Fatalf("Window = %+v, want none", w)
	}
}

func TestWindowRoundTripsThroughDisk(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "portunus")
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []Window{
		{Width: 1280, Height: 800, Position: &Point{-1500, 40}, Maximised: true},
		{Width: 900, Height: 600},
	} {
		if err := s.SetWindow(want); err != nil {
			t.Fatal(err)
		}
		reopened, err := Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := reopened.Window()
		if !ok || !got.equal(want) {
			t.Fatalf("reopened = %+v, %v; want %+v", got, ok, want)
		}
	}

	info, err := os.Stat(filepath.Join(dir, fileName))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("mode = %v, want owner-only", info.Mode().Perm())
	}
}

func TestOnDiskFormat(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	if err := s.SetWindow(Window{Width: 1000, Height: 700, Position: &Point{10, 20}, Maximised: true}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, fileName))
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"window\": {\n    \"width\": 1000,\n    \"height\": 700,\n    \"position\": {\n      \"x\": 10,\n      \"y\": 20\n    },\n    \"maximised\": true\n  }\n}\n"
	if string(raw) != want {
		t.Fatalf("state.json = %q, want %q", raw, want)
	}
}

func TestOpenIgnoresUnknownKeysAndSurvivesCorruption(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, fileName)

	if err := os.WriteFile(path, []byte(`{"panes": {"left": "/"}, "window": {"width": 800, "height": 600, "dock": "left"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if w, ok := s.Window(); !ok || w.Width != 800 || w.Height != 600 {
		t.Fatalf("Window = %+v, %v; want the 800x600 frame despite unknown keys", w, ok)
	}

	if err := os.WriteFile(path, []byte(`{nope`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatalf("corrupt state blocked startup: %v", err)
	}
	if _, ok := s.Window(); ok {
		t.Fatal("corrupt file produced a window")
	}
	if err := s.SetWindow(Window{Width: 1, Height: 1}); err != nil {
		t.Fatal(err)
	}
	if s, _ = Open(dir); s == nil {
		t.Fatal("reopen failed")
	}
	if _, ok := s.Window(); !ok {
		t.Fatal("state not rewritten over the corrupt file")
	}
}
