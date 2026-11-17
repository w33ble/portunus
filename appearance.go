package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/w33ble/portunus/internal/theme"
)

// windowColor is the theme's page background, for the native window behind the webview.
func windowColor(id string, systemDark bool) application.RGBA {
	bg := theme.Resolve(id, systemDark).Background
	return application.NewRGB(bg.R, bg.G, bg.B)
}

// macAppearance makes the macOS backdrop and title bar match a theme that overrides the system
// appearance. It is fixed when the window opens, which covers the moments before the page paints.
func macAppearance(id string) application.MacAppearanceType {
	switch {
	case id == theme.System:
		return application.DefaultAppearance
	case theme.Resolve(id, false).Dark:
		return application.NSAppearanceNameDarkAqua
	default:
		return application.NSAppearanceNameAqua
	}
}
