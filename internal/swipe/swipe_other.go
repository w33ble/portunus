//go:build !darwin || purego

package swipe

// Watch does nothing here: only macOS sends swipe gestures as their own events.
func Watch(func(Direction)) {}

// FromScroll is true: Linux touchpads send swipes as horizontal scrolls, and the CGO-free
// macOS build can't read the system setting.
func FromScroll() bool { return true }
