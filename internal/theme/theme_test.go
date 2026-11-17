package theme

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestResolveFollowsSystemOnlyForSystemAndUnknownIDs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		id         string
		systemDark bool
		want       string
	}{
		{System, false, Light},
		{System, true, Dark},
		{"nord-dark", false, "nord-dark"},
		{"solarized-light", true, "solarized-light"},
		{"gone", true, Dark},
		{"", false, Light},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%s/dark=%v", c.id, c.systemDark), func(t *testing.T) {
			t.Parallel()
			if got := Resolve(c.id, c.systemDark).ID; got != c.want {
				t.Fatalf("Resolve(%q, %v) = %q, want %q", c.id, c.systemDark, got, c.want)
			}
		})
	}
}

func TestScriptSetsSavedThemeOrResolvesSystemInPage(t *testing.T) {
	t.Parallel()
	if got, want := string(Script("tokyo-night")), `document.documentElement.dataset.theme="tokyo-night";`+"\n"; got != want {
		t.Fatalf("Script(tokyo-night) = %q, want %q", got, want)
	}
	sys := string(Script(System))
	if !strings.Contains(sys, "prefers-color-scheme: dark") || !strings.Contains(sys, `"dark":"light"`) {
		t.Fatalf("Script(system) = %q, want a prefers-color-scheme choice between dark and light", sys)
	}
	if string(Script("</script><b>")) != sys {
		t.Fatal("an unknown ID must not reach the script")
	}
}

// The palettes live in CSS; the catalog must agree on which themes exist, which are dark, and their
// background, or the native window flashes a different color before the page paints.
func TestCatalogMatchesFrontendThemes(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("..", "..", "frontend", "src", "themes")
	files, err := filepath.Glob(filepath.Join(dir, "*.css"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no theme CSS in %s: %v", dir, err)
	}

	blocks := regexp.MustCompile(`(?s)([^{}]*)\{([^{}]*)\}`)
	ids := regexp.MustCompile(`\[data-theme='([^']+)'\]`)
	bgDecl := regexp.MustCompile(`--bg:\s*#([0-9a-f]{6});`)
	scheme := regexp.MustCompile(`color-scheme:\s*(light|dark);`)

	dark := map[string]bool{}
	bg := map[string]string{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range blocks.FindAllStringSubmatch(string(raw), -1) {
			for _, id := range ids.FindAllStringSubmatch(m[1], -1) {
				if s := scheme.FindStringSubmatch(m[2]); s != nil {
					dark[id[1]] = s[1] == "dark"
				}
				if b := bgDecl.FindStringSubmatch(m[2]); b != nil {
					bg[id[1]] = b[1]
				}
			}
		}
	}

	for _, th := range All() {
		isDark, ok := dark[th.ID]
		if !ok {
			t.Errorf("%s: missing from the light or dark group in schemes.css", th.ID)
		} else if isDark != th.Dark {
			t.Errorf("%s: CSS dark=%v, catalog dark=%v", th.ID, isDark, th.Dark)
		}
		want := fmt.Sprintf("%02x%02x%02x", th.Background.R, th.Background.G, th.Background.B)
		if bg[th.ID] != want {
			t.Errorf("%s: CSS --bg #%s, catalog #%s", th.ID, bg[th.ID], want)
		}
		delete(dark, th.ID)
		delete(bg, th.ID)
	}
	for id := range dark {
		t.Errorf("%s: in CSS but not in the catalog", id)
	}
	for id := range bg {
		t.Errorf("%s: in CSS but not in the catalog", id)
	}
}

// The settings UI lists themes from frontend/src/lib/theme.ts; it must offer exactly this catalog.
func TestCatalogMatchesSettingsList(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("..", "..", "frontend", "src", "lib", "theme.ts"))
	if err != nil {
		t.Fatal(err)
	}
	entry := regexp.MustCompile(`\{ id: '([^']+)', name: '[^']+', dark: (true|false) \}`)
	var got []string
	for _, m := range entry.FindAllStringSubmatch(string(raw), -1) {
		got = append(got, fmt.Sprintf("%s dark=%s", m[1], m[2]))
	}
	var want []string
	for _, th := range All() {
		want = append(want, fmt.Sprintf("%s dark=%v", th.ID, th.Dark))
	}
	if !slices.Equal(got, want) {
		t.Fatalf("lib/theme.ts THEMES = %v, want %v", got, want)
	}
}
