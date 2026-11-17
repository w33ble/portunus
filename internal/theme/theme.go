// Package theme is the backend's view of the built-in color themes: which IDs exist, whether
// each is dark, and its background, so the native window and the first paint match the saved
// theme. The palettes themselves live in frontend/src/themes.
package theme

import (
	"fmt"
	"slices"
)

// System follows the OS appearance, choosing between the Light and Dark themes.
const System = "system"

// IDs of the default themes System resolves to.
const (
	Light = "light"
	Dark  = "dark"
)

// RGB is an opaque color.
type RGB struct{ R, G, B uint8 }

// Theme is one built-in palette. Background is its `--bg` token.
type Theme struct {
	ID         string
	Dark       bool
	Background RGB
}

var builtIn = []Theme{
	{ID: Light, Background: RGB{0xff, 0xff, 0xff}},
	{ID: Dark, Dark: true, Background: RGB{0x1e, 0x1e, 0x1e}},
	{ID: "solarized-light", Background: RGB{0xfd, 0xf6, 0xe3}},
	{ID: "solarized-dark", Dark: true, Background: RGB{0x00, 0x2b, 0x36}},
	{ID: "nord-light", Background: RGB{0xec, 0xef, 0xf4}},
	{ID: "nord-dark", Dark: true, Background: RGB{0x2e, 0x34, 0x40}},
	{ID: "catppuccin-latte", Background: RGB{0xef, 0xf1, 0xf5}},
	{ID: "catppuccin-mocha", Dark: true, Background: RGB{0x1e, 0x1e, 0x2e}},
	{ID: "everforest-light", Background: RGB{0xfd, 0xf6, 0xe3}},
	{ID: "everforest-dark", Dark: true, Background: RGB{0x2d, 0x35, 0x3b}},
	{ID: "tokyo-night-day", Background: RGB{0xe1, 0xe2, 0xe7}},
	{ID: "tokyo-night", Dark: true, Background: RGB{0x1a, 0x1b, 0x26}},
}

// All returns the built-in themes in display order.
func All() []Theme {
	return slices.Clone(builtIn)
}

// Valid reports whether id is System or a built-in theme.
func Valid(id string) bool {
	if id == System {
		return true
	}
	_, ok := lookup(id)
	return ok
}

// Resolve returns the theme a setting applies: System picks Light or Dark from systemDark, and
// an unknown ID falls back the same way.
func Resolve(id string, systemDark bool) Theme {
	if t, ok := lookup(id); ok {
		return t
	}
	t, _ := lookup(Light)
	if systemDark {
		t, _ = lookup(Dark)
	}
	return t
}

// Script is JavaScript that sets the root element's data-theme before the page paints. System is
// resolved in the page, where the OS appearance is known.
func Script(id string) []byte {
	if t, ok := lookup(id); ok {
		return fmt.Appendf(nil, "document.documentElement.dataset.theme=%q;\n", t.ID)
	}
	return fmt.Appendf(nil,
		"document.documentElement.dataset.theme=matchMedia('(prefers-color-scheme: dark)').matches?%q:%q;\n",
		Dark, Light)
}

func lookup(id string) (Theme, bool) {
	i := slices.IndexFunc(builtIn, func(t Theme) bool { return t.ID == id })
	if i < 0 {
		return Theme{}, false
	}
	return builtIn[i], true
}
