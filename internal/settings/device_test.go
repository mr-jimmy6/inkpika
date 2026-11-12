package settings

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mr-jimmy6/inkpika/internal/transfer"
)

func TestDeviceSettingsIndependentOfCloudService(t *testing.T) {
	s, _ := settingsFixture(t)
	for _, missing := range []bool{false, true} {
		if missing {
			if err := os.Remove(s.filename); err != nil {
				t.Fatal(err)
			}
		}
		portal, err := New(transfer.Config{Port: 8791, UIPort: 8792}, s.filename, Hooks{})
		if err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse(portal.LaunchURL())
		token, _ := url.ParseQuery(u.Fragment)
		for _, tc := range []struct {
			method, path, body string
			code               int
		}{
			{"GET", "/api/config", "", 200},
			{"POST", "/api/config", `{}`, 404},
			{"GET", "/api/connection-check", "", map[bool]int{true: 400, false: 200}[missing]},
			{"POST", "/api/connection-check", `{"endpoint":"https://unexpected.invalid"}`, 400},
			{"POST", "/api/connection-check", `{} {}`, 400},
			{"DELETE", "/api/connection-check", "", 404},
			{"GET", "/api/objects", "", 404},
		} {
			r := httptest.NewRequest(tc.method, "http://localhost:8792"+tc.path, strings.NewReader(tc.body))
			r.Header.Set("X-Transfer-Token", token.Get("token"))
			w := httptest.NewRecorder()
			portal.LocalHandler().ServeHTTP(w, r)
			if w.Code != tc.code {
				t.Fatalf("missing=%v %s %s: %d %s", missing, tc.method, tc.path, w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "SECRET") || strings.Contains(w.Body.String(), `"source"`) {
				t.Fatal("private data exposed")
			}
			if tc.path == "/api/config" && tc.method == "GET" {
				var data map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
					t.Fatal(err)
				}
				if data["sync_prefix"] != "sync/" || data["provider_label"] == "" {
					t.Fatal("incomplete summary")
				}
			}
		}
	}
	files, err := os.ReadDir(filepath.Dir(s.filename))
	if err != nil || len(files) != 0 {
		t.Fatal("read or invalid request created state", files, err)
	}
}

func TestLANSettingsRejectConnectionCheck(t *testing.T) {
	s, _ := settingsFixture(t)
	for _, method := range []string{"GET", "POST"} {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest(method, "http://localhost/api/connection-check", strings.NewReader("{}")))
		if w.Code != 404 {
			t.Fatal("check exposed on paired LAN handler", w.Code)
		}
	}
}
