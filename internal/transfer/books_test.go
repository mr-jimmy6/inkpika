package transfer

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func localFixture(t *testing.T) *Server {
	s := fixture(t)
	s.cfg.LibraryDir = s.cfg.DownloadDir
	s.cfg.FormatsFile = filepath.Join(t.TempDir(), "config.json")
	for name, body := range map[string]string{"中文书.azw3": "book-cn", "nested/book.pdf": "pdf", "notes.txt": "text", "book.epub": "epub", "private.json": "secret", ".hidden.txt": "hidden", "book.sdr/metadata.txt": "hidden"} {
		p := filepath.Join(s.cfg.LibraryDir, name)
		os.MkdirAll(filepath.Dir(p), 0700)
		os.WriteFile(p, []byte(body), 0600)
	}
	outside := filepath.Join(t.TempDir(), "secret.txt")
	os.WriteFile(outside, []byte("do-not-export"), 0600)
	os.Symlink(outside, filepath.Join(s.cfg.LibraryDir, "linked.txt"))
	return s
}

func listIDs(t *testing.T, s *Server, query string) []localBook {
	t.Helper()
	w := request(s, "GET", "/api/books"+query, "", s.token)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var out struct{ Books []localBook }
	json.Unmarshal(w.Body.Bytes(), &out)
	return out.Books
}

func exportURL(t *testing.T, s *Server, ids []string) string {
	t.Helper()
	b, _ := json.Marshal(map[string]interface{}{"ids": ids})
	w := request(s, "POST", "/api/export", string(b), s.token)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var out struct{ URL string }
	json.Unmarshal(w.Body.Bytes(), &out)
	return out.URL
}

func TestLocalBooksSearchBatchDownloadAndIsolation(t *testing.T) {
	s := localFixture(t)
	books := listIDs(t, s, "")
	if len(books) != 3 {
		t.Fatalf("unexpected local catalogue: %+v", books)
	}
	if got := listIDs(t, s, "?q=BOOK"); len(got) != 1 || got[0].Name != "book.pdf" {
		t.Fatal(got)
	}
	ids := []string{}
	for _, b := range books {
		ids = append(ids, b.ID)
	}
	url := exportURL(t, s, ids)
	r := httptest.NewRequest("GET", "http://192.168.1.100:8789"+url, nil)
	r.AddCookie(&http.Cookie{Name: s.cookieName(), Value: s.token})
	w := httptest.NewRecorder()
	s.LANHandler().ServeHTTP(w, r)
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/zip" {
		t.Fatal(w.Code, w.Body.String())
	}
	z, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(z.File) != 3 {
		t.Fatal("batch lost books")
	}
	for _, f := range z.File {
		r, _ := f.Open()
		b, _ := io.ReadAll(r)
		r.Close()
		if strings.Contains(string(b), "secret") || strings.Contains(string(b), "hidden") {
			t.Fatal("private file exposed")
		}
	}
	if w := request(s, "GET", url, "", s.token); w.Code != 410 {
		t.Fatal("export ticket replay accepted")
	}
	if w := request(s, "POST", "/api/export", `{"ids":["../../etc/passwd"]}`, s.token); w.Code != 409 {
		t.Fatal("path accepted")
	}
	url = exportURL(t, s, ids[:1])
	w = request(s, "GET", url, "", s.token)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatal(w.Code)
	}
}

func TestFormatsApplyToUploadsAndListAndExistingExport(t *testing.T) {
	s := localFixture(t)
	books := listIDs(t, s, "")
	url := exportURL(t, s, []string{books[0].ID})
	os.WriteFile(s.cfg.FormatsFile, []byte(`{"ebook_formats":["epub"]}`), 0600)
	if w := request(s, "GET", url, "", s.token); w.Code != 409 {
		t.Fatal("old export bypassed changed format rules")
	}
	if b := listIDs(t, s, ""); len(b) != 1 || b[0].Name != "book.epub" {
		t.Fatal(b)
	}
	if w := send(s, "new.azw3", bookRequest("book")); w.Code != 400 {
		t.Fatal("disallowed upload accepted")
	}
	if w := send(s, "new.epub", bookRequest("book")); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestExportRejectsReplacedFilesAndDirectorySymlinks(t *testing.T) {
	s := localFixture(t)
	book := listIDs(t, s, "?q=book.pdf")[0]
	url := exportURL(t, s, []string{book.ID})
	p := filepath.Join(s.cfg.LibraryDir, "nested/book.pdf")
	os.Remove(p)
	os.Symlink(filepath.Join(s.cfg.LibraryDir, "notes.txt"), p)
	if w := request(s, "GET", url, "", s.token); w.Code != 409 {
		t.Fatal("replaced file exported")
	}
}

func TestWiFiNameUsesOnlyCurrentConnectedNetwork(t *testing.T) {
	if got := parseWiFiName("Hash Index: 0\n\tessid = \"测试 Wi-Fi\"\n\tconnected = \"yes\"\n"); got != "测试 Wi-Fi" {
		t.Fatal(got)
	}
	if got := parseWiFiName("essid = \"old\"\nconnected = \"no\""); got != "" {
		t.Fatal("stale SSID exposed")
	}
}
