package settings

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mr-jimmy6/inkpika/internal/config"
)

func TestAdditionalPlatformsSaveAndPreserveCredentials(t *testing.T) {
	for _, tc := range []struct{ provider, preset, region, bucket string }{
		{config.S3, "tos", "cn-beijing", "books"}, {config.S3, "ibm", "us-south", "books"},
		{config.S3, "gcs", "auto", "books"}, {config.S3, "cos", "ap-guangzhou", "books-1250000000"},
		{config.AzureBlob, "", "", "books"},
	} {
		t.Run(tc.provider+tc.preset, func(t *testing.T) {
			s, old := settingsFixture(t)
			e := editing(t, s)
			e.Provider = tc.provider
			e.S3Preset = tc.preset
			e.Region = tc.region
			e.Bucket = tc.bucket
			e.Endpoint = ""
			if w := save(s, e); w.Code != 400 {
				t.Fatal("platform change reused old credentials")
			}
			e.AccessKeyID = "bookaccount"
			e.SecretAccessKey = base64.StdEncoding.EncodeToString(make([]byte, 64))
			if w := save(s, e); w.Code != 200 {
				t.Fatal(w.Body.String())
			}
			c, err := config.Load(s.filename)
			if err != nil {
				t.Fatal(err)
			}
			if c.SecurityToken != "" || c.SourceIdentity() == old.SourceIdentity() || c.SecretAccessKey != e.SecretAccessKey {
				t.Fatal("source/credential transition failed")
			}
			view := editing(t, s)
			if view.AccessKeyID != "" || view.SecretAccessKey != "" {
				t.Fatal("credentials exposed")
			}
			if w := save(s, view); w.Code != 200 {
				t.Fatal(w.Body.String())
			}
			again, _ := config.Load(s.filename)
			if again.SourceIdentity() != c.SourceIdentity() || again.SecretAccessKey != c.SecretAccessKey {
				t.Fatal("no-op lost identity/key")
			}
			if tc.provider == config.AzureBlob {
				view = editing(t, s)
				view.AccessKeyID = "otheraccount"
				if w := save(s, view); w.Code != 200 {
					t.Fatal(w.Body.String())
				}
				again, _ = config.Load(s.filename)
				if again.Endpoint != "https://otheraccount.blob.core.windows.net" || again.SourceIdentity() == c.SourceIdentity() {
					t.Fatal("Azure account endpoint not refreshed")
				}
			}
		})
	}
}

func settingsFixture(t *testing.T) (*Server, config.Config) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.json")
	raw := config.Config{Provider: config.AliyunOSS, Region: "cn-beijing", Bucket: "books", AccessKeyID: "AK-ID-SECRET", SecretAccessKey: "KEY-SECRET", SecurityToken: "TOKEN-SECRET", Listen: "127.0.0.1:8787", DownloadDir: "/mnt/us/documents/InkPika", MaxDownloadMB: 512}
	b, _ := json.Marshal(raw)
	if err := os.WriteFile(p, b, 0600); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	return &Server{filename: p}, c
}
func call(s *Server, method, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest(method, "http://localhost/api/config", strings.NewReader(body)))
	return w
}
func editing(t *testing.T, s *Server) edit {
	t.Helper()
	w := call(s, "GET", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "SECRET") {
		t.Fatal("credential returned")
	}
	var data edit
	json.Unmarshal(w.Body.Bytes(), &data)
	return data
}
func save(s *Server, e edit) *httptest.ResponseRecorder {
	b, _ := json.Marshal(e)
	return call(s, "POST", string(b))
}

func TestPrefillNoOpSecretsAndAtomicFormatSave(t *testing.T) {
	s, old := settingsFixture(t)
	before, _ := os.ReadFile(s.filename)
	e := editing(t, s)
	e.EbookFormats = []string{} // The browser submits an empty list for defaults.
	if e.Bucket != "books" || e.Region != "cn-beijing" || e.SecretAccessKey != "" {
		t.Fatal("prefill incomplete or secret exposed")
	}
	if w := save(s, e); w.Code != 200 || !strings.Contains(w.Body.String(), `"changed":false`) {
		t.Fatal(w.Code, w.Body.String())
	}
	after, _ := os.ReadFile(s.filename)
	if string(before) != string(after) {
		t.Fatal("no-op changed file")
	}
	e.EbookFormats = []string{".PDF", "epub"}
	e.Prefix = "books"
	called := false
	restarted := false
	s.hooks.BeforeSave = func() (func() string, error) {
		called = true
		b, _ := os.ReadFile(s.filename)
		if string(b) != string(before) {
			t.Fatal("old config changed before stop")
		}
		return func() string { restarted = true; return "applied" }, nil
	}
	if w := save(s, e); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	next, err := config.Load(s.filename)
	if err != nil {
		t.Fatal(err)
	}
	if !called || !restarted || next.AccessKeyID != old.AccessKeyID || next.SecretAccessKey != old.SecretAccessKey || next.SecurityToken != old.SecurityToken || next.StateDir != old.StateDir || next.SourceIdentity() != old.SourceIdentity() || !reflect.DeepEqual(next.EbookFormats, []string{"pdf", "epub"}) {
		t.Fatal("configuration lost identity/credentials/rules")
	}
	backup, _ := os.ReadFile(s.filename + ".previous")
	if string(backup) != string(before) {
		t.Fatal("old config not backed up")
	}
	if w := save(s, e); w.Code != 409 {
		t.Fatal("stale revision overwrote newer config")
	}
}

func TestValidationAndStopFailurePreserveOriginal(t *testing.T) {
	s, _ := settingsFixture(t)
	before, _ := os.ReadFile(s.filename)
	for _, change := range []func(*edit){func(e *edit) { e.Bucket = "INVALID" }, func(e *edit) { e.Endpoint = "http://evil" }, func(e *edit) { e.DownloadDir = "/etc" }, func(e *edit) { e.EbookFormats = []string{"../txt"} }, func(e *edit) { e.Provider = config.QingStor; e.Region = "pek3b" }} {
		e := editing(t, s)
		change(&e)
		if w := save(s, e); w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
		after, _ := os.ReadFile(s.filename)
		if string(before) != string(after) {
			t.Fatal("invalid config persisted")
		}
	}
	e := editing(t, s)
	e.Bucket = "different"
	s.hooks.BeforeSave = func() (func() string, error) { return nil, errors.New("stopping failed") }
	if w := save(s, e); w.Code != 409 {
		t.Fatal(w.Code)
	}
	after, _ := os.ReadFile(s.filename)
	if string(before) != string(after) {
		t.Fatal("config changed despite stop failure")
	}
}

func TestSourceChangeGetsSeparateStateAndTokenClearing(t *testing.T) {
	s, old := settingsFixture(t)
	raw, _ := os.ReadFile(s.filename)
	var c config.Config
	json.Unmarshal(raw, &c)
	c.StateDir = old.StateDir
	b, _ := json.Marshal(c)
	os.WriteFile(s.filename, b, 0600)
	e := editing(t, s)
	e.Bucket = "new-books"
	e.ClearSecurityToken = true
	if w := save(s, e); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	next, err := config.Load(s.filename)
	if err != nil {
		t.Fatal(err)
	}
	if next.StateDir == old.StateDir || next.SecurityToken != "" || next.DownloadDir != old.DownloadDir {
		t.Fatal("source change reused state or moved books")
	}
}

func TestFirstConfigurationDoesNotRequireCloudAccount(t *testing.T) {
	s := &Server{filename: filepath.Join(t.TempDir(), "config.json")}
	e := editing(t, s)
	e.Provider = config.AliyunOSS
	e.Region = "cn-beijing"
	e.Bucket = "first-books"
	e.AccessKeyID = "new-id"
	e.SecretAccessKey = "new-key"
	if w := save(s, e); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, err := config.Load(s.filename); err != nil {
		t.Fatal(err)
	}
}

func TestS3ProxyPreservationClearingAndRedactedPrefill(t *testing.T) {
	s, _ := settingsFixture(t)
	e := editing(t, s)
	e.Provider = config.S3
	e.S3Preset = "r2"
	e.Region = "auto"
	e.Endpoint = "https://example.r2.cloudflarestorage.com"
	e.AccessKeyID = "new-id"
	e.SecretAccessKey = "new-secret"
	e.SOCKS5Proxy = "socks5h://reader:PROXY-SECRET@127.0.0.1:1080"
	if w := save(s, e); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	old, err := config.Load(s.filename)
	if err != nil {
		t.Fatal(err)
	}
	if old.SecurityToken != "" || old.S3Addressing != "path" || old.SOCKS5Proxy != e.SOCKS5Proxy {
		t.Fatal("bad S3 transition")
	}
	view := call(s, "GET", "")
	if strings.Contains(view.Body.String(), "reader") || strings.Contains(view.Body.String(), "PROXY-SECRET") || strings.Contains(view.Body.String(), "127.0.0.1:1080") || !strings.Contains(view.Body.String(), `"socks5_proxy_set":true`) {
		t.Fatal("proxy leaked or missing set flag")
	}
	e = editing(t, s)
	if e.SOCKS5Proxy != "" {
		t.Fatal("raw proxy returned")
	}
	e.Prefix = "books/"
	if w := save(s, e); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	kept, err := config.Load(s.filename)
	if err != nil || kept.SOCKS5Proxy != old.SOCKS5Proxy || kept.StateDir != old.StateDir {
		t.Fatal("blank proxy lost credentials or state", err)
	}
	e = editing(t, s)
	e.SOCKS5Proxy = "http://reader:PROXY-SECRET@proxy:1080"
	if w := save(s, e); w.Code != 400 || strings.Contains(w.Body.String(), "PROXY-SECRET") {
		t.Fatal("invalid proxy accepted or leaked")
	}
	e = editing(t, s)
	e.ClearSOCKS5Proxy = true
	if w := save(s, e); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	cleared, err := config.Load(s.filename)
	if err != nil || cleared.SOCKS5Proxy != "" || cleared.SourceIdentity() != old.SourceIdentity() || cleared.StateDir != old.StateDir {
		t.Fatal("proxy clearing changed source", err)
	}
	e = editing(t, s)
	e.S3Preset = "aws"
	e.Region = "us-east-1"
	e.Endpoint = ""
	if w := save(s, e); w.Code != 400 {
		t.Fatal("preset change reused old cloud credentials")
	}
}

func TestCustomS3RegionEditKeepsEndpointAndRejectsInvalidCA(t *testing.T) {
	s := &Server{filename: filepath.Join(t.TempDir(), "config.json")}
	e := editing(t, s)
	e.Provider = config.S3
	e.S3Preset = "custom"
	e.Endpoint = "https://nas.example:9443"
	e.Region = "garage"
	e.Bucket = "books"
	e.AccessKeyID = "id"
	e.SecretAccessKey = "secret"
	if w := save(s, e); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	e = editing(t, s)
	e.Region = "another-region"
	if w := save(s, e); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	c, err := config.Load(s.filename)
	if err != nil || c.Endpoint != "https://nas.example:9443" {
		t.Fatal("custom endpoint cleared", err)
	}
	before, _ := os.ReadFile(s.filename)
	e = editing(t, s)
	e.CABundle = filepath.Join(t.TempDir(), "missing.pem")
	if w := save(s, e); w.Code != 400 {
		t.Fatal("invalid CA saved")
	}
	after, _ := os.ReadFile(s.filename)
	if string(before) != string(after) {
		t.Fatal("bad CA replaced working config")
	}
}

func TestLanguageOnlySavePreservesSourceCredentialsAndPairing(t *testing.T) {
	s, before := settingsFixture(t)
	e := editing(t, s)
	language := "zh"
	w := save(s, edit{Revision: e.Revision, Language: &language, LanguageOnly: true})
	if w.Code != 200 || w.Header().Get("Content-Language") != "zh" {
		t.Fatal(w.Code, w.Body.String())
	}
	after, err := config.Load(s.filename)
	if err != nil {
		t.Fatal(err)
	}
	if after.Language != "zh" || after.SourceIdentity() != before.SourceIdentity() || after.StateDir != before.StateDir || after.AccessKeyID != before.AccessKeyID || after.SecretAccessKey != before.SecretAccessKey {
		t.Fatal("language save changed cloud identity or credentials")
	}
	// A second save through the same server needs only the latest revision.
	e = editing(t, s)
	language = "en"
	if w = save(s, edit{Revision: e.Revision, Language: &language, LanguageOnly: true}); w.Code != 200 || w.Header().Get("Content-Language") != "en" {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestLanguageCanBeSavedBeforeCloudSetup(t *testing.T) {
	s := &Server{filename: filepath.Join(t.TempDir(), "config.json")}
	e := editing(t, s)
	language := "zh"
	if w := save(s, edit{Revision: e.Revision, Language: &language, LanguageOnly: true}); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	b, err := os.ReadFile(s.filename)
	if err != nil {
		t.Fatal(err)
	}
	var c config.Config
	if err = json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	if c.Language != "zh" || c.AccessKeyID != "" || c.SecretAccessKey != "" {
		t.Fatal("language-only setup changed credentials")
	}
	language = "fr"
	e = editing(t, s)
	if w := save(s, edit{Revision: e.Revision, Language: &language, LanguageOnly: true}); w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	after, _ := os.ReadFile(s.filename)
	if string(after) != string(b) {
		t.Fatal("invalid language changed configuration")
	}
}
