// Package transfer provides an independent LAN ebook transfer service. It never loads
// cloud credentials or exposes the cloud client's APIs.
package transfer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Config struct {
	Port        int    `json:"port"`
	UIPort      int    `json:"ui_port"`
	DownloadDir string `json:"download_dir"`
	LibraryDir  string `json:"library_dir"`
	MaxUploadMB int64  `json:"max_upload_mb"`
	StateDir    string `json:"-"`
	// FormatsFile is the shared cloud config path for format and language preferences.
	FormatsFile string `json:"-"`
}

func Load(filename string) (Config, error) {
	c := Config{Port: 8789, UIPort: 8790, DownloadDir: "/mnt/us/documents/InkPika/Wireless", LibraryDir: "/mnt/us/documents", MaxUploadMB: 512}
	full, err := filepath.Abs(filename)
	if err != nil {
		return c, err
	}
	c.StateDir = filepath.Join(filepath.Dir(full), "transfer-state")
	f, err := os.Open(filename)
	if err != nil && !os.IsNotExist(err) {
		return c, err
	}
	if err == nil {
		defer f.Close()
		d := json.NewDecoder(io.LimitReader(f, 16*1024))
		d.DisallowUnknownFields()
		if err = d.Decode(&c); err != nil {
			return c, fmt.Errorf("Invalid wireless transfer configuration: %w", err)
		}
		if err = d.Decode(new(interface{})); err != io.EOF {
			return c, errors.New("Wireless transfer configuration must contain exactly one JSON object")
		}
	}
	if c.Port < 1024 || c.Port > 65535 || c.UIPort < 1024 || c.UIPort > 65535 || c.Port == c.UIPort {
		return c, errors.New("Wireless transfer ports must be distinct and between 1024 and 65535")
	}
	if !filepath.IsAbs(c.DownloadDir) || !filepath.IsAbs(c.LibraryDir) || c.MaxUploadMB < 1 || c.MaxUploadMB > 16384 {
		return c, errors.New("Wireless transfer download folder must be absolute; file limit is 1–16384 MB")
	}
	return c, nil
}
