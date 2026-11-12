package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mr-jimmy6/inkpika/internal/config"
	"github.com/mr-jimmy6/inkpika/internal/diagnostics"
)

func TestOfflineMenusAndCheckNeverStartService(t *testing.T) {
	dir := t.TempDir()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) }))
	defer server.Close()
	c := config.Config{Provider: config.QingStor, Region: "pek3b", Bucket: "books", Endpoint: server.URL, AccessKeyID: "canary-access", SecretAccessKey: "canary-secret", Listen: "127.0.0.1:8787", DownloadDir: filepath.Join(dir, "books"), StateDir: filepath.Join(dir, "state"), MaxDownloadMB: 1}
	b, _ := json.Marshal(c)
	file := filepath.Join(dir, "config.json")
	os.WriteFile(file, b, 0600)
	menu := filepath.Join(dir, "menu.json")
	for _, action := range []string{"refresh", "check"} {
		if err := menuControl(file, menu, action); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(menu)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{c.AccessKeyID, c.SecretAccessKey} {
			if strings.Contains(string(b), secret) {
				t.Fatal("credentials exposed")
			}
		}
		if strings.Contains(string(b), "查看配置") || strings.Contains(string(b), "连接检查") {
			t.Fatal("configuration and diagnostics must only appear in web")
		}
		for _, name := range []string{"server.pid", "launch.url", "server.lock", "sync.json", "jobs.json"} {
			if _, err := os.Stat(filepath.Join(c.StateDir, name)); !os.IsNotExist(err) {
				t.Fatal("offline action initialized service state", name)
			}
		}
	}
	b, _ = os.ReadFile(filepath.Join(c.StateDir, "connection-check.json"))
	var r diagnostics.Result
	if err := json.Unmarshal(b, &r); err != nil || r.Running || r.Network != "Failed" || !strings.Contains(r.Detail, "certificate") {
		t.Fatal("TLS failure missing", r, err)
	}
	// Malformed config is shown in the menu without erasing sync state or
	// publishing config values/credentials as executable action text.
	os.WriteFile(file, []byte(`{"provider":"unknown"}`), 0600)
	if err := menuControl(file, menu, "check"); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(menu)
	if !strings.Contains(string(b), "Configuration error") {
		t.Fatal("config failure not shown")
	}
}
