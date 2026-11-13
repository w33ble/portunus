package state

import "testing"

func TestCanPositionWindows(t *testing.T) {
	tests := []struct {
		name string
		goos string
		env  map[string]string
		want bool
	}{
		{"macOS", "darwin", map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, true},
		{"Linux X11 session", "linux", map[string]string{"DISPLAY": ":0"}, true},
		{"Linux Wayland session", "linux", map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, false},
		{"Wayland session forced to X11", "linux", map[string]string{"WAYLAND_DISPLAY": "wayland-0", "GDK_BACKEND": "x11"}, true},
		{"X11 preferred in a backend list", "linux", map[string]string{"WAYLAND_DISPLAY": "wayland-0", "GDK_BACKEND": "x11,wayland"}, true},
		{"Wayland preferred in a backend list", "linux", map[string]string{"WAYLAND_DISPLAY": "wayland-0", "GDK_BACKEND": "wayland,x11"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanPositionWindows(tc.goos, func(k string) string { return tc.env[k] }); got != tc.want {
				t.Errorf("CanPositionWindows = %v, want %v", got, tc.want)
			}
		})
	}
}
