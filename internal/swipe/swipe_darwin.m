//go:build darwin && !purego

#import <AppKit/AppKit.h>
#include <stdbool.h>

extern void portunusSwiped(double deltaX);

static id monitor;

// A local monitor sees gesture events before any view does, so the swipe is reported no
// matter which view is under the pointer (WKWebView ignores them unless its own
// back/forward gestures are on). The event goes on to the window unchanged.
void portunusWatchSwipes(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		monitor = [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskSwipe
		                                                handler:^NSEvent *(NSEvent *event) {
			if (event.deltaX != 0) portunusSwiped(event.deltaX);
			return event;
		}];
	});
}

bool portunusSwipeFromScroll(void) {
	return [NSEvent isSwipeTrackingFromScrollEventsEnabled];
}
