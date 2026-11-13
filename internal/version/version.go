// Package version reports what build of Portunus is running.
//
// The build stamps the values with -ldflags -X (see the root Taskfile): releases get their
// semver, everything else "dev-<commit>". Plain `go build`/`go run` falls back to the VCS
// information Go embeds, so even unstamped binaries identify their commit.
package version

import (
	"runtime"
	"runtime/debug"
)

// Set by -ldflags -X github.com/w33ble/portunus/internal/version.<name>=<value>.
var (
	version string
	commit  string
	date    string
)

// Info describes the running build.
type Info struct {
	Version string `json:"version"` // "1.2.0" for releases, "dev-<commit>[-dirty]" otherwise
	Commit  string `json:"commit"`  // short SHA, "" when unknown
	Date    string `json:"date"`    // RFC 3339 build or commit time, "" when unknown
	Go      string `json:"go"`
}

// Release reports whether this is a tagged release build rather than a dev build.
func (i Info) Release() bool { return i.Version != "" && i.Version[0] >= '0' && i.Version[0] <= '9' }

// Get returns the stamped build info, filling gaps from Go's embedded VCS settings.
func Get() Info {
	bi, _ := debug.ReadBuildInfo()
	return resolve(version, commit, date, bi)
}

func resolve(v, c, d string, bi *debug.BuildInfo) Info {
	info := Info{Version: v, Commit: c, Date: d, Go: runtime.Version()}
	if bi != nil {
		var dirty bool
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				if info.Commit == "" {
					info.Commit = s.Value
				}
			case "vcs.time":
				if info.Date == "" {
					info.Date = s.Value
				}
			case "vcs.modified":
				dirty = s.Value == "true"
			}
		}
		if info.Version == "" && info.Commit != "" && dirty {
			info.Version = "dev-" + short(info.Commit) + "-dirty"
		}
	}
	info.Commit = short(info.Commit)
	if info.Version == "" {
		info.Version = "dev"
		if info.Commit != "" {
			info.Version = "dev-" + info.Commit
		}
	}
	return info
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
