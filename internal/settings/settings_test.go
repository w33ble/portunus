package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenMissingFileUsesDefaults(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Get(); got != Defaults() {
		t.Fatalf("got %+v, want defaults", got)
	}
}

func TestSaveRoundTripsThroughDisk(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "Portunus")
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	want := Settings{GlobalTransfers: 4, PerConnectionTransfers: 2, LoadAppleKeychain: false, ShowHidden: true, SwipeNavigation: false}
	if _, err := s.Save(want); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Get(); got != want {
		t.Fatalf("reopened = %+v, want %+v", got, want)
	}

	info, err := os.Stat(filepath.Join(dir, fileName))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("settings file mode = %v, want no group/other access", info.Mode().Perm())
	}
}

func TestSaveNormalizesLimits(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.Save(Settings{GlobalTransfers: 2, PerConnectionTransfers: 9})
	if err != nil {
		t.Fatal(err)
	}
	if got.PerConnectionTransfers != 2 {
		t.Fatalf("per-connection = %d, want capped at global 2", got.PerConnectionTransfers)
	}

	got, err = s.Save(Settings{GlobalTransfers: 0, PerConnectionTransfers: -1})
	if err != nil {
		t.Fatal(err)
	}
	if got.GlobalTransfers != 4 || got.PerConnectionTransfers != 2 {
		t.Fatalf("got %+v, want invalid limits replaced with defaults", got)
	}

	got, err = s.Save(Settings{GlobalTransfers: 1000, PerConnectionTransfers: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if got.GlobalTransfers != 32 || got.PerConnectionTransfers != 32 {
		t.Fatalf("got %+v, want limits capped at 32", got)
	}
}

func TestOpenKeepsDefaultsForMissingFields(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(`{"showHidden": true}`), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := s.Get()
	if !got.ShowHidden || !got.LoadAppleKeychain || got.GlobalTransfers != 4 {
		t.Fatalf("got %+v, want showHidden plus defaults", got)
	}
}

func TestOpenRejectsCorruptFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(`{nope`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("corrupt settings accepted")
	}
}

func TestOpenReadsFilesWrittenByOlderAndNewerVersions(t *testing.T) {
	dir := t.TempDir()
	// A future version's extra key is ignored; a differently cased key is not our field.
	raw := `{"globalTransfers": 8, "futureOption": {"a": 1}, "ShowHidden": true}`
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := s.Get()
	if got.GlobalTransfers != 8 || got.ShowHidden {
		t.Fatalf("got %+v, want globalTransfers=8 and showHidden left at its default", got)
	}
}

func TestOpenRejectsDuplicateKeys(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(`{"showHidden": true, "showHidden": false}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("duplicate keys accepted")
	}
}

func TestSaveKeepsOnDiskFieldNames(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(Defaults()); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, fileName))
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"globalTransfers\": 4,\n  \"perConnectionTransfers\": 2,\n  \"loadAppleKeychain\": true,\n  \"showHidden\": false,\n  \"swipeNavigation\": true\n}\n"
	if string(raw) != want {
		t.Fatalf("settings.json = %q, want %q", raw, want)
	}
}
