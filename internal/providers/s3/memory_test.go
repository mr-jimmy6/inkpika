package s3_test

import (
	"bytes"
	"context"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
	"github.com/mr-jimmy6/inkpika/internal/config"
	"github.com/mr-jimmy6/inkpika/internal/download"
	"github.com/mr-jimmy6/inkpika/internal/providers"
	"github.com/mr-jimmy6/inkpika/internal/storage"
	"github.com/mr-jimmy6/inkpika/internal/syncer"
)

// This is an independent S3 HTTP implementation with an in-memory backend,
// not canned XML. Writes only seed/change test data; production stays read-only.
// gofakes3 does not verify SigV4 or If-Match, so client_test.go separately covers
// signing, preconditions, URL encoding, redirects and malformed responses.
type memoryService struct {
	db        *s3mem.Backend
	cfg       config.Config
	store     storage.Store
	interrupt atomic.Bool
	resumes   atomic.Int32
	pages     atomic.Int32
}

type shortResponse struct {
	http.ResponseWriter
	remaining int
}

// The Go-1.23-compatible release computes the correct Range body and headers,
// but omits status 206 (upstream fix: gofakes3 PR #121, July 2026). Apply only
// that HTTP status fix at the test boundary; never relax the production client.
type rangeStatusResponse struct {
	http.ResponseWriter
	wroteHeader bool
}

func (w *rangeStatusResponse) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	if status == http.StatusOK && w.Header().Get("Content-Range") != "" {
		status = http.StatusPartialContent
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *rangeStatusResponse) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

func (w *shortResponse) Write(p []byte) (int, error) {
	// Preserve the real Content-Length and headers, but simulate a broken body.
	n := min(w.remaining, len(p))
	_, err := w.ResponseWriter.Write(p[:n])
	w.remaining -= n
	return len(p), err
}

func newMemoryService(t *testing.T, preset string) *memoryService {
	t.Helper()
	f := &memoryService{db: s3mem.New()}
	if err := f.db.CreateBucket("books"); err != nil {
		t.Fatal(err)
	}
	handler := gofakes3.New(f.db).Server()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w = &rangeStatusResponse{ResponseWriter: w}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			t.Errorf("production provider attempted a write: %s", r.Method)
			http.Error(w, "read only", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Query().Get("continuation-token") != "" {
			f.pages.Add(1)
		}
		if r.Header.Get("Range") != "" {
			f.resumes.Add(1)
		}
		if r.Method == http.MethodGet && r.URL.Path == "/books/resume.azw3" && f.interrupt.CompareAndSwap(true, false) {
			handler.ServeHTTP(&shortResponse{ResponseWriter: w, remaining: 127}, r)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	ca := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	f.cfg = config.Config{Provider: config.S3, S3Preset: preset, S3Addressing: "path", Region: "us-east-1", Endpoint: srv.URL, CABundle: ca, Bucket: "books", AccessKeyID: "fixture-id", SecretAccessKey: "fixture-secret", Listen: "127.0.0.1:8787", StateDir: filepath.Join(dir, "state"), DownloadDir: filepath.Join(dir, "documents"), MaxDownloadMB: 8}
	if preset == "r2" {
		f.cfg.Region = "auto"
	}
	if err := f.cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	var err error
	f.store, err = providers.New(f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *memoryService) put(t *testing.T, key string, data []byte) {
	t.Helper()
	if _, err := f.db.PutObject("books", key, map[string]string{}, bytes.NewReader(data), int64(len(data))); err != nil {
		t.Fatal(err)
	}
}

func readBody(t *testing.T, resp *http.Response, want []byte) {
	t.Helper()
	defer resp.Body.Close()
	got, err := io.ReadAll(resp.Body)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("object bytes differ: got %d bytes, want %d: %v", len(got), len(want), err)
	}
}

func TestMemoryS3PresetsReadObjects(t *testing.T) {
	for _, preset := range []string{"aws", "r2", "b2", "spaces", "custom"} {
		t.Run(preset, func(t *testing.T) {
			f := newMemoryService(t, preset)
			key, data := "书架/中文 +#%?&=.azw3", []byte("test ebook contents")
			f.put(t, key, data)
			list, err := f.store.List(context.Background(), "书架/", "")
			if err != nil || len(list.Keys) != 1 || list.Keys[0].Key != key || list.Keys[0].Size != int64(len(data)) {
				t.Fatalf("listing: %+v %v", list, err)
			}
			head, err := f.store.Head(context.Background(), key)
			if err != nil {
				t.Fatal(err)
			}
			head.Body.Close()
			if head.ContentLength != int64(len(data)) || head.Header.Get("ETag") == "" {
				t.Fatalf("invalid HEAD metadata: %v", head.Header)
			}
			for _, offset := range []int64{0, 5} {
				resp, err := f.store.Get(context.Background(), key, offset, head.Header.Get("ETag"))
				if err != nil {
					t.Fatal(err)
				}
				if offset > 0 && (resp.StatusCode != 206 || resp.Header.Get("Content-Range") != "bytes 5-18/19") {
					t.Error("missing partial response", resp.StatusCode, resp.Header)
				}
				readBody(t, resp, data[offset:])
			}
			if _, err := f.store.Head(context.Background(), "missing.azw3"); err == nil || !strings.Contains(err.Error(), "404") {
				t.Fatalf("missing object should fail: %v", err)
			}
			if _, err := f.store.Get(context.Background(), key, int64(len(data)+1), ""); err == nil || !strings.Contains(err.Error(), "416") {
				t.Fatalf("invalid range should fail: %v", err)
			}
		})
	}
}

func TestMemoryS3InterruptedFiveMBDownload(t *testing.T) {
	f := newMemoryService(t, "custom")
	data := bytes.Repeat([]byte("0123456789"), 500000)
	f.put(t, "resume.azw3", data)
	f.interrupt.Store(true)
	m, err := download.New(f.cfg, f.store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	job, err := m.Add("resume.azw3")
	if err != nil {
		t.Fatal(err)
	}
	await := func(status string) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			for _, j := range m.Snapshot() {
				if j.ID == job.ID && j.Status == status {
					return
				}
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatalf("want %s: %+v", status, m.Snapshot())
	}
	await("failed")
	if _, err := os.Stat(m.Target(job.Key)); !os.IsNotExist(err) {
		t.Fatal("interrupted book became visible", err)
	}
	if err := m.Action(job.ID, "retry"); err != nil {
		t.Fatal(err)
	}
	await("completed")
	got, err := os.ReadFile(m.Target(job.Key))
	if err != nil || !bytes.Equal(got, data) || f.resumes.Load() != 1 {
		t.Fatalf("resume failed: %v, range requests %d", err, f.resumes.Load())
	}
}

func TestMemoryS3RecursiveSyncAndRemoteChanges(t *testing.T) {
	f := newMemoryService(t, "custom")
	if err := f.db.SetVersioningConfiguration("books", gofakes3.VersioningConfiguration{Status: gofakes3.VersioningEnabled}); err != nil {
		t.Fatal(err)
	}
	books := map[string][]byte{"sync/子目录/中文 +%.azw3": []byte("nested book")}
	for i := 0; i < 12; i++ {
		key := fmt.Sprintf("sync/book-%02d.azw3", i)
		books[key] = []byte(fmt.Sprintf("book %02d contents", i))
	}
	for key, data := range books {
		f.put(t, key, data)
	}
	f.put(t, "outside.azw3", []byte("outside sync"))
	f.put(t, "sync/unsupported.exe", []byte("unsupported format"))
	m, err := download.New(f.cfg, f.store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	e, err := syncer.New(f.cfg, f.store, m, filepath.Join(t.TempDir(), "menu.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Close)
	run := func(fetch bool) syncer.Snapshot {
		t.Helper()
		if err := e.Start(fetch); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			s := e.Snapshot()
			if !s.Running {
				if s.Error != "" {
					t.Fatal(s.Error)
				}
				return s
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatal("sync timed out")
		return syncer.Snapshot{}
	}
	if s := run(true); s.Total != len(books) || s.Fetched != len(books) || s.Pending != 0 || f.pages.Load() < 2 {
		t.Fatalf("recursive paginated sync: %+v, pages %d", s, f.pages.Load())
	}
	for key, want := range books {
		got, err := os.ReadFile(m.Target(key))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("synced bytes differ for %q: %v", key, err)
		}
	}
	if s := run(true); s.Fetched != 0 || s.Pending != 0 {
		t.Fatal("unchanged books should be reused", s)
	}
	key := "sync/book-00.azw3"
	f.put(t, key, books[key]) // Same content/ETag, new server-side version.
	if s := run(false); s.Pending != 1 {
		t.Fatal("version-only change was missed", s)
	}
	want := []byte("updated edition")
	f.put(t, key, want)
	if s := run(true); s.Fetched != 1 || s.Pending != 0 {
		t.Fatal("changed object not reconciled", s)
	}
	if got, err := os.ReadFile(m.Target(key)); err != nil || !bytes.Equal(got, want) {
		t.Fatal("verified replacement missing", err)
	}
	if _, err := f.db.DeleteObject("books", key); err != nil {
		t.Fatal(err)
	}
	if s := run(true); s.Total != len(books)-1 || s.Fetched != 0 {
		t.Fatal("remote deletion not reflected", s)
	}
	if got, err := os.ReadFile(m.Target(key)); err != nil || !bytes.Equal(got, want) {
		t.Fatal("remote deletion removed local book", err)
	}
}
