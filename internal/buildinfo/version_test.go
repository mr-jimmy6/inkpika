package buildinfo

import "testing"

func TestPackageTimeComesFromBuildMetadata(t *testing.T) {
	before := packagedAt
	t.Cleanup(func() { packagedAt = before })
	packagedAt = ""
	if PackagedAt() != "" {
		t.Fatal("source runs must not invent a package time")
	}
	packagedAt = "2026-11-09T01:02:03Z"
	if PackagedAt() != packagedAt || Version() != "0.0.1.dev" {
		t.Fatal("release metadata was not preserved")
	}
}
