package aliyunoss

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mr-jimmy6/inkpika/internal/config"
	"github.com/mr-jimmy6/inkpika/internal/download"
)

// Expected signatures are published by the official OSS Go v2 SDK tests.
func TestOfficialV4Signatures(t *testing.T) {
	for _, tc := range []struct {
		token            string
		stamp            int64
		scope, signature string
	}{
		{"", 1702743657, "20231216", "e21d18daa82167720f9b1047ae7e7f1ce7cb77a31e8203a7d5f4624fa0284afe"},
		{"token", 1702784856, "20231217", "b94a3f999cf85bcdc00d332fbd3734ba03e48382c36fa4d5af5df817395bd9ea"},
	} {
		t.Run(tc.scope, func(t *testing.T) {
			r, _ := http.NewRequest("PUT", "https://bucket.oss-cn-hangzhou.aliyuncs.com", nil)
			r.Header.Set("x-oss-head1", "value")
			r.Header.Set("abc", "value")
			r.Header.Set("ZAbc", "value")
			r.Header.Set("XYZ", "value")
			r.Header.Set("content-type", "text/plain")
			r.URL.RawQuery = url.Values{"param1": {"value1"}, "+param1": {"value3"}, "|param1": {"value4"}, "+param2": {""}, "|param2": {""}, "param2": {""}}.Encode()
			sign(r, "bucket", "1234+-/123/1.txt", "cn-hangzhou", "ak", "sk", tc.token, time.Unix(tc.stamp, 0))
			want := "OSS4-HMAC-SHA256 Credential=ak/" + tc.scope + "/cn-hangzhou/oss/aliyun_v4_request,Signature=" + tc.signature
			if got := r.Header.Get("Authorization"); got != want {
				t.Fatalf("got %s\nwant %s", got, want)
			}
			if r.Header.Get("X-Oss-Security-Token") != tc.token {
				t.Fatal("STS header missing")
			}
		})
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Preserve the real OSS URL and Host until RoundTrip; only redirect the test connection.
func fixture(t *testing.T, handler http.HandlerFunc) (*Client, config.Config) {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)
	cfg := config.Config{Provider: config.AliyunOSS, Region: "cn-hangzhou", Endpoint: "https://oss-cn-hangzhou.aliyuncs.com", Bucket: "books", AccessKeyID: "ak", SecretAccessKey: "sk", SecurityToken: "sts-test"}
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	target, _ := url.Parse(srv.URL)
	c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "books.oss-cn-hangzhou.aliyuncs.com" {
			t.Errorf("wrong virtual host: %s", r.URL.Host)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "OSS4-HMAC-SHA256 Credential=ak/") || !strings.Contains(r.Header.Get("Authorization"), "/cn-hangzhou/oss/aliyun_v4_request,") {
			t.Error("missing scoped V4 auth")
		}
		if r.Header.Get("X-Oss-Security-Token") != "sts-test" {
			t.Error("missing STS token")
		}
		copy := r.Clone(r.Context())
		u := *r.URL
		u.Scheme = target.Scheme
		u.Host = target.Host
		copy.URL = &u
		return srv.Client().Transport.RoundTrip(copy)
	})
	return c, cfg
}

func TestListPaginationAndSpecialKeys(t *testing.T) {
	key := "书架/中文 +#%?&=.azw3"
	folder := "书架/子目录 +/"
	marker := "next+/%2F=="
	c, _ := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			q := r.URL.Query()
			if q.Get("list-type") != "2" || q.Get("max-keys") != "6" || q.Get("encoding-type") != "url" || q.Get("prefix") != "书架/" {
				t.Errorf("bad query: %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/xml")
			if q.Get("continuation-token") == "" {
				fmt.Fprintf(w, `<ListBucketResult><EncodingType>url</EncodingType><IsTruncated>true</IsTruncated><NextContinuationToken>%s</NextContinuationToken><Contents><Key>%s</Key><Size>10</Size><ETag>&quot;v1&quot;</ETag><LastModified>2026-11-06T00:00:00.000Z</LastModified></Contents><CommonPrefixes><Prefix>%s</Prefix></CommonPrefixes></ListBucketResult>`, marker, escape(key), escape(folder))
			} else {
				if q.Get("continuation-token") != marker {
					t.Error("opaque marker changed")
				}
				fmt.Fprint(w, `<ListBucketResult><IsTruncated>false</IsTruncated></ListBucketResult>`)
			}
			return
		}
		if r.URL.Path != "/"+key || r.URL.RawQuery != "" {
			t.Errorf("key changed: %s", r.URL)
		}
		w.Header().Set("ETag", `"v1"`)
		if r.Method == "HEAD" {
			w.Header().Set("Content-Length", "10")
			return
		}
		if r.Header.Get("Range") != "bytes=3-" || r.Header.Get("If-Match") != `"v1"` {
			t.Error("missing resume preconditions")
		}
		w.Header().Set("Content-Range", "bytes 3-9/10")
		w.WriteHeader(206)
		fmt.Fprint(w, "3456789")
	})
	list, err := c.List(context.Background(), "书架/", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Keys) != 1 || list.Keys[0].Key != key || list.Keys[0].Modified == 0 || len(list.Folders) != 1 || list.Folders[0] != folder || list.NextMarker != marker {
		t.Fatalf("bad listing: %+v", list)
	}
	last, err := c.List(context.Background(), "书架/", list.NextMarker)
	if err != nil || last.HasMore || last.NextMarker != "" {
		t.Fatalf("pagination: %+v %v", last, err)
	}
	head, err := c.Head(context.Background(), key)
	if err != nil || head.ContentLength != 10 {
		t.Fatalf("head: %v %v", head, err)
	}
	resp, err := c.Get(context.Background(), key, 3, `"v1"`)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if string(data) != "3456789" {
		t.Fatal(string(data))
	}
}

func TestOSSInterruptedDownloadResumesThroughManager(t *testing.T) {
	var gets atomic.Int32
	c, cfg := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"version1"`)
		if r.Method == "HEAD" {
			w.Header().Set("Content-Length", "10")
			return
		}
		if gets.Add(1) == 1 {
			w.Header().Set("Content-Length", "10")
			fmt.Fprint(w, "012")
			return
		}
		if r.Header.Get("Range") != "bytes=3-" || r.Header.Get("If-Match") != `"version1"` {
			t.Error("resume headers lost")
		}
		w.Header().Set("Content-Length", "7")
		w.Header().Set("Content-Range", "bytes 3-9/10")
		w.WriteHeader(206)
		fmt.Fprint(w, "3456789")
	})
	dir := t.TempDir()
	cfg.StateDir = filepath.Join(dir, "state")
	cfg.DownloadDir = filepath.Join(dir, "documents")
	cfg.MaxDownloadMB = 1
	m, err := download.New(cfg, c)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	j, err := m.Add("中文/测试.azw3")
	if err != nil {
		t.Fatal(err)
	}
	await := func(want string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			for _, job := range m.Snapshot() {
				if job.ID == j.ID && job.Status == want {
					return
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("want %s, got %+v", want, m.Snapshot())
	}
	await("failed")
	if _, err = os.Stat(filepath.Join(cfg.DownloadDir, j.Name)); !os.IsNotExist(err) {
		t.Fatal("partial visible")
	}
	if err = m.Action(j.ID, "retry"); err != nil {
		t.Fatal(err)
	}
	await("completed")
	data, err := os.ReadFile(filepath.Join(cfg.DownloadDir, j.Name))
	if err != nil || string(data) != "0123456789" {
		t.Fatalf("bad resumed data: %q %v", data, err)
	}
}

func TestErrorsAndBadPagination(t *testing.T) {
	for _, tc := range []struct {
		status         int
		body, contains string
	}{
		{403, `<Error><Code>SecurityTokenExpired</Code><Message>DO-NOT-EXPOSE</Message></Error>`, "STS"},
		{403, `<Error><Code>AccessDenied</Code></Error>`, "oss:ListObjects"},
		{403, `<Error><Code>InvalidObjectState</Code></Error>`, "restore"},
		{200, `<ListBucketResult><IsTruncated>true</IsTruncated></ListBucketResult>`, "pagination"},
		{200, `<ListBucketResult><EncodingType>url</EncodingType><Contents><Key>%GG</Key></Contents></ListBucketResult>`, "encoding"},
		{302, ``, "HTTP 302"},
	} {
		t.Run(strconv.Itoa(tc.status)+tc.contains, func(t *testing.T) {
			c, _ := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "https://invalid.example")
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			})
			_, err := c.List(context.Background(), "", "")
			if err == nil || !strings.Contains(err.Error(), tc.contains) || strings.Contains(err.Error(), "DO-NOT-EXPOSE") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
