// Package build holds what the program says about itself: who it is, which
// version it is, and when it was built.
//
// It lives apart from any user interface because every backend needs it and none
// of them owns it. The About window exists three times over - GTK, Win32, Qt -
// and each one carried its own copy of the subtitle; a string written out once
// per backend is a string that eventually says three different things.
package build

// Version and Date are injected at link time with -X.
//
// They must stay package-level string VARIABLES. On a constant the flag is
// silently ignored - no error, no warning, just the placeholder shipping to
// users, which is worth remembering the next time the version looks stale.
var (
	Version = "dev"
	Date    = "unknown"
)

// Subtitle is the one-line description under the name in the About window.
//
// Ukrainian and not translated, deliberately: it is the project's own tagline
// rather than interface text, and the same sentence is the package description in
// dist/. If it is ever localised it belongs in internal/i18n, not here.
const Subtitle = "Мультиплатформний інформер погоди."

// Line is what the About window shows underneath the subtitle: the version, with
// the build date when there is a real one to show.
func Line() string { return formatVersion(Version, Date) }

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
