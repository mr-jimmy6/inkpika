package buildinfo

import (
	"html"
	"strings"
)

// HTML fills metadata placeholders in embedded pages before localization.
// It renders the binary's build identity even before a LAN visitor pairs.
func HTML(data []byte) []byte {
	built := PackagedAt()
	if built == "" {
		built = "Not packaged (source run)"
	}
	return []byte(strings.NewReplacer(
		"@BUILD_VERSION@", html.EscapeString(Version()),
		"@BUILD_TIME@", html.EscapeString(built),
	).Replace(string(data)))
}
