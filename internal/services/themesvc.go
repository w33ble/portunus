package services

import (
	"net/http"

	"github.com/w33ble/portunus/internal/theme"
)

// ThemeScriptRoute is where the asset server mounts ThemeService.
const ThemeScriptRoute = "/theme.js"

// ThemeService serves a script that applies the saved theme. index.html loads it as a blocking
// script in <head>, so the first paint already has the theme's colours.
type ThemeService struct {
	app *App
}

func (s *ThemeService) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "text/javascript; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	_, _ = w.Write(theme.Script(s.app.settings.Get().Theme))
}

// Theme returns the saved theme ID, for styling the native window.
func (a *App) Theme() string {
	return a.settings.Get().Theme
}
