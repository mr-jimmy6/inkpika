package buildinfo

import (
	"os"
	"strings"
	"testing"
)

func TestPackageTimeComesFromBuildMetadata(t *testing.T) {
	before := packagedAt
	t.Cleanup(func() { packagedAt = before })
	packagedAt = ""
	if PackagedAt() != "" {
		t.Fatal("source runs must not invent a package time")
	}
	packagedAt = "2026-11-09T01:02:03Z"
	expectedVersion, err := os.ReadFile("VERSION")
	if err != nil {
		t.Fatal(err)
	}
	if PackagedAt() != packagedAt || Version() != strings.TrimSpace(string(expectedVersion)) {
		t.Fatal("release metadata was not preserved")
	}
}
