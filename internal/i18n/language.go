// Package i18n resolves device-owned language preferences and renders messages.
package i18n

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
)

const DeviceLocaleFile = "/var/local/system/locale"

func Validate(language string) error {
	if language != "" && language != "auto" && language != "en" && language != "zh" {
		return errors.New("language must be auto, en or zh")
	}
	return nil
}

// Locale recognizes Kindle LANG assignments as well as a bare locale. Host
// environment variables and HTTP/browser language preferences are never used.
func Locale(value string) string {
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		if strings.HasPrefix(line, "LANG=") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "LANG="))
		}
		line = strings.ToLower(strings.Trim(line, "\"'"))
		base := strings.FieldsFunc(line, func(r rune) bool { return r == '_' || r == '-' || r == '.' || r == '@' })
		if len(base) > 0 && (base[0] == "zh" || base[0] == "en") {
			return base[0]
		}
	}
	return "en"
}

func readBounded(filename string, limit int64) ([]byte, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if len(b) > int(limit) {
		return nil, errors.New("language configuration exceeds size limit")
	}
	return b, err
}

func Resolve(preference string) string { return ResolveFile(preference, DeviceLocaleFile) }

// ResolveFile makes the device locale source testable without changing the host.
func ResolveFile(preference, filename string) string {
	if preference == "en" || preference == "zh" {
		return preference
	}
	if Validate(preference) != nil {
		return "en"
	}
	b, err := readBounded(filename, 4096)
	if err != nil {
		return "en"
	}
	return Locale(string(b))
}

// Preference reads only the shared language setting. Wireless transfer and
// settings must work before cloud credentials have been configured.
func Preference(filename string) string {
	b, err := readBounded(filename, 64*1024)
	if os.IsNotExist(err) || filename == "" {
		return "auto"
	}
	var c struct {
		Language string `json:"language"`
	}
	if err != nil || json.Unmarshal(b, &c) != nil || Validate(c.Language) != nil {
		return "en"
	}
	if c.Language == "" {
		return "auto"
	}
	return c.Language
}

func FromConfig(filename string) string { return Resolve(Preference(filename)) }
