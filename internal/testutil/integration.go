// Package testutil holds helpers shared by integration tests.
package testutil

import (
	"os"
	"testing"
)

// RequireEnv makes missing integration dependencies (sshd, docker, ssh) fail instead of skip.
// CI sets it so a broken runner can't turn integration coverage into silent skips.
const RequireEnv = "PORTUNUS_REQUIRE_INTEGRATION"

// Unavailable skips t because an external dependency is missing, or fails it when
// RequireEnv is set.
func Unavailable(t testing.TB, format string, args ...any) {
	t.Helper()
	if os.Getenv(RequireEnv) != "" {
		t.Fatalf("%s is set but "+format, append([]any{RequireEnv}, args...)...)
	}
	t.Skipf(format, args...)
}
