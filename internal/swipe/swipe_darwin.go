//go:build darwin && !purego

package swipe

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit

#include <stdbool.h>

void portunusWatchSwipes(void);
bool portunusSwipeFromScroll(void);
*/
import "C"

import "sync"

var (
	mu      sync.Mutex
	onSwipe func(Direction)
	once    sync.Once
)

// Watch reports every OS swipe to fn for the life of the process. Call it once the app is
// running: the event monitor needs NSApp.
func Watch(fn func(Direction)) {
	mu.Lock()
	onSwipe = fn
	mu.Unlock()
	once.Do(func() { C.portunusWatchSwipes() })
}

// FromScroll reports whether the user's "Swipe between pages" setting uses two-finger
// scrolls (NSEvent.isSwipeTrackingFromScrollEventsEnabled). It is read on every call, so a
// change in System Settings applies right away.
func FromScroll() bool {
	return bool(C.portunusSwipeFromScroll())
}

//export portunusSwiped
func portunusSwiped(deltaX C.double) {
	d, ok := direction(float64(deltaX))
	if !ok {
		return
	}
	mu.Lock()
	fn := onSwipe
	mu.Unlock()
	if fn != nil {
		fn(d)
	}
}
