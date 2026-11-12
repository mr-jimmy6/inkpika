package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mr-jimmy6/inkpika/internal/config"
	"github.com/mr-jimmy6/inkpika/internal/download"
	"github.com/mr-jimmy6/inkpika/internal/storage"
	"github.com/mr-jimmy6/inkpika/internal/syncer"
)

func TestLocalSessionAndSecretIsolation(t *testing.T) {
	dir := t.TempDir()
	c := config.Config{Bucket: "books", AccessKeyID: "SECRET-ID", SecretAccessKey: "SECRET-KEY", Listen: "127.0.0.1:8787", DownloadDir: filepath.Join(dir, "documents"), StateDir: filepath.Join(dir, "state"), MaxDownloadMB: 1}
	client := fakeStore{}
	m, err := download.New(c, client)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	h := New(c, client, m, "token")
	for _, tc := range []struct {
		name, host, token, origin, path, method string
		code                                    int
	}{
		{"missing token", c.Listen, "", "", "/api/info", "GET", 401},
		{"bad token", c.Listen, "wrong", "", "/api/info", "GET", 401},
		{"DNS rebinding", "attacker.test:8787", "token", "", "/api/info", "GET", 403},
		{"valid", c.Listen, "token", "", "/api/info", "GET", 200},
		{"Kindle browser hostname", "localhost:8787", "token", "", "/api/info", "GET", 200},
		{"Kindle browser POST", "localhost:8787", "token", "http://localhost:8787", "/api/jobs/clear", "POST", 200},
		{"foreign origin on localhost", "localhost:8787", "token", "https://attacker.test", "/api/jobs/clear", "POST", 403},
		{"foreign origin", c.Listen, "token", "https://attacker.test", "/api/jobs/clear", "POST", 403},
		{"no secret route", c.Listen, "token", "", "/config.json", "GET", 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "http://"+tc.host+tc.path, strings.NewReader("{}"))
			r.Host = tc.host
			r.Header.Set("X-InkPika-Token", tc.token)
			r.Header.Set("Origin", tc.origin)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.code {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "SECRET") {
				t.Fatal("credentials exposed")
			}
		})
	}
}

// A provider-independent fake proves that the web API accepts the common interface.
type fakeStore struct{}

func (fakeStore) List(context.Context, string, string) (storage.Listing, error) {
	return storage.Listing{Keys: []storage.Object{{Key: "books/中文.azw3", Size: 12, Modified: 1793979982}}, Folders: []string{"books/子目录/"}}, nil
}
func (fakeStore) Head(context.Context, string) (*http.Response, error) {
	return &http.Response{ContentLength: 12, Body: http.NoBody}, nil
}
func (fakeStore) Get(context.Context, string, int64, string) (*http.Response, error) {
	return &http.Response{StatusCode: 200, ContentLength: 12, Body: io.NopCloser(strings.NewReader("hello kindle"))}, nil
}

func TestListingReportsDownloadedFilesAfterHistoryClear(t *testing.T) {
	dir := t.TempDir()
	c := config.Config{Bucket: "books", Listen: "127.0.0.1:8787", DownloadDir: filepath.Join(dir, "documents"), StateDir: filepath.Join(dir, "state"), MaxDownloadMB: 1}
	m, err := download.New(c, fakeStore{})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	h := New(c, fakeStore{}, m, "token")
	get := func(route string, out interface{}) {
		t.Helper()
		r := httptest.NewRequest("GET", "http://"+c.Listen+route, nil)
		r.Header.Set("X-InkPika-Token", "token")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
			t.Fatal(err)
		}
	}
	type listing struct {
		Keys []struct {
			Downloaded bool  `json:"downloaded"`
			Modified   int64 `json:"modified"`
		} `json:"keys"`
	}
	var before listing
	get("/api/objects", &before)
	if len(before.Keys) != 1 || before.Keys[0].Downloaded || before.Keys[0].Modified != 1793979982 {
		t.Fatalf("bad listing: %+v", before)
	}
	if _, err = m.Add("books/中文.azw3"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(m.Snapshot()) != 1 || m.Snapshot()[0].Status != "completed" {
		if time.Now().After(deadline) {
			t.Fatal("download did not complete")
		}
		time.Sleep(10 * time.Millisecond)
	}
	var jobs []struct {
		SavedLocally     bool   `json:"saved_locally"`
		LocalState       string `json:"local_state"`
		TransferComplete bool   `json:"transfer_complete"`
	}
	get("/api/jobs", &jobs)
	if len(jobs) != 1 || !jobs[0].SavedLocally || jobs[0].LocalState != "saved" || !jobs[0].TransferComplete {
		t.Fatalf("missing completion update: %+v", jobs)
	}
	file := m.Target("books/中文.azw3")
	original, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(file); err != nil {
		t.Fatal(err)
	}
	get("/api/jobs", &jobs)
	if jobs[0].SavedLocally || jobs[0].LocalState != "removed" || !jobs[0].TransferComplete {
		t.Fatalf("history incorrectly claims removed file is saved: %+v", jobs)
	}
	if err = os.WriteFile(file, append(original, 'x'), 0600); err != nil {
		t.Fatal(err)
	}
	get("/api/jobs", &jobs)
	if jobs[0].LocalState != "changed" {
		t.Fatal("changed local file reported as intact")
	}
	if err = os.WriteFile(file, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err = m.Clear(); err != nil {
		t.Fatal(err)
	}
	var after listing
	get("/api/objects", &after)
	if len(after.Keys) != 1 || !after.Keys[0].Downloaded {
		t.Fatalf("cleared history lost downloaded flag: %+v", after)
	}
}

func TestProviderMetadataAndCommonListing(t *testing.T) {
	for _, provider := range []string{config.QingStor, config.AliyunOSS} {
		t.Run(provider, func(t *testing.T) {
			dir := t.TempDir()
			c := config.Config{Provider: provider, Region: "region", Bucket: "books", SecurityToken: "SECRET-STS", Prefix: "books/", Listen: "127.0.0.1:8787", StateDir: filepath.Join(dir, "state"), DownloadDir: filepath.Join(dir, "documents"), MaxDownloadMB: 1}
			m, err := download.New(c, fakeStore{})
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			h := New(c, fakeStore{}, m, "token")
			for _, route := range []string{"/api/info", "/api/objects"} {
				r := httptest.NewRequest("GET", "http://"+c.Listen+route, nil)
				r.Header.Set("X-InkPika-Token", "token")
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if w.Code != 200 || strings.Contains(w.Body.String(), "SECRET") {
					t.Fatal(w.Code, w.Body.String())
				}
				if route == "/api/info" {
					var out map[string]interface{}
					if err = json.Unmarshal(w.Body.Bytes(), &out); err != nil {
						t.Fatal(err)
					}
					if out["provider"] != provider || out["provider_label"] != c.ProviderLabel() {
						t.Fatal(out)
					}
				} else if !strings.Contains(w.Body.String(), "中文.azw3") {
					t.Fatal(w.Body.String())
				}
			}
		})
	}
}

func TestSyncControlUsesExistingSessionBoundary(t *testing.T) {
	dir := t.TempDir()
	c := config.Config{Listen: "127.0.0.1:8787", DownloadDir: filepath.Join(dir, "books"), StateDir: filepath.Join(dir, "state")}
	store := fakeStore{}
	m, err := download.New(c, store)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	engine, err := syncer.New(c, store, m, "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	handler := New(c, store, m, "secret-token", engine)
	for _, tc := range []struct {
		method, command, token, origin string
		code                           int
	}{
		{"GET", "status", "", "", 401},
		{"GET", "status", "secret-token", "", 200},
		{"POST", "start", "secret-token", "https://foreign.test", 403},
		{"GET", "start", "secret-token", "", 405},
		{"POST", "unknown", "secret-token", "", 404},
		{"POST", "scan", "secret-token", "", 202},
		{"POST", "refresh", "secret-token", "", 202},
		{"POST", "cancel", "secret-token", "", 202},
	} {
		r := httptest.NewRequest(tc.method, "http://"+c.Listen+"/api/sync/"+tc.command, nil)
		r.Header.Set("X-InkPika-Token", tc.token)
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.code {
			t.Fatalf("%s %s: %d %s", tc.method, tc.command, w.Code, w.Body.String())
		}
	}
}

func TestConfiguredFormatsFilterCloudListingAndDownload(t *testing.T) {
	dir := t.TempDir()
	c := config.Config{Bucket: "books", Listen: "127.0.0.1:8787", DownloadDir: filepath.Join(dir, "books"), StateDir: filepath.Join(dir, "state"), MaxDownloadMB: 1, EbookFormats: []string{"pdf"}}
	m, err := download.New(c, fakeStore{})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	h := New(c, fakeStore{}, m, "token")
	r := httptest.NewRequest("GET", "http://"+c.Listen+"/api/objects", nil)
	r.Header.Set("X-InkPika-Token", "token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var data struct{ Keys []storage.Object }
	json.Unmarshal(w.Body.Bytes(), &data)
	if w.Code != 200 || len(data.Keys) != 0 {
		t.Fatal("disallowed AZW3 still listed", w.Body.String())
	}
	if _, err = m.Add("books/中文.azw3"); err == nil {
		t.Fatal("direct download bypassed formats")
	}
}
