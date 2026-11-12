package transfer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) *Server {
	t.Helper()
	s, err := New(Config{Port: 8789, UIPort: 8790, DownloadDir: t.TempDir(), MaxUploadMB: 1})
	if err != nil {
		t.Fatal(err)
	}
	// Synthetic private-network address; these tests do not connect to a device.
	s.addresses = func() []string { return []string{"192.168.1.100"} }
	return s
}

func TestDevicePortalAPIBoundaries(t *testing.T) {
	s := fixture(t)
	calls := 0
	s.portal.DeviceAPI = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(204) })
	for _, tc := range []struct {
		method, host, token, origin string
		want                        int
	}{
		{"GET", "localhost:8790", s.localToken, "", 204},
		{"POST", "localhost:8790", s.localToken, "http://localhost:8790", 204},
		{"GET", "localhost:8790", "", "", 401},
		{"GET", "localhost:8790", s.token, "", 401},
		{"GET", "foreign.test:8790", s.localToken, "", 403},
		{"POST", "localhost:8790", s.localToken, "http://foreign.test", 403},
	} {
		r := httptest.NewRequest(tc.method, "http://"+tc.host+"/api/config", nil)
		r.Header.Set("X-Transfer-Token", tc.token)
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		s.LocalHandler().ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s %s: %d, want %d", tc.method, tc.host, w.Code, tc.want)
		}
	}
	if w := request(s, "GET", "/api/config", "", s.token); w.Code != 404 {
		t.Fatal("device API reached over LAN", w.Code)
	}
	if calls != 2 {
		t.Fatal("unauthorized handler call", calls)
	}
	s.portal.DeviceAPI = nil
	r := httptest.NewRequest("GET", "http://localhost:8790/api/config", nil)
	r.Header.Set("X-Transfer-Token", s.localToken)
	w := httptest.NewRecorder()
	s.LocalHandler().ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatal("transfer gained configuration API", w.Code)
	}
}
func request(s *Server, method, path, body, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://192.168.1.100:8789"+path, strings.NewReader(body))
	r.Header.Set("X-Transfer-Token", token)
	w := httptest.NewRecorder()
	s.LANHandler().ServeHTTP(w, r)
	return w
}
func send(s *Server, name string, r *http.Request) *httptest.ResponseRecorder {
	r.Header.Set("X-File-Name", url.PathEscape(name))
	r.Header.Set("X-Transfer-Token", s.token)
	w := httptest.NewRecorder()
	s.LANHandler().ServeHTTP(w, r)
	return w
}
func bookRequest(body string) *http.Request {
	return httptest.NewRequest("POST", "http://192.168.1.100:8789/api/upload", strings.NewReader(body))
}

func TestConfigIndependentOfCloud(t *testing.T) {
	file := filepath.Join(t.TempDir(), "transfer.json")
	c, err := Load(file)
	if err != nil || c.Port != 8789 || c.UIPort != 8790 || c.MaxUploadMB != 512 {
		t.Fatal(c, err)
	}
	for _, data := range []string{`{"port":8789,"ui_port":8789}`, `{"port":80}`, `{"download_dir":"relative"}`, `{"max_upload_mb":0}`, `{"bucket":"unknown"}`, `{} {}`} {
		os.WriteFile(file, []byte(data), 0600)
		if _, err = Load(file); err == nil {
			t.Fatal("accepted invalid config", data)
		}
	}
}

func TestAuthenticationAndServiceIsolation(t *testing.T) {
	s := fixture(t)
	if w := request(s, "POST", "/api/pair", `{"code":"`+s.code+`"}`, ""); w.Code != 200 || !strings.Contains(w.Body.String(), s.token) {
		t.Fatal(w.Code)
	}
	for _, token := range []string{"", s.localToken, "invalid"} {
		if w := request(s, "GET", "/api/info", "", token); w.Code != 401 {
			t.Fatal("unauthenticated access", w.Code)
		}
	}
	for _, path := range []string{"/api/config", "/api/jobs", "/api/connection", "/api/sync/status"} {
		if w := request(s, "GET", path, "", s.token); w.Code != 404 {
			t.Fatal("cloud/local endpoint exposed", path, w.Code)
		}
	}
	for _, host := range []string{"attacker.example:8789", "192.168.1.100:8790", "localhost:8789"} {
		r := bookRequest("book")
		r.Host = host
		if w := send(s, "book.azw3", r); w.Code != 403 {
			t.Fatal("bad host accepted", host)
		}
	}
	r := bookRequest("book")
	r.Header.Set("Origin", "https://attacker.example")
	if w := send(s, "book.azw3", r); w.Code != 403 {
		t.Fatal("bad origin accepted")
	}
	other := fixture(t)
	if w := request(other, "GET", "/api/info", "", s.token); w.Code != 401 {
		t.Fatal("old token accepted")
	}
	for i := 0; i < 10; i++ {
		if w := request(s, "POST", "/api/pair", `{"code":"000000"}`, ""); w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
	if w := request(s, "POST", "/api/pair", `{"code":"`+s.code+`"}`, ""); w.Code != 429 {
		t.Fatal("no rate limit")
	}
}

func TestLocalConnectionQRAndNoNetwork(t *testing.T) {
	s := fixture(t)
	for _, token := range []string{"", s.token, s.localToken} {
		r := httptest.NewRequest("GET", "http://localhost:8790/api/connection", nil)
		r.Header.Set("X-Transfer-Token", token)
		w := httptest.NewRecorder()
		s.LocalHandler().ServeHTTP(w, r)
		if token != s.localToken {
			if w.Code != 401 {
				t.Fatal("LAN token can access connection page")
			}
			continue
		}
		if w.Code != 200 {
			t.Fatal(w.Code)
		}
		var data struct {
			Code      string
			Endpoints []struct{ IP, URL, QR string }
		}
		if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
		if data.Code != s.code || len(data.Endpoints) != 1 || data.Endpoints[0].URL != "http://192.168.1.100:8789/" {
			t.Fatal("bad connection info")
		}
		b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(data.Endpoints[0].QR, "data:image/png;base64,"))
		if err != nil {
			t.Fatal(err)
		}
		image, err := png.Decode(bytes.NewReader(b))
		if err != nil || image.Bounds().Dx() != 300 {
			t.Fatal("invalid QR", err)
		}
	}
	s.addresses = func() []string { return nil }
	r := httptest.NewRequest("GET", "http://localhost:8790/api/connection", nil)
	r.Header.Set("X-Transfer-Token", s.localToken)
	w := httptest.NewRecorder()
	s.LocalHandler().ServeHTTP(w, r)
	if !strings.Contains(w.Body.String(), `"endpoints":[]`) {
		t.Fatal("no-network state invented an address")
	}
}

func TestUploadsPreserveNamesAndExistingBooks(t *testing.T) {
	s := fixture(t)
	for _, name := range []string{"中文 + & # 测试.azw3", "book.pdf", "book.pdf"} {
		w := send(s, name, bookRequest("valid-book-bytes"))
		if w.Code != 201 {
			t.Fatal(w.Code, w.Body.String())
		}
		var result struct {
			Name, SHA256 string
			Size         int
		}
		json.Unmarshal(w.Body.Bytes(), &result)
		b, err := os.ReadFile(filepath.Join(s.cfg.DownloadDir, result.Name))
		if err != nil || string(b) != "valid-book-bytes" || result.Size != len(b) || len(result.SHA256) != 64 {
			t.Fatal("stored data mismatch", err)
		}
	}
	if _, err := os.Stat(filepath.Join(s.cfg.DownloadDir, "book (2).pdf")); err != nil {
		t.Fatal("duplicate overwritten")
	}
	outside := filepath.Join(t.TempDir(), "protected")
	os.WriteFile(outside, []byte("original"), 0600)
	os.Symlink(outside, filepath.Join(s.cfg.DownloadDir, "link.txt"))
	if w := send(s, "link.txt", bookRequest("new")); w.Code != 201 {
		t.Fatal(w.Code)
	}
	b, _ := os.ReadFile(outside)
	if string(b) != "original" {
		t.Fatal("symlink target overwritten")
	}
	parts, _ := filepath.Glob(filepath.Join(s.cfg.DownloadDir, ".wireless-upload-*"))
	if len(parts) != 0 {
		t.Fatal("partial leaked")
	}
}

func TestRejectedAndInterruptedUploadsLeaveNoBooks(t *testing.T) {
	s := fixture(t)
	for _, name := range []string{"../escape.txt", "folder/book.txt", `C:\book.txt`, "book.epub", "book.exe", ".txt", "book.txt\x00"} {
		if w := send(s, name, bookRequest("book")); w.Code != 400 {
			t.Fatal("bad filename accepted", name, w.Code)
		}
	}
	for _, mode := range []string{"empty", "oversized", "streamed-oversized", "truncated", "cancelled"} {
		r := bookRequest("book")
		switch mode {
		case "empty":
			r = bookRequest("")
		case "oversized":
			r.ContentLength = 2 * 1024 * 1024
		case "streamed-oversized":
			r = bookRequest(strings.Repeat("x", 1024*1024+1))
			r.ContentLength = -1
		case "truncated":
			r.ContentLength = 100
		case "cancelled":
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			r = r.WithContext(ctx)
		}
		w := send(s, "book.azw3", r)
		if w.Code < 400 {
			t.Fatal("incomplete book committed", mode, w.Code)
		}
		entries, _ := os.ReadDir(s.cfg.DownloadDir)
		if len(entries) != 0 {
			t.Fatal("failed upload leaked files", mode)
		}
	}
}

type gatedReader struct {
	entered, release chan struct{}
	reader           io.Reader
	started          bool
}

func (g *gatedReader) Read(b []byte) (int, error) {
	if !g.started {
		g.started = true
		close(g.entered)
		<-g.release
	}
	return g.reader.Read(b)
}
func TestConcurrentUploadsAreBounded(t *testing.T) {
	s := fixture(t)
	g := &gatedReader{entered: make(chan struct{}), release: make(chan struct{}), reader: strings.NewReader("book")}
	r := httptest.NewRequest("POST", "http://192.168.1.100:8789/api/upload", g)
	finished := make(chan *httptest.ResponseRecorder, 1)
	go func() { finished <- send(s, "first.azw3", r) }()
	<-g.entered
	w := send(s, "second.azw3", bookRequest("second"))
	close(g.release)
	if w.Code != 409 {
		t.Fatal("concurrent upload was not bounded", w.Code)
	}
	if w = <-finished; w.Code != 201 {
		t.Fatal(w.Code)
	}
}

func TestBothListenersIgnoreVisitorLanguageAndRefreshPreference(t *testing.T) {
	s := fixture(t)
	s.cfg.FormatsFile = filepath.Join(t.TempDir(), "config.json")
	for _, language := range []string{"zh", "en"} {
		if err := os.WriteFile(s.cfg.FormatsFile, []byte(`{"language":"`+language+`"}`), 0600); err != nil {
			t.Fatal(err)
		}
		for _, local := range []bool{false, true} {
			host := "192.168.1.100:8789"
			handler := s.LANHandler()
			if local {
				host = "localhost:8790"
				handler = s.LocalHandler()
			}
			r := httptest.NewRequest("GET", "http://"+host+"/?lang=ja", nil)
			r.Header.Set("Accept-Language", "ja,en,zh")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != 200 || w.Header().Get("Content-Language") != language {
				t.Fatal(w.Code, w.Header())
			}
			expected := "Wireless transfer"
			if language == "zh" {
				expected = "无线传书"
			}
			if !strings.Contains(w.Body.String(), expected) {
				t.Fatal(w.Body.String())
			}
		}
	}
}
