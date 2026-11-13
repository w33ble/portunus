package version

import (
	"runtime/debug"
	"testing"
)

func buildInfo(settings ...string) *debug.BuildInfo {
	bi := &debug.BuildInfo{}
	for i := 0; i+1 < len(settings); i += 2 {
		bi.Settings = append(bi.Settings, debug.BuildSetting{Key: settings[i], Value: settings[i+1]})
	}
	return bi
}

func TestResolve(t *testing.T) {
	const sha = "abc1234def5678"
	cases := []struct {
		name                  string
		v, c, d               string
		bi                    *debug.BuildInfo
		version, commit, date string
		release               bool
	}{
		{
			name: "stamped release", v: "1.2.0", c: "abc1234", d: "2026-11-13T10:00:00Z",
			bi:      buildInfo("vcs.revision", "ffffffffffff"),
			version: "1.2.0", commit: "abc1234", date: "2026-11-13T10:00:00Z", release: true,
		},
		{
			name: "stamped dev build", v: "dev-abc1234-dirty", c: "abc1234",
			version: "dev-abc1234-dirty", commit: "abc1234",
		},
		{
			name:    "unstamped go build with vcs info",
			bi:      buildInfo("vcs.revision", sha, "vcs.time", "2026-11-12T09:00:00Z", "vcs.modified", "false"),
			version: "dev-abc1234", commit: "abc1234", date: "2026-11-12T09:00:00Z",
		},
		{
			name:    "unstamped dirty tree",
			bi:      buildInfo("vcs.revision", sha, "vcs.modified", "true"),
			version: "dev-abc1234-dirty", commit: "abc1234",
		},
		{
			name:    "no information at all",
			version: "dev",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolve(tc.v, tc.c, tc.d, tc.bi)
			if got.Version != tc.version || got.Commit != tc.commit || got.Date != tc.date {
				t.Fatalf("got %+v, want version %q commit %q date %q", got, tc.version, tc.commit, tc.date)
			}
			if got.Release() != tc.release {
				t.Fatalf("Release() = %v, want %v", got.Release(), tc.release)
			}
			if got.Go == "" {
				t.Fatal("Go version empty")
			}
		})
	}
}
