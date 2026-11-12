package config

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mr-jimmy6/inkpika/internal/bookformat"
	"github.com/mr-jimmy6/inkpika/internal/i18n"
)

type Config struct {
	Language        string   `json:"language,omitempty"`
	Provider        string   `json:"provider"`
	Region          string   `json:"region"`
	Bucket          string   `json:"bucket"`
	Zone            string   `json:"zone"`
	Endpoint        string   `json:"endpoint"`
	AccessKeyID     string   `json:"access_key_id"`
	SecretAccessKey string   `json:"secret_access_key"`
	SecurityToken   string   `json:"security_token,omitempty"`
	S3Preset        string   `json:"s3_preset,omitempty"`
	S3Addressing    string   `json:"s3_addressing,omitempty"`
	SOCKS5Proxy     string   `json:"socks5_proxy,omitempty"`
	CABundle        string   `json:"ca_bundle,omitempty"`
	Prefix          string   `json:"prefix"`
	Listen          string   `json:"listen"`
	DownloadDir     string   `json:"download_dir"`
	StateDir        string   `json:"state_dir"`
	MaxDownloadMB   int64    `json:"max_download_mb"`
	EbookFormats    []string `json:"ebook_formats,omitempty"`
}

var label = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`)

func Load(filename string) (Config, error) {
	c := Config{Language: "auto", Listen: "127.0.0.1:8787", MaxDownloadMB: 512}
	f, err := os.Open(filename)
	if err != nil {
		return c, fmt.Errorf("Cannot read configuration; copy config.example.json to config.json and fill it in: %w", err)
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 64*1024))
	d.DisallowUnknownFields()
	if err = d.Decode(&c); err != nil {
		return c, fmt.Errorf("Invalid configuration JSON: %w", err)
	}
	if err = d.Decode(new(interface{})); err != io.EOF {
		return c, errors.New("Configuration must contain exactly one JSON object")
	}
	defaultState := c.StateDir == ""
	if c.DownloadDir == "" {
		c.DownloadDir = "/mnt/us/documents/InkPika"
	}
	if defaultState {
		c.StateDir = filepath.Join(filepath.Dir(filename), "state")
	}
	if c.StateDir, err = filepath.Abs(c.StateDir); err != nil {
		return c, err
	}
	if err = c.Validate(); err != nil {
		return c, err
	}
	if defaultState {
		// A provider/Bucket switch must not pick up another source's partial files.
		h := sha256.Sum256([]byte(c.SourceIdentity()))
		c.StateDir = filepath.Join(c.StateDir, fmt.Sprintf("%s-%x", c.Provider, h[:6]))
	}
	return c, nil
}

func (c *Config) Validate() error {
	if err := i18n.Validate(c.Language); err != nil {
		return err
	}
	if len(c.EbookFormats) > 0 {
		formats, err := bookformat.Normalize(c.EbookFormats)
		if err != nil {
			return err
		}
		c.EbookFormats = formats
	}
	validBucket := label.MatchString(c.Bucket)
	if c.Provider == S3 {
		validBucket = s3Bucket.MatchString(c.Bucket) && !strings.Contains(c.Bucket, "..") && !strings.Contains(c.Bucket, ".-") && !strings.Contains(c.Bucket, "-.") && net.ParseIP(c.Bucket) == nil
	}
	if !validBucket || len(c.Bucket) < 3 || len(c.Bucket) > 63 {
		return errors.New("bucket must be the actual bucket name (3–63 lowercase letters, digits or hyphens)")
	}
	if c.AccessKeyID == "" || c.SecretAccessKey == "" || strings.Contains(c.AccessKeyID, "YOUR_") || strings.Contains(c.SecretAccessKey, "YOUR_") {
		return errors.New("Fill in access_key_id and secret_access_key in config.json")
	}
	if strings.ContainsAny(c.AccessKeyID+c.SecretAccessKey+c.SecurityToken, "\r\n") {
		return errors.New("Credentials cannot contain line breaks")
	}
	if err := c.validateProvider(); err != nil {
		return err
	}
	if _, err := ParseSOCKS5Proxy(c.SOCKS5Proxy); err != nil {
		return err
	}
	if c.CABundle != "" && (!filepath.IsAbs(c.CABundle) || strings.ContainsAny(c.CABundle, "\x00\r\n")) {
		return errors.New("ca_bundle must be an absolute path to a PEM CA certificate on Kindle")
	}

	u, err := url.Parse(c.Endpoint)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("endpoint must be a regional HTTPS address without a bucket, path or query")
	}
	if strings.HasPrefix(u.Hostname(), c.Bucket+".") && c.Provider != AzureBlob && !(c.Provider == S3 && c.S3Preset == "custom") {
		return errors.New("Use a regional endpoint without the bucket prefix")
	}
	c.Endpoint = strings.TrimRight(c.Endpoint, "/")
	if c.Provider == S3 && c.S3Addressing == "virtual" && (net.ParseIP(u.Hostname()) != nil || strings.Contains(c.Bucket, ".")) {
		return errors.New("Use path addressing for IP endpoints or dotted S3 bucket names to validate HTTPS certificates")
	}
	host, port, err := net.SplitHostPort(c.Listen)
	if err != nil || host != "127.0.0.1" || port == "" {
		return errors.New("listen must be 127.0.0.1:port; the service is device-local only")
	}
	if c.Prefix != "" && !strings.HasSuffix(c.Prefix, "/") {
		c.Prefix += "/"
	}
	if !filepath.IsAbs(c.DownloadDir) || !filepath.IsAbs(c.StateDir) {
		return errors.New("download_dir and state_dir must be absolute paths")
	}
	if c.MaxDownloadMB < 1 || c.MaxDownloadMB > 16384 {
		return errors.New("max_download_mb must be between 1 and 16384")
	}
	return nil
}

func (c Config) Supports(name string) bool { return bookformat.Allows(c.EbookFormats, name) }

// SourceIdentity excludes credentials and language, which can change without losing progress.
// QingStor identifies a source by endpoint and bucket; other providers include region.
func (c Config) SourceIdentity() string {
	base := c.Endpoint + "\x00" + c.Bucket
	if c.Provider == QingStor {
		return base
	}
	return c.Provider + "\x00" + c.Region + "\x00" + base
}

// Use a stable loopback hostname for the native WAF window. localhost resolves to
// the same loopback listener and needs no system/browser configuration changes.
func (c Config) BrowserAddress() string {
	return strings.Replace(c.Listen, "127.0.0.1:", "localhost:", 1)
}
