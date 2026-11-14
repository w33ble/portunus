// Package swipe reports the operating system's back/forward swipe gestures.
//
// Only macOS has them. When System Settings maps "Swipe between pages" to three fingers,
// each swipe arrives as one discrete gesture event, which Watch reports. When it maps the
// gesture to two fingers, the OS sends plain scroll events and every app (Safari included)
// interprets them itself; FromScroll tells the frontend whether to.
package swipe

// Direction is where a swipe goes in the pane's history.
type Direction string

const (
	Back    Direction = "back"
	Forward Direction = "forward"
)

// direction maps a gesture's horizontal delta to a history step, as Firefox and Safari do:
// a positive deltaX (AppKit's "left" swipe) goes back.
func direction(deltaX float64) (Direction, bool) {
	switch {
	case deltaX > 0:
		return Back, true
	case deltaX < 0:
		return Forward, true
	default:
		return "", false
	}
}
