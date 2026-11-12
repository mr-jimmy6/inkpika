package qingstor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mr-jimmy6/inkpika/internal/config"
)

// Independent expected value from the official SDK's Apache-2.0 Chinese key test.
func TestOfficialChineseSignature(t *testing.T) {
	r, _ := http.NewRequest("GET", "https://zone.qingstor.com/bucket-name/中文", nil)
	r.Header.Set("Date", "Mon, 01 Jan 0001 00:00:00 GMT")
	sign(r, "ENV_ACCESS_KEY_ID", "ENV_SECRET_ACCESS_KEY")
	want := "QS ENV_ACCESS_KEY_ID:XsTXX50kzqBf92zLG1aIUIJmZ0hqIHoaHgkumwnV3fs="
	if got := r.Header.Get("Authorization"); got != want {
		t.Fatalf("signature=%s want=%s", got, want)
	}
}

func TestListingAndSpecialObjectKeys(t *testing.T) {
	key := "中文/书名 +?&#%=~.azw3"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "QS test:") {
			t.Error("missing signature")
		}
		if r.URL.Path == "/books/" {
			if r.URL.Query().Get("prefix") != "中文/" || r.URL.Query().Get("marker") != "中文/a +.mobi" {
				t.Error("query encoding")
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"keys":[{"key":"中文/书.mobi","size":4}],"common_prefixes":["中文/子目录/"],"has_more":true,"next_marker":"中文/书.mobi"}`)
		} else {
			if r.URL.Path != "/books/"+key {
				t.Errorf("path=%q", r.URL.Path)
			}
			if r.URL.RawQuery != "" {
				t.Error("key escaped into query")
			}
			if r.Header.Get("Range") != "bytes=2-" || r.Header.Get("If-Match") != "\"v1\"" {
				t.Error("missing resume precondition")
			}
			w.Header().Set("Content-Range", "bytes 2-3/4")
			w.WriteHeader(206)
			fmt.Fprint(w, "ok")
		}
	}))
	defer srv.Close()
	c, err := New(config.Config{Endpoint: srv.URL, Bucket: "books", AccessKeyID: "test", SecretAccessKey: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	list, err := c.List(context.Background(), "中文/", "中文/a +.mobi")
	if err != nil {
		t.Fatal(err)
	}
	if !list.HasMore || len(list.Keys) != 1 || len(list.Folders) != 1 {
		t.Fatal(list)
	}
	r, err := c.Get(context.Background(), key, 2, "\"v1\"")
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
}

func TestRedirectCannotLeakCredentials(t *testing.T) {
	hit := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer srv.Close()
	c, err := New(config.Config{Endpoint: srv.URL, Bucket: "books", AccessKeyID: "test", SecretAccessKey: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.List(context.Background(), "", "")
	if err == nil || hit {
		t.Fatal("redirect must be rejected")
	}
}

func TestUntrustedTLSRejected(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{}`) }))
	defer srv.Close()
	c, err := New(config.Config{Endpoint: srv.URL, Bucket: "books", AccessKeyID: "test", SecretAccessKey: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.List(context.Background(), "", ""); err == nil {
		t.Fatal("untrusted TLS accepted")
	}
}
