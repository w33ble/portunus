//go:build race

package sftpfs_test

import "testing"

// skipUnderRace skips throughput tests: the race detector makes timings meaningless on shared
// CI runners. The non-race test job still enforces the throughput floors.
func skipUnderRace(t *testing.T) {
	t.Helper()
	t.Skip("throughput is not measurable under the race detector")
}
