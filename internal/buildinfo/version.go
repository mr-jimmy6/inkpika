// Package buildinfo shares release metadata across the CLI, API and packaging.
package buildinfo

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var version string

func Version() string { return strings.TrimSpace(version) }

// packagedAt is injected once by the build script and shared with build.json.
// Source runs deliberately leave it empty instead of inventing a package time.
var packagedAt string

func PackagedAt() string { return packagedAt }
