package state

import "strings"

// CanPositionWindows reports whether the app can read and set window positions. Wayland
// compositors own window placement, so only the size is restorable there.
func CanPositionWindows(goos string, getenv func(string) string) bool {
	if goos != "linux" || getenv("WAYLAND_DISPLAY") == "" {
		return true
	}
	// GDK_BACKEND is a preference list; an X11 entry first means GTK runs on X11 (or XWayland).
	first, _, _ := strings.Cut(getenv("GDK_BACKEND"), ",")
	return first == "x11"
}
