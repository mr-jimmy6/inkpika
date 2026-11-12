// Package settings exposes a separately paired LAN configuration editor.
// It never returns complete stored credentials to the browser.
package settings

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"

	"github.com/mr-jimmy6/inkpika/internal/bookformat"
	"github.com/mr-jimmy6/inkpika/internal/buildinfo"
	"github.com/mr-jimmy6/inkpika/internal/cloudhttp"
	"github.com/mr-jimmy6/inkpika/internal/config"
	"github.com/mr-jimmy6/inkpika/internal/i18n"
	"github.com/mr-jimmy6/inkpika/internal/statefile"
	"github.com/mr-jimmy6/inkpika/internal/syncer"
	"github.com/mr-jimmy6/inkpika/internal/transfer"
)

//go:embed assets/*
var assets embed.FS

type Hooks struct {
	// Called with the old config still on disk. Stop only an already-running
	// cloud service; the returned callback restores its prior running state.
	BeforeSave func() (after func() string, err error)
}

type Server struct {
	filename string
	hooks    Hooks
	mu       sync.Mutex
}

func New(c transfer.Config, filename string, hooks Hooks) (*transfer.Server, error) {
	c.FormatsFile = filename
	s := &Server{filename: filename, hooks: hooks}
	return transfer.NewPortal(c, transfer.PortalOptions{Title: "Settings", Description: "Edit InkPika storage credentials, download folder and ebook formats from a phone or computer.", StopLabel: "Stop settings", Assets: assets, API: s, DeviceAssets: assets, DeviceAPI: http.HandlerFunc(s.device)})
}

func reply(w http.ResponseWriter, status int, value interface{}) {
	i18n.JSON(w, status, value)
}
func failure(w http.ResponseWriter, status int, message string) {
	reply(w, status, map[string]string{"error": message})
}
func revision(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func masked(s string) string {
	r := []rune(s)
	if len(r) < 5 {
		if len(r) == 0 {
			return "Not set"
		}
		return "****"
	}
	return string(r[:2]) + "****" + string(r[len(r)-2:])
}

func (s *Server) read() ([]byte, config.Config, error) {
	b, err := os.ReadFile(s.filename)
	if os.IsNotExist(err) {
		return nil, config.Config{Provider: config.AliyunOSS, Listen: "127.0.0.1:8787", DownloadDir: "/mnt/us/documents/InkPika", MaxDownloadMB: 512}, nil
	}
	if err != nil || len(b) > 64*1024 {
		return nil, config.Config{}, errors.New("Cannot read the current configuration")
	}
	var raw config.Config
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&raw) != nil || d.Decode(new(interface{})) != io.EOF {
		return nil, raw, errors.New("Invalid configuration JSON; correct the configuration file first")
	}
	return b, raw, nil
}

type edit struct {
	LanguageOnly       bool     `json:"language_only"`
	Language           *string  `json:"language"`
	Revision           string   `json:"revision"`
	Provider           string   `json:"provider"`
	Region             string   `json:"region"`
	Bucket             string   `json:"bucket"`
	Endpoint           string   `json:"endpoint"`
	S3Preset           string   `json:"s3_preset"`
	S3Addressing       string   `json:"s3_addressing"`
	CABundle           string   `json:"ca_bundle"`
	SOCKS5Proxy        string   `json:"socks5_proxy"`
	ClearSOCKS5Proxy   bool     `json:"clear_socks5_proxy"`
	Prefix             string   `json:"prefix"`
	DownloadDir        string   `json:"download_dir"`
	MaxDownloadMB      int64    `json:"max_download_mb"`
	EbookFormats       []string `json:"ebook_formats"`
	AccessKeyID        string   `json:"access_key_id"`
	SecretAccessKey    string   `json:"secret_access_key"`
	SecurityToken      string   `json:"security_token"`
	ClearSecurityToken bool     `json:"clear_security_token"`
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Language", i18n.FromConfig(s.filename))
	if r.URL.Path != "/api/config" || (r.Method != "GET" && r.Method != "POST") {
		failure(w, 404, "Unknown endpoint or unsupported request method")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Method == "GET" {
		b, raw, err := s.read()
		if err != nil {
			failure(w, 500, err.Error())
			return
		}
		c, err := config.Load(s.filename)
		if err != nil {
			c = raw
		}
		if c.Provider == "" {
			c.Provider = config.AliyunOSS
		}
		if c.MaxDownloadMB == 0 {
			c.MaxDownloadMB = 512
		}
		if c.DownloadDir == "" {
			c.DownloadDir = "/mnt/us/documents/InkPika"
		}
		reply(w, 200, map[string]interface{}{"version": buildinfo.Version(), "packaged_at": buildinfo.PackagedAt(), "revision": revision(b), "language": i18n.Preference(s.filename), "effective_language": i18n.FromConfig(s.filename), "provider": c.Provider, "provider_label": c.ProviderLabel(), "sync_prefix": syncer.Prefix, "region": c.Region, "bucket": c.Bucket, "endpoint": c.Endpoint, "s3_preset": c.S3Preset, "s3_addressing": c.S3Addressing, "ca_bundle": c.CABundle, "socks5_proxy_set": raw.SOCKS5Proxy != "", "prefix": c.Prefix, "download_dir": c.DownloadDir, "max_download_mb": c.MaxDownloadMB, "ebook_formats": raw.EbookFormats, "default_formats": bookformat.Defaults(), "access_key_id_masked": masked(raw.AccessKeyID), "secret_access_key_masked": masked(raw.SecretAccessKey), "security_token_set": raw.SecurityToken != ""})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	var in edit
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil || d.Decode(new(interface{})) != io.EOF {
		failure(w, 400, "Invalid configuration content")
		return
	}
	lock, err := os.OpenFile(s.filename+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		failure(w, 500, "Cannot lock the configuration file")
		return
	}
	defer lock.Close()
	if syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		failure(w, 409, "Configuration is being saved; try again shortly")
		return
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	b, raw, err := s.read()
	if err != nil {
		failure(w, 500, err.Error())
		return
	}
	if revision(b) != in.Revision {
		failure(w, 409, "Configuration changed in another page; reload before saving")
		return
	}
	if in.LanguageOnly {
		if in.Language == nil {
			failure(w, 400, "language must be auto, en or zh")
			return
		}
		if err := i18n.Validate(*in.Language); err != nil {
			failure(w, 400, err.Error())
			return
		}
		raw.Language = *in.Language
		s.save(w, b, raw)
		return
	}
	old, oldErr := config.Load(s.filename)
	platformChanged := in.Provider != old.Provider || (in.Provider == config.S3 && in.S3Preset != old.S3Preset)
	if len(b) > 0 && platformChanged && (in.AccessKeyID == "" || in.SecretAccessKey == "") {
		failure(w, 400, "Enter new credentials when switching storage platforms")
		return
	}
	if in.Language != nil {
		raw.Language = *in.Language
	}
	raw.Provider = in.Provider
	raw.Region = strings.TrimSpace(in.Region)
	raw.Zone = ""
	raw.Bucket = strings.TrimSpace(in.Bucket)
	raw.Endpoint = strings.TrimSpace(in.Endpoint)
	raw.S3Preset = in.S3Preset
	raw.S3Addressing = in.S3Addressing
	raw.CABundle = strings.TrimSpace(in.CABundle)
	if in.ClearSOCKS5Proxy {
		raw.SOCKS5Proxy = ""
	} else if in.SOCKS5Proxy != "" {
		raw.SOCKS5Proxy = in.SOCKS5Proxy
	}
	raw.Prefix = strings.TrimSpace(in.Prefix)
	if oldErr == nil && raw.Endpoint == old.Endpoint {
		if platformChanged {
			raw.Endpoint = ""
		} else if old.Region != raw.Region || (raw.Provider == config.AzureBlob && in.AccessKeyID != "" && in.AccessKeyID != old.AccessKeyID) {
			derived := old
			derived.Endpoint = ""
			if derived.Validate() == nil && derived.Endpoint == old.Endpoint {
				raw.Endpoint = ""
			}
		}
	}
	raw.DownloadDir = filepath.Clean(strings.TrimSpace(in.DownloadDir))
	raw.MaxDownloadMB = in.MaxDownloadMB
	raw.EbookFormats = in.EbookFormats
	if len(raw.EbookFormats) == 0 {
		raw.EbookFormats = nil
	}
	if raw.Listen == "" {
		raw.Listen = "127.0.0.1:8787"
	}
	if raw.DownloadDir != "/mnt/us/documents" && !strings.HasPrefix(raw.DownloadDir, "/mnt/us/documents/") {
		failure(w, 400, "The download folder must be under /mnt/us/documents so Kindle can discover books")
		return
	}
	if in.AccessKeyID != "" {
		raw.AccessKeyID = in.AccessKeyID
	}
	if in.SecretAccessKey != "" {
		raw.SecretAccessKey = in.SecretAccessKey
	}
	if in.ClearSecurityToken || (platformChanged && in.SecurityToken == "") {
		raw.SecurityToken = ""
	} else if in.SecurityToken != "" {
		raw.SecurityToken = in.SecurityToken
	}
	validate := func() (config.Config, error) {
		data, _ := json.Marshal(raw)
		f, e := os.CreateTemp(filepath.Dir(s.filename), ".config-validate-*")
		if e != nil {
			return config.Config{}, e
		}
		name := f.Name()
		defer os.Remove(name)
		_, e = f.Write(data)
		closeErr := f.Close()
		if e != nil {
			return config.Config{}, e
		}
		if closeErr != nil {
			return config.Config{}, closeErr
		}
		checked, err := config.Load(name)
		if err != nil {
			return checked, err
		}
		// Check local CA/proxy settings before saving; no network request is made.
		client, err := cloudhttp.New(checked)
		if err != nil {
			return checked, err
		}
		client.CloseIdleConnections()
		return checked, nil
	}
	next, err := validate()
	if err != nil {
		failure(w, 400, safeError(err))
		return
	}
	if oldErr == nil {
		if next.SourceIdentity() != old.SourceIdentity() {
			raw.StateDir = ""
		}
		next, err = validate()
		if err != nil {
			failure(w, 400, safeError(err))
			return
		}
		if reflect.DeepEqual(old, next) {
			reply(w, 200, map[string]interface{}{"saved": true, "changed": false, "message": "No configuration changes; current settings preserved."})
			return
		}
	}
	if len(raw.EbookFormats) > 0 {
		raw.EbookFormats, _ = bookformat.Normalize(raw.EbookFormats)
	}
	s.save(w, b, raw)
}

func (s *Server) save(w http.ResponseWriter, b []byte, raw config.Config) {
	var err error
	data, _ := json.MarshalIndent(raw, "", "  ")
	data = append(data, '\n')
	after := func() string { return "" }
	if s.hooks.BeforeSave != nil {
		after, err = s.hooks.BeforeSave()
		if err != nil {
			failure(w, 409, "Cannot safely stop the cloud library; configuration unchanged. Try again shortly")
			return
		}
	}
	if len(b) > 0 {
		err = statefile.Write(s.filename+".previous", b)
	}
	if err == nil {
		err = statefile.Write(s.filename, data)
	}
	warning := after()
	if err != nil {
		failure(w, 500, "Save failed; original configuration preserved. Check free storage")
		return
	}
	language := i18n.FromConfig(s.filename)
	message := i18n.Text(language, "Configuration saved. The next sync or cloud library session will use these settings.")
	if warning != "" {
		message += " " + i18n.Text(language, warning)
	}
	w.Header().Set("Content-Language", i18n.FromConfig(s.filename))
	reply(w, 200, map[string]interface{}{"saved": true, "changed": true, "message": message, "effective_language": i18n.FromConfig(s.filename)})
}

func safeError(err error) string {
	// Validation errors are deliberately generic where OS/JSON errors could
	// contain filenames or input. Config validation itself never embeds keys.
	message := err.Error()
	if strings.Contains(strings.ToLower(message), "configuration json") || strings.Contains(strings.ToLower(message), "read configuration") || strings.Contains(message, ".config-validate-") {
		return "Configuration validation failed; check the fields"
	}
	return message
}
