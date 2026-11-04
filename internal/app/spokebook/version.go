package spokebook

import "runtime/debug"

// Version is set at build time:
//
//	go build -ldflags "-X github.com/spokebook/spokebook/internal/app/spokebook.Version=v0.1.0"
//
// Without it, AppVersion falls back to the commit Go records in the binary.
var Version = ""

// SourceURL is where users get the source code of the running version, as the
// AGPL asks for a network service. Point it at your copy if you change the
// code:
//
//	go build -ldflags "-X github.com/spokebook/spokebook/internal/app/spokebook.SourceURL=https://example.com/spokebook"
var SourceURL = "https://github.com/spokebook/spokebook"

// AppVersion returns Version, else the short commit hash (with "-dirty" for
// uncommitted changes), else "dev".
func AppVersion() string {
	if Version != "" {
		return Version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	var rev string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return "dev"
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	if dirty {
		rev += "-dirty"
	}
	return rev
}
