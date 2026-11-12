package download

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mr-jimmy6/inkpika/internal/config"
)

type fakeStore struct {
	mu          sync.Mutex
	data        string
	etag        string
	offsets     []int64
	truncate    bool
	ignoreRange bool
	badRange    bool
	started     chan struct{}
	blocked     bool
}

func (s *fakeStore) Head(ctx context.Context, key string) (*http.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &http.Response{ContentLength: int64(len(s.data)), Header: http.Header{"Etag": {s.etag}}, Body: http.NoBody}, nil
}
func (s *fakeStore) Get(ctx context.Context, key string, offset int64, etag string) (*http.Response, error) {
	s.mu.Lock()
	s.offsets = append(s.offsets, offset)
	data := s.data
	code := 200
	header := http.Header{"Etag": {s.etag}}
	if offset > 0 && !s.ignoreRange {
		data = data[offset:]
		code = 206
		header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, len(s.data)-1, len(s.data)))
	}
	if s.badRange {
		header.Set("Content-Range", "bytes 0-1/2")
	}
	length := int64(len(data))
	if s.truncate {
		data = data[:3]
		s.truncate = false
	}
	blocked := s.blocked
	started := s.started
	s.blocked = false
	s.mu.Unlock()
	if blocked {
		if started != nil {
			close(started)
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return &http.Response{StatusCode: code, Header: header, ContentLength: length, Body: io.NopCloser(strings.NewReader(data))}, nil
}
func testConfig(t *testing.T) config.Config {
	t.Helper()
	dir := t.TempDir()
	return config.Config{Bucket: "books", Endpoint: "https://storage.example.test", DownloadDir: filepath.Join(dir, "documents"), StateDir: filepath.Join(dir, "state"), MaxDownloadMB: 1}
}
func newManager(t *testing.T, c config.Config, s Store) *Manager {
	t.Helper()
	m, err := New(c, s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	return m
}
func awaitStatus(t *testing.T, m *Manager, id, status string) Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, j := range m.Snapshot() {
			if j.ID == id && j.Status == status {
				return j
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected %s; got %+v", status, m.Snapshot())
	return Job{}
}

func TestResumeInterruptedDownload(t *testing.T) {
	c := testConfig(t)
	s := &fakeStore{data: "0123456789", etag: `"v1"`, truncate: true}
	m := newManager(t, c, s)
	j, err := m.Add("中文/书.azw3")
	if err != nil {
		t.Fatal(err)
	}
	awaitStatus(t, m, j.ID, "failed")
	if _, err = os.Stat(filepath.Join(c.DownloadDir, j.Name)); !os.IsNotExist(err) {
		t.Fatal("partial exposed in library")
	}
	if err = m.Action(j.ID, "retry"); err != nil {
		t.Fatal(err)
	}
	awaitStatus(t, m, j.ID, "completed")
	data, err := os.ReadFile(filepath.Join(c.DownloadDir, j.Name))
	if err != nil || string(data) != s.data {
		t.Fatalf("bad content %q %v", data, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.offsets) != 2 || s.offsets[1] != 3 {
		t.Fatalf("not resumed: %v", s.offsets)
	}
}

func TestRangeAndObjectChanges(t *testing.T) {
	for _, mode := range []string{"ignored", "changed", "invalid"} {
		t.Run(mode, func(t *testing.T) {
			c := testConfig(t)
			s := &fakeStore{data: "0123456789", etag: `"v1"`, truncate: true}
			m := newManager(t, c, s)
			j, err := m.Add("book.mobi")
			if err != nil {
				t.Fatal(err)
			}
			awaitStatus(t, m, j.ID, "failed")
			s.mu.Lock()
			switch mode {
			case "ignored":
				s.ignoreRange = true
			case "changed":
				s.data = "abcdefghij"
				s.etag = `"v2"`
			case "invalid":
				s.badRange = true
			}
			s.mu.Unlock()
			if err = m.Action(j.ID, "retry"); err != nil {
				t.Fatal(err)
			}
			if mode == "invalid" {
				awaitStatus(t, m, j.ID, "failed")
				if _, err = os.Stat(filepath.Join(c.DownloadDir, j.Name)); !os.IsNotExist(err) {
					t.Fatal("invalid range exposed")
				}
				return
			}
			awaitStatus(t, m, j.ID, "completed")
			data, _ := os.ReadFile(filepath.Join(c.DownloadDir, j.Name))
			if string(data) != s.data {
				t.Fatalf("corrupt result %q", data)
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			if mode == "changed" && s.offsets[1] != 0 {
				t.Fatal("changed object not restarted")
			}
		})
	}
}

func TestNameSafetyAndNoOverwrite(t *testing.T) {
	c := testConfig(t)
	s := &fakeStore{data: "book", etag: `"v1"`}
	m := newManager(t, c, s)
	a, err := m.Add("../../中文:书?.pdf")
	if err != nil {
		t.Fatal(err)
	}
	awaitStatus(t, m, a.ID, "completed")
	b, err := m.Add("other/中文:书?.pdf")
	if err != nil {
		t.Fatal(err)
	}
	awaitStatus(t, m, b.ID, "completed")
	if a.Name == b.Name || strings.ContainsAny(a.Name, `/\:?`) {
		t.Fatal("unsafe/colliding name")
	}
	if _, err = m.Add("book.epub"); err == nil {
		t.Fatal("epub accepted")
	}
	if err = m.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Add(a.Key); err == nil {
		t.Fatal("existing file accepted for overwrite")
	}
	if data, err := os.ReadFile(filepath.Join(c.DownloadDir, a.Name)); err != nil || string(data) != "book" {
		t.Fatal("clearing history removed book")
	}
}

func TestCancelThenContinue(t *testing.T) {
	c := testConfig(t)
	s := &fakeStore{data: "book", etag: `"v1"`, blocked: true, started: make(chan struct{})}
	m := newManager(t, c, s)
	j, err := m.Add("book.txt")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.started:
	case <-time.After(5 * time.Second):
		t.Fatal("not started")
	}
	if err = m.Action(j.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		err = m.Action(j.ID, "retry")
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	awaitStatus(t, m, j.ID, "completed")
}

func TestDownloadedTracksFinalFileAfterHistoryClearAndRestart(t *testing.T) {
	c := testConfig(t)
	s := &fakeStore{data: "a complete book", etag: `"v1"`}
	m, err := New(c, s)
	if err != nil {
		t.Fatal(err)
	}
	key := "中文/下载状态.azw3"
	if m.IsDownloaded(key) {
		t.Fatal("missing book marked downloaded")
	}
	j, err := m.Add(key)
	if err != nil {
		m.Close()
		t.Fatal(err)
	}
	awaitStatus(t, m, j.ID, "completed")
	if err = m.Clear(); err != nil {
		m.Close()
		t.Fatal(err)
	}
	if !m.IsDownloaded(key) {
		t.Fatal("clearing history lost local book status")
	}
	m.Close()
	m = newManager(t, c, s)
	if !m.IsDownloaded(key) {
		t.Fatal("restart lost local book status")
	}
	otherConfig := c
	otherConfig.Bucket = "another-bucket"
	otherConfig.StateDir = filepath.Join(t.TempDir(), "state")
	other := newManager(t, otherConfig, s)
	if other.IsDownloaded(key) {
		t.Fatal("different bucket inherited local book status")
	}
	filename := filepath.Join(c.DownloadDir, j.Name)
	if err = os.Remove(filename); err != nil {
		t.Fatal(err)
	}
	if m.IsDownloaded(key) {
		t.Fatal("deleted book still marked downloaded")
	}
	if err = os.Mkdir(filename, 0700); err != nil {
		t.Fatal(err)
	}
	if m.IsDownloaded(key) {
		t.Fatal("directory marked downloaded")
	}
}

func TestDeletedCompletedBookCanBeDownloadedAgain(t *testing.T) {
	c := testConfig(t)
	s := &fakeStore{data: "a complete book", etag: `"v1"`}
	m := newManager(t, c, s)
	j, err := m.Add("book.azw3")
	if err != nil {
		t.Fatal(err)
	}
	awaitStatus(t, m, j.ID, "completed")
	if err = os.Remove(filepath.Join(c.DownloadDir, j.Name)); err != nil {
		t.Fatal(err)
	}
	again, err := m.Add(j.Key)
	if err != nil {
		t.Fatal(err)
	}
	if again.Status != "queued" {
		t.Fatalf("deleted book not queued again: %+v", again)
	}
	awaitStatus(t, m, again.ID, "completed")
	data, err := os.ReadFile(filepath.Join(c.DownloadDir, j.Name))
	if err != nil || string(data) != s.data {
		t.Fatal("book was not restored", err)
	}
	if len(m.Snapshot()) != 1 {
		t.Fatal("duplicate download records")
	}
}

func TestRestartPreservesPartial(t *testing.T) {
	c := testConfig(t)
	s := &fakeStore{data: "0123456789", etag: `"v1"`, truncate: true}
	m, err := New(c, s)
	if err != nil {
		t.Fatal(err)
	}
	j, err := m.Add("book.mobi")
	if err != nil {
		t.Fatal(err)
	}
	awaitStatus(t, m, j.ID, "failed")
	m.Close()
	m = newManager(t, c, s)
	if err = m.Action(j.ID, "retry"); err != nil {
		t.Fatal(err)
	}
	awaitStatus(t, m, j.ID, "completed")
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.offsets[1] != 3 {
		t.Fatal(s.offsets)
	}
}

func TestLimitAndPrefix(t *testing.T) {
	c := testConfig(t)
	c.Prefix = "books/"
	s := &fakeStore{data: string(bytes.Repeat([]byte("x"), 2*1024*1024)), etag: `"v1"`}
	m := newManager(t, c, s)
	if _, err := m.Add("outside.pdf"); err == nil {
		t.Fatal("prefix escaped")
	}
	j, err := m.Add("books/big.pdf")
	if err != nil {
		t.Fatal(err)
	}
	awaitStatus(t, m, j.ID, "failed")
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.offsets) != 0 {
		t.Fatal("oversize file downloaded")
	}
}
