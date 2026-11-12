package s3

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

// Published S3 GET Object example: sig-v4-header-based-auth.html (2013-05-24).
func TestAWSPublishedSignature(t *testing.T) {
	r, _ := http.NewRequest("GET", "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	r.Header.Set("Range", "bytes=0-9")
	sign(r, "us-east-1", "AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "", time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC))
	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if r.Header.Get("Authorization") != want {
		t.Fatalf("AWS reference signature mismatch: %s", r.Header.Get("Authorization"))
	}
}

func TestPathStylePreservesObjectNamesAndSignedHeaders(t *testing.T) {
	keys := []string{"书架/中文 +#%?&=~.azw3", "a//b/.././book.azw3", "a/%2F.azw3", "/leading.azw3"}
	for _, key := range keys {
		c, _ := fixture(t, func(w http.ResponseWriter, r *http.Request) {})
		c.cfg.Endpoint = "https://nas.example:9443"
		c.cfg.S3Addressing = "path"
		c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host != "nas.example:9443" || r.URL.Path != "/books/"+key || r.URL.RawQuery != "" {
				t.Fatalf("object identity changed: %s", r.URL)
			}
			if strings.Contains(r.URL.EscapedPath(), "+") || !strings.Contains(r.Header.Get("Authorization"), "host;if-match;range;x-amz-content-sha256;x-amz-date;x-amz-security-token") {
				t.Fatal("encoding or signed preconditions lost")
			}
			return &http.Response{StatusCode: 206, Header: make(http.Header), Body: http.NoBody}, nil
		})
		r, err := c.Get(context.Background(), key, 2, `"opaque-multipart-2"`)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
	}
	if got := canonicalQuery(url.Values{"a": {"+", " ", ""}, "a-": {"last"}, "中文": {"a/b"}, "~": {"%"}}); got != "%E4%B8%AD%E6%96%87=a%2Fb&a=&a=%20&a=%2B&a-=last&~=%25" {
		t.Fatal("canonical query must sort encoded keys/values and preserve empty values", got)
	}
}

func TestRedirectNeverReceivesSignedRequest(t *testing.T) {
	for _, sameHost := range []bool{false, true} {
		var forwarded atomic.Int32
		target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded.Add(1) }))
		defer target.Close()
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/stolen" {
				forwarded.Add(1)
				return
			}
			location := target.URL
			if sameHost {
				location = "/stolen"
			}
			http.Redirect(w, r, location, 307)
		}))
		defer server.Close()
		c, err := New(config.Config{Provider: config.S3, Endpoint: server.URL, Region: "us-east-1", Bucket: "books", S3Addressing: "path", AccessKeyID: "fixture-id", SecretAccessKey: "fixture-secret"})
		if err != nil {
			t.Fatal(err)
		}
		c.http.Transport = server.Client().Transport
		if _, err := c.Get(context.Background(), "book.azw3", 0, ""); err == nil || forwarded.Load() != 0 {
			t.Fatal("redirect received signed request")
		}
	}
}

func TestErrorBodiesClosedAndNeverEchoed(t *testing.T) {
	for _, status := range []int{301, 307, 308, 403, 404, 412, 416, 500} {
		c, _ := fixture(t, func(w http.ResponseWriter, r *http.Request) {})
		body := &trackedBody{Reader: strings.NewReader(`<Error><Code>secret-echo</Code><Message>secret-echo</Message></Error>`)}
		c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Header: http.Header{"Location": {"https://elsewhere.invalid"}}, Body: body}, nil
		})
		_, err := c.Get(context.Background(), "book.azw3", 0, "")
		if err == nil || strings.Contains(err.Error(), "secret-echo") || !body.closed {
			t.Fatal(status, err, body.closed)
		}
	}
}

type trackedBody struct {
	io.Reader
	closed bool
}

func (b *trackedBody) Close() error { b.closed = true; return nil }

func TestInvalidPaginationAndXML(t *testing.T) {
	for _, body := range []string{
		`<ListBucketResult><IsTruncated>true</IsTruncated><NextContinuationToken>same</NextContinuationToken></ListBucketResult>`,
		`<ListBucketResult><Contents><Key>book</Key><Size>-1</Size></Contents></ListBucketResult>`,
		`<ListBucketResult><EncodingType>url</EncodingType><CommonPrefixes><Prefix>%GG</Prefix></CommonPrefixes></ListBucketResult>`,
		`<ListBucketResult><EncodingType>unknown</EncodingType></ListBucketResult>`,
		`<Error><Code>not-a-list</Code></Error>`,
		`<ListBucketResult>`,
	} {
		c, _ := fixture(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) })
		if _, err := c.List(context.Background(), "", "same"); err == nil {
			t.Fatal("accepted malformed listing")
		}
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Preserve the real S3 URL and Host until RoundTrip; only redirect the test connection.
func fixture(t *testing.T, handler http.HandlerFunc) (*Client, config.Config) {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)
	cfg := config.Config{Provider: config.S3, S3Preset: "aws", S3Addressing: "virtual", Region: "us-east-1", Endpoint: "https://s3.us-east-1.amazonaws.com", Bucket: "books", AccessKeyID: "ak", SecretAccessKey: "sk", SecurityToken: "sts-test"}
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	target, _ := url.Parse(srv.URL)
	c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "books.s3.us-east-1.amazonaws.com" {
			t.Errorf("wrong virtual host: %s", r.URL.Host)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=ak/") || !strings.Contains(r.Header.Get("Authorization"), "/us-east-1/s3/aws4_request,") {
			t.Error("missing scoped V4 auth")
		}
		if r.Header.Get("X-Amz-Security-Token") != "sts-test" {
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

func TestS3InterruptedDownloadResumesThroughManager(t *testing.T) {
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
		{403, `<Error><Code>ExpiredToken</Code><Message>DO-NOT-EXPOSE</Message></Error>`, "Temporary credentials"},
		{403, `<Error><Code>AccessDenied</Code></Error>`, "s3:ListBucket"},
		{403, `<Error><Code>InvalidObjectState</Code></Error>`, "Restore"},
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
