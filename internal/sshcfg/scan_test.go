package sshcfg

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func fixtureHome(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", "home"))
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func aliases(hs []Host) []string {
	out := make([]string, 0, len(hs))
	for _, h := range hs {
		out = append(out, h.Alias)
	}
	return out
}

func equal(a, b []string) bool { return slices.Equal(a, b) }

func TestListHostsFixture(t *testing.T) {
	t.Parallel()
	home := fixtureHome(t)
	hs, err := listHosts(filepath.Join(home, ".ssh", "config"), home)
	if err != nil {
		t.Fatal(err)
	}
	// includes are expanded in place (sorted glob), cycles are tolerated, patterns skipped, first alias wins
	want := []string{"from-inc1", "from-inc2", "cyc2", "cyc", "web", "db", "eq", "lower", "quoted alias"}
	if got := aliases(hs); !equal(got, want) {
		t.Fatalf("aliases = %q, want %q", got, want)
	}
	for _, h := range hs {
		if h.Alias == "web" {
			if h.Source != filepath.Join(home, ".ssh", "config")+":11" {
				t.Errorf("web source = %q", h.Source)
			}
		}
	}
}

func TestListHostsMissingFile(t *testing.T) {
	t.Parallel()
	hs, err := listHosts(filepath.Join(t.TempDir(), "nope"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if hs == nil || len(hs) != 0 {
		t.Fatalf("hosts = %#v, want empty non-nil", hs)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestListHostsIncludeForms(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	abs := filepath.Join(t.TempDir(), "abs.conf")
	write(t, abs, "Host from-abs\n")
	write(t, filepath.Join(home, "tilde.conf"), "Host from-tilde\n")
	write(t, filepath.Join(home, ".ssh", "rel.conf"), "Host from-rel\n")
	write(t, filepath.Join(home, ".ssh", "config"),
		"Host first\nInclude rel.conf ~/tilde.conf\nInclude "+abs+"\nHost last\n")
	hs, err := listHosts(filepath.Join(home, ".ssh", "config"), home)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"first", "from-rel", "from-tilde", "from-abs", "last"}
	if got := aliases(hs); !equal(got, want) {
		t.Fatalf("aliases = %q, want %q", got, want)
	}
}

func TestListHostsIncludeDepthLimit(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	ssh := filepath.Join(home, ".ssh")
	const n = 20
	for i := range n {
		body := "Host h" + string(rune('a'+i)) + "\n"
		if i+1 < n {
			body += "Include f" + string(rune('a'+i+1)) + "\n"
		}
		name := "config"
		if i > 0 {
			name = "f" + string(rune('a'+i))
		}
		write(t, filepath.Join(ssh, name), body)
	}
	hs, err := listHosts(filepath.Join(ssh, "config"), home)
	if err != nil {
		t.Fatal(err)
	}
	if len(hs) != maxIncludeDepth+1 {
		t.Fatalf("got %d hosts, want %d", len(hs), maxIncludeDepth+1)
	}
}

func TestListHostsToleratesMatchExecAndJunk(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	p := filepath.Join(home, "c")
	write(t, p, "Match exec \"test -f /x && echo \\\"y\\\"\"\n  Host inmatch\nBogusKeyword 1 2 3\nHost\nHost after   # c\n\"unterminated\n")
	hs, err := listHosts(p, home)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := aliases(hs), []string{"inmatch", "after"}; !equal(got, want) {
		t.Fatalf("aliases = %q, want %q", got, want)
	}
}
