package ui

import "github.com/JastRedPanda/Nimbus/internal/build"

// versionLine is what the About window shows underneath the subtitle.
//
// The version and build date are injected at link time with -X, which only
// works on package-level string VARIABLES - on a constant the flag is silently
// ignored, which is worth remembering the next time the value looks stale.
func versionLine() string { return formatVersion(build.Version, build.Date) }

// formatVersion joins the two injected values, dropping either when it is only
// the placeholder a build without -X leaves behind. Showing "dev · unknown"
// would be noise pretending to be information.
func formatVersion(version, date string) string {
	if version == "" {
		version = "dev"
	}
	if version == "dev" || date == "" || date == "unknown" {
		return version
	}
	return version + " · " + date
}
