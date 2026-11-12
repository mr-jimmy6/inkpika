package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mr-jimmy6/inkpika/internal/config"
)

func TestSyncControlUsesInkPikaSession(t *testing.T) {
	token := strings.Repeat("a", 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/sync/status" || r.Header.Get("X-InkPika-Token") != token {
			t.Errorf("invalid local sync request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"phase":"idle"}`)
	}))
	defer server.Close()
	address, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	c := config.Config{Listen: address.Host, StateDir: t.TempDir()}
	session := "http://" + c.BrowserAddress() + "/#token=" + token + "\n"
	if err = os.WriteFile(filepath.Join(c.StateDir, "launch.url"), []byte(session), 0600); err != nil {
		t.Fatal(err)
	}
	if err = syncControl(c, "status"); err != nil {
		t.Fatal(err)
	}
	if err = syncControl(c, "unexpected"); err == nil {
		t.Fatal("unknown command accepted")
	}
}
