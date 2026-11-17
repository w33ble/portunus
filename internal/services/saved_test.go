package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/w33ble/portunus/internal/connprefs"
	"github.com/w33ble/portunus/internal/profiles"
)

func newSavedApp(t *testing.T, sshConfig string) *ConnectionService {
	t.Helper()
	app, err := New(Config{ConfigDir: t.TempDir(), Emit: func(string, any) {}, Secrets: profiles.Memory()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	app.sshConfig = sshConfig
	return &ConnectionService{app: app}
}

func writeSSHConfig(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSavedDropsPrefsOfAliasesRemovedFromSSHConfig(t *testing.T) {
	t.Parallel()
	cfg := filepath.Join(t.TempDir(), "config")
	writeSSHConfig(t, cfg, "Host web1\n  HostName web1.example.com\nHost gone\n  HostName gone.example.com\n")
	s := newSavedApp(t, cfg)

	for _, id := range []string{"ssh:web1", "ssh:gone"} {
		if err := s.SaveConnectionPrefs(id, connprefs.Prefs{Color: connprefs.ColorRed}, []connprefs.Bookmark{{Path: "/var/log"}}); err != nil {
			t.Fatal(err)
		}
	}
	writeSSHConfig(t, cfg, "Host web1\n  HostName web1.example.com\n")

	d, err := s.Saved()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := d.Prefs["ssh:gone"]; ok || len(d.Prefs) != 1 {
		t.Fatalf("prefs = %+v", d.Prefs)
	}
	if len(d.Bookmarks) != 1 || d.Bookmarks[0].ConnID != "ssh:web1" {
		t.Fatalf("bookmarks = %+v", d.Bookmarks)
	}
}

func TestSavedKeepsAliasPrefsWhileSSHConfigIsUnreadable(t *testing.T) {
	t.Parallel()
	cfg := filepath.Join(t.TempDir(), "config")
	writeSSHConfig(t, cfg, "Host web1\n  HostName web1.example.com\n")
	s := newSavedApp(t, cfg)
	if err := s.SaveConnectionPrefs("ssh:web1", connprefs.Prefs{OpenAt: "/srv"}, []connprefs.Bookmark{{Path: "/srv"}}); err != nil {
		t.Fatal(err)
	}

	// A directory where the config file should be makes ListHosts fail.
	if err := os.Remove(cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(cfg, 0o700); err != nil {
		t.Fatal(err)
	}

	d, err := s.Saved()
	if err != nil {
		t.Fatal(err)
	}
	if d.Prefs["ssh:web1"].OpenAt != "/srv" {
		t.Fatalf("prefs dropped while the config was unreadable: %+v", d.Prefs)
	}
}

func TestBookmarkingNeedsASavedConnection(t *testing.T) {
	t.Parallel()
	s := newSavedApp(t, filepath.Join(t.TempDir(), "missing"))
	quick := connprefs.Bookmark{ConnID: "ssh:joe@box.lan:2222", Path: "/srv"}

	if _, err := s.AddBookmark(quick); err == nil {
		t.Fatal("bookmarked a quick connect that isn't saved")
	}
	if _, err := s.SaveSSHHost(connprefs.Host{Name: "box", User: "joe", Host: "box.lan", Port: 2222}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddBookmark(quick); err != nil {
		t.Fatalf("bookmark on a saved host: %v", err)
	}
	if _, err := s.AddBookmark(connprefs.Bookmark{ConnID: LocalID, Path: "/tmp"}); err != nil {
		t.Fatalf("bookmark on local: %v", err)
	}
}

func TestDeletingAnS3ProfileForgetsItsPrefs(t *testing.T) {
	t.Parallel()
	s := newSavedApp(t, filepath.Join(t.TempDir(), "missing"))
	p, err := s.SaveS3Profile(profiles.S3Profile{Name: "bucket", Profile: "default"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveConnectionPrefs("s3:"+p.ID, connprefs.Prefs{Color: connprefs.ColorBlue}, []connprefs.Bookmark{{Path: "/b/logs"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteS3Profile(p.ID); err != nil {
		t.Fatal(err)
	}
	if d := s.app.conns.Data(); len(d.Prefs) != 0 || len(d.Bookmarks) != 0 {
		t.Fatalf("left behind: %+v", d)
	}
}
