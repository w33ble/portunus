//go:build !race

package transfer

import "testing"

func skipUnderRace(*testing.T) {}
