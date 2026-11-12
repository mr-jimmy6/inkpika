package azureblob

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mr-jimmy6/inkpika/internal/config"
)

func testClient(t *testing.T) *Client {
	t.Helper()
	c, err := New(config.Config{Provider: config.AzureBlob, Bucket: "books", Endpoint: "https://myaccount.blob.core.windows.net", AccessKeyID: "myaccount", SecretAccessKey: base64.StdEncoding.EncodeToString(make([]byte, 64))})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type trackedBody struct {
	io.Reader
	closed bool
}

func (b *trackedBody) Close() error { b.closed = true; return nil }

func TestCanonicalPublishedExampleAndSignature(t *testing.T) {
	// Canonical resource/header example from Microsoft's Shared Key reference.
	r, _ := http.NewRequest("GET", "https://myaccount.blob.core.windows.net/mycontainer?restype=container&comp=metadata&timeout=20", nil)
	r.Header.Set("X-Ms-Date", "Fri, 26 Jun 2015 23:39:12 GMT")
	r.Header.Set("X-Ms-Version", "2015-02-21")
	want := "GET" + strings.Repeat("\n", 12) + "x-ms-date:Fri, 26 Jun 2015 23:39:12 GMT\nx-ms-version:2015-02-21\n/myaccount/mycontainer\ncomp:metadata\nrestype:container\ntimeout:20"
	if got := canonical(r, "myaccount"); got != want {
		t.Fatalf("canonical mismatch: %q", got)
	}
	// Independent fixed vector, calculated with Python hmac/hashlib, all-zero test key.
	sign(r, "myaccount", make([]byte, 64), time.Date(2015, 6, 26, 23, 39, 12, 0, time.UTC))
	if got := r.Header.Get("Authorization"); got != "SharedKey myaccount:gJN5BORAXloGSixGD61DCXUbhRcOgckyYPlKsvLFU9c=" { // gitleaks:allow -- fixed HMAC of the all-zero test key, not an account credential.
		t.Fatal(got)
	}
	r, _ = http.NewRequest("GET", "https://example.test/books/a%20%2B%25?z=b&z=a%2Bc&Z=a%20b", nil)
	r.Header.Set("If-Match", `"opaque"`)
	r.Header.Set("Range", "bytes=3-")
	r.Header.Set("X-Ms-Meta-Title", "  two   words ")
	got := canonical(r, "myaccount")
	if !strings.Contains(got, "\n\"opaque\"\n\n\nbytes=3-\nx-ms-meta-title:two words\n") || !strings.HasSuffix(got, "/myaccount/books/a%20%2B%25\nz:a b,a+c,b") {
		t.Fatal(got)
	}
}

func TestListingOpaqueMarkerAndEncodedNames(t *testing.T) {
	c := testClient(t)
	marker := "opaque+/%2F=="
	c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query()
		if r.URL.Path != "/books" || q.Get("comp") != "list" || q.Get("restype") != "container" || q.Get("maxresults") != "6" || q.Get("prefix") != "书架/" || q.Get("delimiter") != "/" {
			t.Fatal(r.URL)
		}
		body := `<EnumerationResults><Blobs><Blob><Name Encoded="true">%E4%B9%A6%E6%9E%B6%2F%E4%B8%AD%20%2B%25.azw3</Name><Properties><Content-Length>10</Content-Length><Etag>&quot;v1&quot;</Etag><Last-Modified>Fri, 26 Jun 2015 23:39:12 GMT</Last-Modified></Properties></Blob><BlobPrefix><Name Encoded="true">%E4%B9%A6%E6%9E%B6%2Fsub%2F</Name></BlobPrefix></Blobs><NextMarker>` + marker + `</NextMarker></EnumerationResults>`
		if q.Get("marker") != "" {
			if q.Get("marker") != marker {
				t.Fatal("marker changed")
			}
			body = `<EnumerationResults><Blobs></Blobs><NextMarker/></EnumerationResults>`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	first, err := c.List(context.Background(), "书架/", "")
	if err != nil || len(first.Keys) != 1 || first.Keys[0].Key != "书架/中 +%.azw3" || first.Keys[0].ETag != `"v1"` || first.Keys[0].Modified == 0 || len(first.Folders) != 1 || first.Folders[0] != "书架/sub/" || !first.HasMore || first.NextMarker != marker {
		t.Fatalf("%+v %v", first, err)
	}
	last, err := c.List(context.Background(), "书架/", first.NextMarker)
	if err != nil || last.HasMore {
		t.Fatalf("%+v %v", last, err)
	}
}

func TestHeadGetIdentityAndPreconditions(t *testing.T) {
	for _, key := range []string{"书架/中 +#%?&=.azw3", "a//b/.././book.azw3", "a/%2F.azw3", "/leading.azw3"} {
		c := testClient(t)
		c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/books/"+key || r.URL.RawQuery != "" || strings.Contains(r.URL.EscapedPath(), "+") {
				t.Fatal(r.URL)
			}
			if !strings.HasPrefix(r.Header.Get("Authorization"), "SharedKey myaccount:") || r.Header.Get("X-Ms-Version") != "2021-12-02" {
				t.Fatal("missing auth")
			}
			if r.Method == "HEAD" {
				return &http.Response{StatusCode: 200, ContentLength: 10, Header: http.Header{"Etag": {`"v1"`}}, Body: http.NoBody}, nil
			}
			if r.Method != "GET" || r.Header.Get("Range") != "bytes=3-" || r.Header.Get("If-Match") != `"v1"` {
				t.Fatal("read/precondition changed")
			}
			return &http.Response{StatusCode: 206, ContentLength: 7, Header: http.Header{"Content-Range": {"bytes 3-9/10"}}, Body: io.NopCloser(strings.NewReader("3456789"))}, nil
		})
		head, err := c.Head(context.Background(), key)
		if err != nil || head.ContentLength != 10 || head.Body != http.NoBody {
			t.Fatal(head, err)
		}
		r, err := c.Get(context.Background(), key, 3, `"v1"`)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(r.Body)
		r.Body.Close()
		if string(b) != "3456789" {
			t.Fatal(string(b))
		}
		if _, err := c.Get(context.Background(), key, -1, ""); err == nil {
			t.Fatal("negative offset accepted")
		}
	}
}

func TestMalformedListingsAndBodyClosure(t *testing.T) {
	for _, body := range []string{
		`<EnumerationResults><NextMarker>same</NextMarker></EnumerationResults>`,
		`<EnumerationResults><Blobs><Blob><Name>book</Name><Properties/></Blob></Blobs></EnumerationResults>`,
		`<EnumerationResults><Blobs><Blob><Name>book</Name><Properties><Content-Length>-1</Content-Length></Properties></Blob></Blobs></EnumerationResults>`,
		`<EnumerationResults><Blobs><BlobPrefix><Name Encoded="true">%GG</Name></BlobPrefix></Blobs></EnumerationResults>`,
		`<Error><Message>private</Message></Error>`, `<EnumerationResults>`,
	} {
		c := testClient(t)
		b := &trackedBody{Reader: strings.NewReader(body)}
		c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) { return &http.Response{StatusCode: 200, Body: b}, nil })
		if _, err := c.List(context.Background(), "", "same"); err == nil || !b.closed {
			t.Fatal("accepted malformed listing or leaked body", err)
		}
	}
	for _, status := range []int{301, 302, 307, 308, 401, 403, 404, 412, 416, 500} {
		c := testClient(t)
		b := &trackedBody{Reader: strings.NewReader("private-signature")}
		c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) { return &http.Response{StatusCode: status, Body: b}, nil })
		if _, err := c.Get(context.Background(), "book", 0, ""); err == nil || strings.Contains(err.Error(), "private") || !b.closed {
			t.Fatal(status, err)
		}
	}
}

func TestRedirectNeverReceivesCredentials(t *testing.T) {
	for _, sameHost := range []bool{true, false} {
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
		c := testClient(t)
		c.cfg.Endpoint = server.URL
		c.http.Transport = server.Client().Transport
		if _, err := c.Get(context.Background(), "book", 0, ""); err == nil || forwarded.Load() != 0 {
			t.Fatal("redirect forwarded auth")
		}
	}
}

func TestBadKeyRejected(t *testing.T) {
	for _, key := range []string{"not-base64", base64.StdEncoding.EncodeToString([]byte("too-short"))} {
		if _, err := New(config.Config{SecretAccessKey: key}); err == nil {
			t.Fatalf("accepted invalid key of length %d", len(key))
		}
	}
}
