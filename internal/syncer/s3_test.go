package syncer

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
)

// Reuse the sync fixture with S3 headers to exercise actual reconciliation,
// including versions that change without a change in ETag, size or timestamp.
type s3MetadataStore struct{ *fakeStore }

func s3Headers(r *http.Response) *http.Response {
	r.Header.Set("X-Amz-Version-Id", r.Header.Get("X-Oss-Version-Id"))
	r.Header.Del("X-Oss-Version-Id")
	if value := r.Header.Get("X-Oss-Meta-Md5"); value != "" {
		r.Header.Set("X-Amz-Meta-Md5", value)
		r.Header.Del("X-Oss-Meta-Md5")
	}
	return r
}
func (s s3MetadataStore) Head(ctx context.Context, key string) (*http.Response, error) {
	r, err := s.fakeStore.Head(ctx, key)
	if err != nil {
		return nil, err
	}
	return s3Headers(r), nil
}
func (s s3MetadataStore) Get(ctx context.Context, key string, offset int64, etag string) (*http.Response, error) {
	r, err := s.fakeStore.Get(ctx, key, offset, etag)
	if err != nil {
		return nil, err
	}
	return s3Headers(r), nil
}
func TestS3VersionChangesTriggerVerifiedSyncReplacement(t *testing.T) {
	e, s, m, _ := setup(t)
	e.store = s3MetadataStore{s}
	key := "sync/book.azw3"
	put(s, key, "first", "opaque-multipart-2", 100)
	if snap := run(t, e, true); snap.Fetched != 1 {
		t.Fatal(snap)
	}
	s.mu.Lock()
	o := s.objects[key]
	o.version = "s3-version-2"
	o.data = "later"
	s.objects[key] = o
	s.mu.Unlock()
	if snap := run(t, e, false); snap.Pending != 1 {
		t.Fatal("S3 version-only change was missed", snap)
	}
	if snap := run(t, e, true); snap.Fetched != 1 {
		t.Fatal(snap)
	}
	data, err := os.ReadFile(m.Target(key))
	if err != nil || string(data) != "later" {
		t.Fatal("S3 update not installed", err)
	}
	s.mu.Lock()
	s.changeDuringGet = true
	s.mu.Unlock()
	os.WriteFile(m.Target(key), []byte("wrong"), 0600)
	if snap := run(t, e, true); snap.Pending != 1 || snap.Phase != "error" {
		t.Fatal("S3 changed during download accepted", snap)
	}
}
func TestS3MetadataAndOpaqueETags(t *testing.T) {
	r := &http.Response{StatusCode: 200, ContentLength: 4, Header: make(http.Header)}
	r.Header.Set("ETag", `"not-an-md5-2"`)
	r.Header.Set("X-Amz-Version-Id", "null")
	r.Header.Set("X-Amz-Meta-Edition", "2")
	m, err := metadataOf(r)
	if err != nil || m.MD5 != "" || m.Version != "" || !strings.Contains(m.Attributes, `"x-amz-meta-edition":"2"`) {
		t.Fatal("S3 metadata lost or ETag treated as a hash", m, err)
	}
	r.Header.Set("X-Amz-Meta-Md5", "098f6bcd4621d373cade4e832627b4f6")
	m, err = metadataOf(r)
	if err != nil || m.MD5 != "098f6bcd4621d373cade4e832627b4f6" {
		t.Fatal(err)
	}
	r.Header.Set("Content-MD5", "ffffffffffffffffffffffffffffffff")
	if _, err := metadataOf(r); err == nil {
		t.Fatal("conflicting explicit checksums accepted")
	}
}
