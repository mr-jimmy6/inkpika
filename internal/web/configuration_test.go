package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mr-jimmy6/inkpika/internal/buildinfo"
	"github.com/mr-jimmy6/inkpika/internal/config"
)

func TestConfigurationOnlyExposesMaskedCredentials(t *testing.T) {
	for _, tc := range []struct{ name, value, want string }{
		{"empty", "", "Not configured"}, {"one", "a", "****"}, {"two", "ab", "****"},
		{"three", "abc", "****"}, {"four", "abcd", "****"}, {"five", "abcde", "ab****de"},
		{"long", "AB-private-middle-YZ", "AB****YZ"}, {"unicode", "甲乙丙丁戊己", "甲乙****戊己"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := config.Config{Provider: config.AliyunOSS, Bucket: "books", Listen: "127.0.0.1:8787", DownloadDir: "/mnt/us/documents/InkPika", AccessKeyID: tc.value, SecretAccessKey: tc.value, SecurityToken: "token-must-stay-private", SOCKS5Proxy: "socks5://private-user:proxy-private@proxy.invalid:1080"}
			h := New(c, nil, nil, "session")
			r := httptest.NewRequest("GET", "http://"+c.Listen+"/api/config", nil)
			r.Header.Set("X-InkPika-Token", "session")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			var out map[string]string
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
				t.Fatal(w.Code, w.Body.String())
			}
			if len(out) != 13 || out["version"] != buildinfo.Version() || out["packaged_at"] != buildinfo.PackagedAt() || out["connection"] != "SOCKS5 (configured)" || out["access_key_id_masked"] != tc.want || out["secret_access_key_masked"] != tc.want || out["download_dir"] != c.DownloadDir || out["sync_prefix"] != "sync/" {
				t.Fatal("unexpected public configuration", out)
			}
			if strings.Contains(w.Body.String(), c.SecurityToken) || strings.Contains(w.Body.String(), "private-middle") || strings.Contains(w.Body.String(), "proxy-private") || strings.Contains(w.Body.String(), "private-user") {
				t.Fatal("credential exposed")
			}
		})
	}
}

func TestConnectionCheckAPIIsExplicitAndPersistsSanitizedResult(t *testing.T) {
	for _, fail := range []bool{false, true} {
		c := config.Config{Provider: config.AliyunOSS, Bucket: "books", Listen: "127.0.0.1:8787", StateDir: t.TempDir(), AccessKeyID: "private-id", SecretAccessKey: "private-secret"}
		calls := 0
		s := &Server{cfg: c, token: "session", store: fakeStore{}, checkProbe: func(context.Context, config.Config) error {
			calls++
			if fail {
				return errors.New("TLS failed private-secret")
			}
			return nil
		}}
		request := func(method, body string, status int) map[string]interface{} {
			t.Helper()
			r := httptest.NewRequest(method, "http://"+c.Listen+"/api/connection-check", strings.NewReader(body))
			r.Header.Set("X-InkPika-Token", "session")
			w := httptest.NewRecorder()
			s.serve(w, r)
			if w.Code != status {
				t.Fatal(w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "private-secret") || strings.Contains(w.Body.String(), "private-id") {
				t.Fatal("credential exposed")
			}
			var out map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			return out
		}
		if r := request("GET", "", 200); r["updated"] != float64(0) || calls != 0 {
			t.Fatal("reading configuration initiated a check", r)
		}
		request("POST", `{"endpoint":"https://unexpected.test"}`, 400)
		request("POST", `{} {}`, 400)
		if calls != 0 {
			t.Fatal("invalid input started a check")
		}
		r := request("POST", `{}`, 200)
		if calls != 1 || r["running"] != false || r["updated_local"] == "" || r["source"] != nil {
			t.Fatal(r)
		}
		if fail && (r["network"] != "Failed" || !strings.Contains(r["detail"].(string), "TLS failed [redacted]")) {
			t.Fatal(r)
		}
		if !fail && r["read"] != "Passed (sample)" {
			t.Fatal(r)
		}
		if saved := request("GET", "", 200); saved["detail"] != r["detail"] || calls != 1 {
			t.Fatal("result was not retained", saved)
		}
		for _, name := range []string{"jobs.json", "sync.json", "launch.url"} {
			if _, err := os.Stat(filepath.Join(c.StateDir, name)); !os.IsNotExist(err) {
				t.Fatal("check created unrelated state", name)
			}
		}
	}
}

func TestConfigurationAndCheckRequireLocalSession(t *testing.T) {
	c := config.Config{Listen: "127.0.0.1:8787"}
	h := New(c, nil, nil, "session")
	for _, path := range []string{"/api/config", "/api/connection-check"} {
		for _, tc := range []struct {
			method, host, token, origin string
			code                        int
		}{
			{"GET", c.Listen, "", "", 401}, {"POST", c.Listen, "", "", 401},
			{"GET", "foreign.test", "session", "", 403},
			{"POST", c.Listen, "session", "https://foreign.test", 403},
		} {
			r := httptest.NewRequest(tc.method, "http://"+tc.host+path, strings.NewReader("{}"))
			r.Header.Set("X-InkPika-Token", tc.token)
			r.Header.Set("Origin", tc.origin)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.code {
				t.Fatal(path, w.Code, w.Body.String())
			}
		}
	}
}
