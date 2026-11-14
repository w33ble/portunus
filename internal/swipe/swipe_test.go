package swipe

import "testing"

func TestDirectionFollowsSafariAndFirefox(t *testing.T) {
	t.Parallel()
	cases := []struct {
		deltaX float64
		want   Direction
		ok     bool
	}{
		{1, Back, true},
		{-1, Forward, true},
		{0, "", false},
	}
	for _, c := range cases {
		got, ok := direction(c.deltaX)
		if got != c.want || ok != c.ok {
			t.Errorf("direction(%v) = %q, %v; want %q, %v", c.deltaX, got, ok, c.want, c.ok)
		}
	}
}
