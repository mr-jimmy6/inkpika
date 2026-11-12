package syncer

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mr-jimmy6/inkpika/internal/config"
	"github.com/mr-jimmy6/inkpika/internal/download"
	"github.com/mr-jimmy6/inkpika/internal/storage"
)

type fakeObject struct {
	data, etag, version, md5 string
	modified                 int64
}
type fakeStore struct {
	mu              sync.Mutex
	objects         map[string]fakeObject
	gets            []string
	listErr         bool
	repeat          bool
	partial         bool
	changeDuringGet bool
	block           chan struct{}
}

func (s *fakeStore) List(ctx context.Context, prefix, marker string) (storage.Listing, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listErr {
		return storage.Listing{}, errors.New("offline")
	}
	entries := map[string]bool{}
	for k := range s.objects {
		if strings.HasPrefix(k, prefix) {
			tail := strings.TrimPrefix(k, prefix)
			if i := strings.Index(tail, "/"); i >= 0 {
				entries[prefix+tail[:i+1]] = true
			} else {
				entries[k] = false
			}
		}
	}
	keys := []string{}
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	start, _ := strconv.Atoi(marker)
	end := start + 2
	if end > len(keys) {
		end = len(keys)
	}
	out := storage.Listing{}
	for _, k := range keys[start:end] {
		if entries[k] {
			out.Folders = append(out.Folders, k)
		} else {
			o := s.objects[k]
			out.Keys = append(out.Keys, storage.Object{Key: k, Size: int64(len(o.data)), ETag: o.etag, Modified: o.modified})
		}
	}
	if end < len(keys) {
		out.HasMore = true
		out.NextMarker = strconv.Itoa(end)
	}
	if s.repeat {
		out.HasMore = true
		out.NextMarker = "0"
	}
	return out, nil
}
func response(o fakeObject) *http.Response {
	h := http.Header{}
	if o.etag != "" {
		h.Set("ETag", o.etag)
	}
	if o.modified > 0 {
		h.Set("Last-Modified", time.Unix(o.modified, 0).UTC().Format(http.TimeFormat))
	}
	h.Set("X-Oss-Version-Id", o.version)
	if o.md5 != "" {
		h.Set("X-Oss-Meta-Md5", o.md5)
	}
	return &http.Response{StatusCode: 200, ContentLength: int64(len(o.data)), Header: h, Body: io.NopCloser(strings.NewReader(o.data))}
}
func (s *fakeStore) Head(ctx context.Context, key string) (*http.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.objects[key]
	if !ok {
		return nil, errors.New("missing")
	}
	return response(o), nil
}
func (s *fakeStore) Get(ctx context.Context, key string, offset int64, tag string) (*http.Response, error) {
	s.mu.Lock()
	o := s.objects[key]
	s.gets = append(s.gets, key)
	partial := s.partial
	block := s.block
	s.block = nil
	if s.changeDuringGet {
		o.version = "changed"
		s.objects[key] = o
		o.version = "old"
	}
	s.mu.Unlock()
	if block != nil {
		close(block)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if tag != "" && tag != o.etag {
		return nil, errors.New("412")
	}
	r := response(o)
	if partial {
		r.Body = io.NopCloser(strings.NewReader(o.data[:1]))
	}
	return r, nil
}
func setup(t *testing.T) (*Engine, *fakeStore, *download.Manager, config.Config) {
	t.Helper()
	dir := t.TempDir()
	c := config.Config{Bucket: "books", Endpoint: "https://example.test", DownloadDir: filepath.Join(dir, "documents"), StateDir: filepath.Join(dir, "state"), MaxDownloadMB: 1}
	s := &fakeStore{objects: map[string]fakeObject{}}
	m, err := download.New(c, s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	e, err := New(c, s, m, filepath.Join(dir, "menu.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Close)
	return e, s, m, c
}
func run(t *testing.T, e *Engine, fetch bool) Snapshot {
	t.Helper()
	if err := e.Start(fetch); err != nil {
		t.Fatal(err)
	}
	return wait(t, e)
}
func wait(t *testing.T, e *Engine) Snapshot {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s := e.Snapshot()
		if !s.Running {
			return s
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("sync timed out")
	return Snapshot{}
}
func put(s *fakeStore, key, data, tag string, modified int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = fakeObject{data: data, etag: tag, modified: modified}
}
func TestRecursivePaginationRemoteCountsAndCompleteCatalogue(t *testing.T) {
	e, s, m, _ := setup(t)
	for i := 0; i < 8; i++ {
		key := fmt.Sprintf("sync/book%d.azw3", i)
		if i == 7 {
			key = "sync/folder/book7.AZW3"
		}
		put(s, key, "book", fmt.Sprint(i), int64(100+i))
	}
	put(s, "sync/no.epub", "ignored", "e", 1000)
	put(s, "outside.azw3", "ignored", "o", 2000)
	if err := os.WriteFile(m.Target("outside.azw3"), []byte("local"), 0600); err != nil {
		t.Fatal(err)
	}
	snap := run(t, e, false)
	if snap.Total != 8 || snap.Downloaded != 0 || snap.Pending != 8 || len(snap.Unfinished) != 8 || len(snap.Books) != 8 {
		t.Fatalf("%+v", snap)
	}
	if snap.Unfinished[0].Key != "sync/folder/book7.AZW3" {
		t.Fatal(snap.Unfinished)
	}
	for i := 1; i < 8; i++ {
		if snap.Unfinished[i].Modified >= snap.Unfinished[i-1].Modified {
			t.Fatal("sort")
		}
	}
	snap = run(t, e, true)
	if snap.Downloaded != 8 || snap.Pending != 0 || snap.Fetched != 8 {
		t.Fatalf("%+v", snap)
	}
	if len(snap.Books) != 8 || snap.Books[0].Status != "downloaded" || snap.Books[7].Status != "downloaded" {
		t.Fatal("downloaded books disappeared from the catalogue", snap)
	}
	snap.Books[0].Status = "changed-by-caller"
	if e.Snapshot().Books[0].Status != "downloaded" {
		t.Fatal("snapshot aliases engine state")
	}
	snap = run(t, e, true)
	if snap.Fetched != 0 || snap.Skipped != 8 || len(s.gets) != 8 {
		t.Fatalf("%+v gets=%v", snap, s.gets)
	}
	s.mu.Lock()
	delete(s.objects, "sync/book0.azw3")
	s.mu.Unlock()
	snap = run(t, e, false)
	if snap.Total != 7 || snap.Downloaded != 7 || !m.IsDownloaded("sync/book0.azw3") {
		t.Fatal("remote delete must retain local book", snap)
	}
}
func TestMetadataUpdateLocalDamageAndRestart(t *testing.T) {
	e, s, m, c := setup(t)
	key := "sync/book.azw3"
	put(s, key, "first", "opaque", 100)
	if snap := run(t, e, true); snap.Fetched != 1 {
		t.Fatal(snap)
	}
	// Version-only metadata update must be detected even with identical ETag/time/size.
	s.mu.Lock()
	o := s.objects[key]
	o.version = "v2"
	o.data = "later"
	s.objects[key] = o
	s.mu.Unlock()
	snap := run(t, e, false)
	if snap.Pending != 1 || snap.Downloaded != 0 || snap.Books[0].Status != "update" {
		t.Fatal(snap)
	}
	snap = run(t, e, true)
	if snap.Fetched != 1 {
		t.Fatal(snap)
	}
	b, _ := os.ReadFile(m.Target(key))
	if string(b) != "later" {
		t.Fatal(string(b))
	}
	os.WriteFile(m.Target(key), []byte("wrong"), 0600)
	if snap = run(t, e, true); snap.Fetched != 1 {
		t.Fatal("same-size corruption missed", snap)
	}
	os.Remove(m.Target(key))
	if snap = run(t, e, true); snap.Fetched != 1 {
		t.Fatal("deletion missed", snap)
	}
	e.Close()
	e2, err := New(c, s, m, "")
	if err != nil {
		t.Fatal(err)
	}
	defer e2.Close()
	if e2.Snapshot().Fresh {
		t.Fatal("restart should mark cached status")
	}
	if snap = run(t, e2, true); snap.Fetched != 0 || snap.Downloaded != 1 {
		t.Fatal(snap)
	}
}
func TestFailuresPreserveOldCopyAndRemainPending(t *testing.T) {
	e, s, m, _ := setup(t)
	key := "sync/book.pdf"
	put(s, key, "old book", "v1", 100)
	run(t, e, true)
	put(s, key, "new book", "v2", 200)
	s.partial = true
	snap := run(t, e, true)
	if snap.Downloaded != 0 || snap.Pending != 1 || snap.Phase != "error" || snap.Books[0].Status != "failed" {
		t.Fatal(snap)
	}
	b, _ := os.ReadFile(m.Target(key))
	if string(b) != "old book" {
		t.Fatal("old copy damaged")
	}
	s.partial = false
	s.changeDuringGet = true
	snap = run(t, e, true)
	if snap.Pending != 1 {
		t.Fatal("changed metadata accepted", snap)
	}
	b, _ = os.ReadFile(m.Target(key))
	if string(b) != "old book" {
		t.Fatal("old copy replaced during race")
	}
	s.changeDuringGet = false
	if snap = run(t, e, true); snap.Pending != 0 {
		t.Fatal(snap)
	}
}
func TestOpaqueETagAndExplicitMD5Adoption(t *testing.T) {
	e, s, m, _ := setup(t)
	key := "sync/book.txt"
	data := "hello"
	sum := md5.Sum([]byte(data))
	md := hex.EncodeToString(sum[:])
	put(s, key, data, md, 100)
	os.WriteFile(m.Target(key), []byte(data), 0600)
	if snap := run(t, e, false); snap.Downloaded != 0 {
		t.Fatal("ETag incorrectly treated as MD5", snap)
	}
	s.mu.Lock()
	o := s.objects[key]
	o.md5 = md
	s.objects[key] = o
	s.mu.Unlock()
	if snap := run(t, e, true); snap.Downloaded != 1 || snap.Fetched != 0 || len(s.gets) != 0 {
		t.Fatal(snap)
	}
	put(s, "sync/multipart.pdf", "content", `"abc-4"`, 100)
	if snap := run(t, e, true); snap.Downloaded != 2 {
		t.Fatal("multipart ETag rejected", snap)
	}
}
func TestUnverifiedLocalCopyNeverOverwritten(t *testing.T) {
	e, s, m, _ := setup(t)
	key := "sync/book.azw3"
	put(s, key, "new", "v1", 100)
	os.WriteFile(m.Target(key), []byte("old"), 0600)
	if snap := run(t, e, true); snap.Pending != 1 {
		t.Fatal(snap)
	}
	b, _ := os.ReadFile(m.Target(key))
	if string(b) != "old" {
		t.Fatal("untracked copy replaced")
	}
	os.WriteFile(m.Target(key), []byte("new"), 0600)
	if snap := run(t, e, true); snap.Downloaded != 1 {
		t.Fatal("identical copy not adopted", snap)
	}
}
func TestFailedEnumerationCachedStatsAndScope(t *testing.T) {
	e, s, _, _ := setup(t)
	put(s, "sync/book.txt", "abc", "v1", 100)
	run(t, e, true)
	s.listErr = true
	snap := run(t, e, false)
	if snap.Total != 1 || snap.Fresh || snap.Phase != "error" {
		t.Fatal(snap)
	}
	s.listErr = false
	s.repeat = true
	snap = run(t, e, false)
	if snap.Fresh || snap.Phase != "error" {
		t.Fatal(snap)
	}
	s.repeat = false
	e.cfg.Prefix = "private/"
	snap = run(t, e, true)
	if snap.Phase != "error" || len(s.gets) != 1 {
		t.Fatal(snap)
	}
}
func TestCancellationDuplicateStartAndTransferSerialization(t *testing.T) {
	e, s, m, _ := setup(t)
	put(s, "sync/book.txt", "abc", "v1", 100)
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		m.WithTransfer(context.Background(), func() error { close(entered); <-release; return nil })
		close(finished)
	}()
	<-entered
	if err := e.Start(true); err != nil {
		t.Fatal(err)
	}
	if err := e.Start(true); err == nil {
		t.Fatal("duplicate accepted")
	}
	e.Cancel()
	snap := wait(t, e)
	if snap.Phase != "cancelled" || len(s.gets) != 0 {
		t.Fatal(snap)
	}
	close(release)
	<-finished
	s.block = make(chan struct{})
	started := s.block
	e.Start(true)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("GET never started")
	}
	if snap = e.Snapshot(); snap.Books[0].Status != "downloading" || !snap.Running {
		t.Fatal("active book is not marked DL", snap)
	}
	if err := e.Refresh(); err != nil {
		t.Fatal("refresh must not start a second scan during a transfer", err)
	}
	e.Cancel()
	snap = wait(t, e)
	if snap.Downloaded != 0 || snap.Pending != 1 || snap.Books[0].Status == "downloading" || snap.Unfinished[0].Status == "downloading" {
		t.Fatal(snap)
	}
	if snap = run(t, e, true); snap.Downloaded != 1 {
		t.Fatal(snap)
	}
}
func TestNoValidatorsRedownloadAndWrongMD5(t *testing.T) {
	e, s, _, _ := setup(t)
	put(s, "sync/a.txt", "abc", "", 0)
	run(t, e, true)
	if snap := run(t, e, true); snap.Fetched != 1 {
		t.Fatal("missing validators trusted", snap)
	}
	put(s, "sync/b.txt", "abc", "tag", 100)
	s.mu.Lock()
	o := s.objects["sync/b.txt"]
	o.md5 = strings.Repeat("0", 32)
	s.objects["sync/b.txt"] = o
	s.mu.Unlock()
	if snap := run(t, e, true); snap.Pending != 1 {
		t.Fatal("bad MD5 accepted", snap)
	}
}
func TestSymlinkRejectedAndMenuDataNotExecutable(t *testing.T) {
	e, s, m, _ := setup(t)
	key := "sync/$(touch evil);`id`.azw3"
	put(s, key, "data", "v1", 100)
	victim := filepath.Join(t.TempDir(), "victim")
	os.WriteFile(victim, []byte("safe"), 0600)
	os.Symlink(victim, m.Target(key))
	snap := run(t, e, true)
	if snap.Pending != 1 {
		t.Fatal(snap)
	}
	b, _ := os.ReadFile(victim)
	if string(b) != "safe" {
		t.Fatal("symlink followed")
	}
	b, _ = os.ReadFile(e.menu)
	var menu struct{ Items []menuItem }
	if err := json.Unmarshal(b, &menu); err != nil {
		t.Fatal(err)
	}
	var visit func([]menuItem)
	visit = func(items []menuItem) {
		for _, item := range items {
			if item.Action != "" && item.Action != ":" && item.Action != "./sync.sh" && item.Action != "./start.sh" && item.Action != "./stop.sh" && item.Action != "./service.sh" && item.Action != "./menu.sh" && item.Action != "./transfer-start.sh" && item.Action != "./transfer-stop.sh" && item.Action != "./transfer-service.sh" && item.Action != "./settings-start.sh" && item.Action != "./settings-stop.sh" && item.Action != "./settings-service.sh" {
				t.Fatal("remote action injection", item)
			}
			for _, r := range item.Internal {
				if r > 127 {
					t.Fatal("KUAL internal text must be ASCII; byte/character lengths differ")
				}
			}
			if strings.ContainsAny(item.Params, "$;`") {
				t.Fatal("remote params injection")
			}
			visit(item.Items)
		}
	}
	visit(menu.Items)
}

func TestConfiguredFormatsFilterSync(t *testing.T) {
	e, store, _, _ := setup(t)
	e.cfg.EbookFormats = []string{"epub"}
	put(store, "sync/allowed.epub", "book", "v1", 100)
	put(store, "sync/excluded.azw3", "book", "v1", 100)
	snap := run(t, e, false)
	if snap.Total != 1 || len(snap.Books) != 1 || snap.Books[0].Key != "sync/allowed.epub" {
		t.Fatalf("sync ignored configured formats: %+v", snap)
	}
}
