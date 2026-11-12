package diagnostics

import (
	"context"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mr-jimmy6/inkpika/internal/config"
	"github.com/mr-jimmy6/inkpika/internal/storage"
)

type body struct {
	read   int
	closed bool
}

func TestProbeUsesCustomCAAndConfiguredProxy(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/books/" || r.Method != "HEAD" || r.Header.Get("Authorization") != "" {
			t.Error("probe sent credentials or wrong bucket address")
		}
		w.WriteHeader(403)
	}))
	defer server.Close()
	ca := filepath.Join(t.TempDir(), "ca.pem")
	os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600)
	c := config.Config{Provider: config.S3, S3Addressing: "path", Endpoint: server.URL, Bucket: "books", CABundle: ca}
	if err := Probe(context.Background(), c); err != nil {
		t.Fatal("custom CA ignored", err)
	}
	c.SOCKS5Proxy = "http://reader:proxy-secret@invalid:1080"
	if err := Probe(context.Background(), c); err == nil {
		t.Fatal("probe ignored invalid proxy")
	}
}

func TestProxyCredentialRedaction(t *testing.T) {
	c := config.Config{SOCKS5Proxy: "socks5://proxy-user:pass%3A%40word@proxy.invalid:1080"}
	text := Redact(c, c.SOCKS5Proxy+" proxy-user pass:@word pass%3A%40word")
	for _, secret := range []string{"proxy-user", "pass:@word", "pass%3A%40word"} {
		if strings.Contains(text, secret) {
			t.Fatal("proxy credential leaked")
		}
	}
}

func (b *body) Read(p []byte) (int, error) {
	b.read += len(p)
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}
func (b *body) Close() error { b.closed = true; return nil }

type testStore struct {
	listErr, getErr error
	empty, loop     bool
	lists, gets     int
	body            body
}

func (s *testStore) List(_ context.Context, prefix, marker string) (storage.Listing, error) {
	s.lists++
	if s.listErr != nil {
		return storage.Listing{}, s.listErr
	}
	if s.empty {
		return storage.Listing{}, nil
	}
	if s.loop {
		return storage.Listing{HasMore: true, NextMarker: "same"}, nil
	}
	if prefix == "allowed/" {
		return storage.Listing{Folders: []string{"outside/", "allowed/books/"}}, nil
	}
	return storage.Listing{Keys: []storage.Object{{Key: "allowed/books/test.azw3", Size: 1024}}}, nil
}
func (s *testStore) Head(context.Context, string) (*http.Response, error) { panic("not used") }
func (s *testStore) Get(_ context.Context, key string, _ int64, _ string) (*http.Response, error) {
	if !strings.HasPrefix(key, "allowed/") {
		panic("out of scope")
	}
	s.gets++
	if s.getErr != nil {
		return nil, s.getErr
	}
	return &http.Response{StatusCode: 200, Body: &s.body}, nil
}
func TestConnectionStagesAndBoundedSample(t *testing.T) {
	c := config.Config{Provider: config.AliyunOSS, Prefix: "allowed/", AccessKeyID: "private-id", SecretAccessKey: "private-secret"}
	pass := func(context.Context, config.Config) error { return nil }
	for _, tc := range []struct {
		name                  string
		store                 testStore
		probeErr              error
		network, access, read string
	}{
		{"network", testStore{}, errors.New("DNS lookup failed"), "Failed", "Not checked", "Not verified"},
		{"forbidden", testStore{listErr: errors.New("HTTP 403 AccessDenied private-secret")}, nil, "Passed (HTTPS)", "Failed", "Not verified"},
		{"read-denied", testStore{getErr: errors.New("HTTP 403 GetObject denied")}, nil, "Passed (HTTPS)", "Passed (current prefix)", "Failed"},
		{"empty", testStore{empty: true}, nil, "Passed (HTTPS)", "Passed (current prefix)", "Not verified"},
		{"success", testStore{}, nil, "Passed (HTTPS)", "Passed (current prefix)", "Passed (sample)"},
		{"loop", testStore{loop: true}, nil, "Passed (HTTPS)", "Passed (current prefix)", "Not verified"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probe := pass
			if tc.probeErr != nil {
				probe = func(context.Context, config.Config) error { return tc.probeErr }
			}
			r := Check(context.Background(), c, &tc.store, probe)
			if r.Network != tc.network || r.Access != tc.access || r.Read != tc.read || r.Running || r.Updated == 0 {
				t.Fatal(r)
			}
			if strings.Contains(r.Detail, c.SecretAccessKey) {
				t.Fatal("credential exposed")
			}
			if tc.name == "network" && tc.store.lists > 0 {
				t.Fatal("tried access after failed connection")
			}
			if tc.name == "success" && (!tc.store.body.closed || tc.store.body.read != 1) {
				t.Fatal("sample not bounded/closed", tc.store.body)
			}
			if tc.name == "loop" && tc.store.lists != 2 {
				t.Fatal("pagination loop not stopped")
			}
		})
	}
}
func TestProbeRejectsUntrustedTLSAndCancellation(t *testing.T) {
	// Verified TLS is a requirement even for the unauthenticated connectivity step.
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) }))
	defer server.Close()
	if err := Probe(context.Background(), config.Config{Endpoint: server.URL, Provider: config.QingStor, Bucket: "books"}); err == nil {
		t.Fatal("untrusted TLS accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Probe(ctx, config.Config{Endpoint: "https://example.invalid", Provider: config.QingStor, Bucket: "books"}); err == nil {
		t.Fatal("cancelled probe succeeded")
	}
}
