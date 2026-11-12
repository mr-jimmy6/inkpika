// Package bookformat defines the shared, user-configurable ebook allowlist.
package bookformat

import (
	"encoding/json"
	"errors"
	"os"
	"path"
	"regexp"
	"strings"
)

var extension = regexp.MustCompile(`^[a-z0-9]{1,12}$`)

func Defaults() []string { return []string{"azw", "azw3", "mobi", "prc", "pdf", "txt"} }

func Normalize(values []string) ([]string, error) {
	if len(values) == 0 {
		return Defaults(), nil
	}
	if len(values) > 32 {
		return nil, errors.New("Specify at most 32 ebook formats")
	}
	out := []string{}
	seen := map[string]bool{}
	for _, v := range values {
		v = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(v)), ".")
		if !extension.MatchString(v) {
			return nil, errors.New("Ebook extensions must contain 1–12 ASCII letters or digits, such as azw3 or pdf")
		}
		if !seen[v] {
			out = append(out, v)
			seen[v] = true
		}
	}
	return out, nil
}

func Allows(formats []string, name string) bool {
	if len(formats) == 0 {
		formats = Defaults()
	}
	ext := strings.TrimPrefix(strings.ToLower(path.Ext(name)), ".")
	for _, allowed := range formats {
		if allowed == ext {
			return true
		}
	}
	return false
}

// Read only the allowlist from the shared config, independently of cloud
// account readiness. Invalid explicit rules fail closed instead of widening it.
func FromFile(filename string) ([]string, error) {
	if filename == "" {
		return Defaults(), nil
	}
	b, err := os.ReadFile(filename)
	if os.IsNotExist(err) {
		return Defaults(), nil
	}
	if err != nil || len(b) > 64*1024 {
		return nil, errors.New("Cannot read ebook format configuration")
	}
	var c struct {
		Formats []string `json:"ebook_formats"`
	}
	if json.Unmarshal(b, &c) != nil {
		return nil, errors.New("Invalid ebook format configuration; correct it in settings")
	}
	return Normalize(c.Formats)
}
