package connprefs

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func open(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestHostsPrefsAndBookmarksSurviveReopen(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := open(t, dir)

	h, err := s.SaveHost(Host{Name: " Staging ", User: "deploy", Host: "10.0.4.12", Port: 2222})
	if err != nil {
		t.Fatal(err)
	}
	if h.ID == "" || h.Name != "Staging" {
		t.Fatalf("saved host = %+v", h)
	}
	show := true
	prefs := Prefs{Color: ColorOrange, OpenAt: "/var/www", Transfers: 3, ShowHidden: &show, HideInPicker: true}
	if err := s.SetConnection(h.ConnID, prefs, []Bookmark{{Path: "/srv/app", Name: "app", Pinned: true}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddBookmark(Bookmark{ConnID: "local", Path: "/Users/joe/src"}); err != nil {
		t.Fatal(err)
	}

	got := open(t, dir).Data()
	if len(got.Hosts) != 1 || got.Hosts[0] != h {
		t.Fatalf("hosts = %+v", got.Hosts)
	}
	p := got.Prefs["ssh:deploy@10.0.4.12:2222"]
	if p.ShowHidden == nil || !*p.ShowHidden {
		t.Fatalf("showHidden = %v", p.ShowHidden)
	}
	p.ShowHidden, prefs.ShowHidden = nil, nil
	if p != prefs {
		t.Fatalf("prefs = %+v, want %+v", p, prefs)
	}
	if len(got.Bookmarks) != 2 || slices.ContainsFunc(got.Bookmarks, func(b Bookmark) bool { return b.ID == "" }) {
		t.Fatalf("bookmarks = %+v", got.Bookmarks)
	}
}

func TestUnknownSectionsSurviveARewriteAndUnknownFieldsAreIgnored(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, fileName)
	raw := `{
  "hosts": [{"id": "h1", "name": "nas", "host": "nas.lan", "future": 1}],
  "prefs": {"local": {"color": "green", "later": "x"}},
  "bookmarks": [],
  "crypt": [{"prefix": "/vault"}]
}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	s := open(t, dir)
	if d := s.Data(); len(d.Hosts) != 1 || d.Hosts[0].Host != "nas.lan" || d.Prefs["local"].Color != ColorGreen {
		t.Fatalf("loaded = %+v", d)
	}
	if _, err := s.AddBookmark(Bookmark{ConnID: "local", Path: "/tmp"}); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"crypt"`) || !strings.Contains(string(b), `"/vault"`) {
		t.Fatalf("unknown section dropped on rewrite:\n%s", b)
	}
}

func TestPruneDropsDataOfConnectionsThatNoLongerExist(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := open(t, dir)
	for _, id := range []string{"ssh:web1", "ssh:gone", "s3:old"} {
		if err := s.SetConnection(id, Prefs{Color: ColorBlue}, []Bookmark{{Path: "/x"}}); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.Prune(func(id string) bool { return id == "ssh:web1" }); err != nil {
		t.Fatal(err)
	}

	for _, d := range []Data{s.Data(), open(t, dir).Data()} {
		if _, ok := d.Prefs["ssh:web1"]; len(d.Prefs) != 1 || !ok {
			t.Fatalf("prefs = %+v", d.Prefs)
		}
		if len(d.Bookmarks) != 1 || d.Bookmarks[0].ConnID != "ssh:web1" {
			t.Fatalf("bookmarks = %+v", d.Bookmarks)
		}
	}
}

func TestPruneWithNothingStaleDoesNotWrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := open(t, dir).Prune(func(string) bool { return false }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, fileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("prune of an empty store wrote the file: %v", err)
	}
}

func TestChangingASavedHostsAddressMovesItsPrefsAndBookmarks(t *testing.T) {
	t.Parallel()
	s := open(t, t.TempDir())
	h, _ := s.SaveHost(Host{Name: "box", Host: "old.lan"})
	if err := s.SetConnection(h.ConnID, Prefs{OpenAt: "/srv"}, []Bookmark{{Path: "/srv/logs"}}); err != nil {
		t.Fatal(err)
	}

	h.Host, h.Port = "new.lan", 2200
	if _, err := s.SaveHost(h); err != nil {
		t.Fatal(err)
	}

	d := s.Data()
	if _, ok := d.Prefs["ssh:old.lan"]; ok {
		t.Fatal("prefs left under the old ID")
	}
	if d.Prefs["ssh:new.lan:2200"].OpenAt != "/srv" {
		t.Fatalf("prefs = %+v", d.Prefs)
	}
	if len(d.Bookmarks) != 1 || d.Bookmarks[0].ConnID != "ssh:new.lan:2200" {
		t.Fatalf("bookmarks = %+v", d.Bookmarks)
	}
}

func TestSavingTheSameAddressTwiceIsRejected(t *testing.T) {
	t.Parallel()
	s := open(t, t.TempDir())
	if _, err := s.SaveHost(Host{Name: "a", User: "u", Host: "h"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveHost(Host{Name: "b", User: "u", Host: "h"}); err == nil || !strings.Contains(err.Error(), `already saved as "a"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestDeletingAHostForgetsItsPrefsAndBookmarks(t *testing.T) {
	t.Parallel()
	s := open(t, t.TempDir())
	h, _ := s.SaveHost(Host{Name: "box", Host: "box.lan"})
	_ = s.SetConnection(h.ConnID, Prefs{Color: ColorRed}, []Bookmark{{Path: "/a"}})
	_ = s.SetConnection("local", Prefs{Color: ColorGreen}, []Bookmark{{Path: "/b"}})

	if err := s.DeleteHost(h.ID); err != nil {
		t.Fatal(err)
	}
	d := s.Data()
	if len(d.Hosts) != 0 || len(d.Prefs) != 1 || len(d.Bookmarks) != 1 || d.Bookmarks[0].ConnID != "local" {
		t.Fatalf("after delete = %+v", d)
	}
	if err := s.DeleteHost(h.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete err = %v", err)
	}
}

func TestSetConnectionReplacesOnlyThatConnection(t *testing.T) {
	t.Parallel()
	s := open(t, t.TempDir())
	_ = s.SetConnection("local", Prefs{Color: ColorGreen}, []Bookmark{{Path: "/keep"}})
	_ = s.SetConnection("ssh:web1", Prefs{Color: ColorRed}, []Bookmark{{Path: "/old"}})

	err := s.SetConnection("ssh:web1", Prefs{}, []Bookmark{{Path: "/new"}, {Path: "/new", Name: "dupe"}})
	if err != nil {
		t.Fatal(err)
	}

	d := s.Data()
	if _, ok := d.Prefs["ssh:web1"]; ok {
		t.Fatal("zero prefs should remove the entry")
	}
	paths := map[string]string{}
	for _, b := range d.Bookmarks {
		paths[b.Path] = b.ConnID
	}
	if len(d.Bookmarks) != 2 || paths["/keep"] != "local" || paths["/new"] != "ssh:web1" {
		t.Fatalf("bookmarks = %+v", d.Bookmarks)
	}
}

func TestInvalidPrefsAndBookmarksAreRejected(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		prefs     Prefs
		bookmarks []Bookmark
	}{
		{name: "unknown colour", prefs: Prefs{Color: "teal"}},
		{name: "relative open-at", prefs: Prefs{OpenAt: "var/www"}},
		{name: "negative transfers", prefs: Prefs{Transfers: -1}},
		{name: "too many transfers", prefs: Prefs{Transfers: MaxTransfers + 1}},
		{name: "relative bookmark", bookmarks: []Bookmark{{Path: "logs"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := open(t, t.TempDir())
			if err := s.SetConnection("local", tc.prefs, tc.bookmarks); err == nil {
				t.Fatal("expected an error")
			}
			if d := s.Data(); len(d.Prefs) != 0 || len(d.Bookmarks) != 0 {
				t.Fatalf("rejected save was stored: %+v", d)
			}
		})
	}
}

func TestWindowsDrivePathsAreAbsolute(t *testing.T) {
	t.Parallel()
	s := open(t, t.TempDir())
	if err := s.SetConnection("local", Prefs{OpenAt: "C:/Users/joe"}, []Bookmark{{Path: "d:/data"}}); err != nil {
		t.Fatal(err)
	}
}

func TestBookmarkingAFolderTwiceReturnsTheFirstBookmark(t *testing.T) {
	t.Parallel()
	s := open(t, t.TempDir())
	first, err := s.AddBookmark(Bookmark{ConnID: "local", Path: "/tmp", Name: "scratch"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.AddBookmark(Bookmark{ConnID: "local", Path: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	if second != first || len(s.Data().Bookmarks) != 1 {
		t.Fatalf("first = %+v, second = %+v", first, second)
	}
	if err := s.RemoveBookmark(first.ID); err != nil {
		t.Fatal(err)
	}
	if len(s.Data().Bookmarks) != 0 {
		t.Fatal("bookmark not removed")
	}
}

func TestDataIsACopy(t *testing.T) {
	t.Parallel()
	s := open(t, t.TempDir())
	show := true
	_ = s.SetConnection("local", Prefs{ShowHidden: &show}, nil)

	d := s.Data()
	*d.Prefs["local"].ShowHidden = false
	d.Prefs["other"] = Prefs{Color: ColorRed}

	again := s.Data()
	if !*again.Prefs["local"].ShowHidden || len(again.Prefs) != 1 {
		t.Fatalf("store mutated through a returned copy: %+v", again.Prefs)
	}
}
