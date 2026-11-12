package s3

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/mr-jimmy6/inkpika/internal/config"
)

func TestVendorProtocolsAndAddressing(t *testing.T) {
	for _, tc := range []struct{ preset, region, bucket, host, list string }{
		{"tos", "cn-beijing", "books", "books.tos-s3-cn-beijing.volces.com", "v1"},
		{"cos", "ap-guangzhou", "books-1250000000", "books-1250000000.cos.ap-guangzhou.myqcloud.com", "v1"},
		{"gcs", "auto", "books", "storage.googleapis.com", "v1"},
		{"ibm", "us-south", "books", "s3.us-south.cloud-object-storage.appdomain.cloud", "v2"},
	} {
		t.Run(tc.preset, func(t *testing.T) {
			cfg := config.Config{Provider: config.S3, S3Preset: tc.preset, Region: tc.region, Bucket: tc.bucket, AccessKeyID: "fixture", SecretAccessKey: "secret", Listen: "127.0.0.1:8787", DownloadDir: "/books", StateDir: "/state", MaxDownloadMB: 512}
			if err := cfg.Validate(); err != nil {
				t.Fatal(err)
			}
			c, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			marker := "中 +/%2F=="
			key := "sync/中 +%2F.azw3"
			calls := 0
			c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Host != tc.host || !strings.Contains(r.Header.Get("Authorization"), "/"+tc.region+"/s3/aws4_request,") {
					t.Fatal("incorrect host/signing region", r.URL)
				}
				root := "/"
				if cfg.S3Addressing == "path" {
					root = "/" + tc.bucket + "/"
				}
				if r.URL.Path != root {
					if r.URL.Path != root+key {
						t.Fatal("object identity changed", r.URL)
					}
					if r.Method == "GET" && (r.Header.Get("Range") != "bytes=3-" || r.Header.Get("If-Match") != `"opaque"`) {
						t.Fatal("preconditions lost")
					}
					return &http.Response{StatusCode: 200, ContentLength: 10, Header: http.Header{}, Body: http.NoBody}, nil
				}
				q := r.URL.Query()
				next := "NextContinuationToken"
				value := marker
				queryKey := "continuation-token"
				if tc.list == "v1" {
					next = "NextMarker"
					value = escape(marker)
					queryKey = "marker"
					if q.Has("list-type") || q.Has("continuation-token") {
						t.Fatal("V2 used for V1 backend")
					}
				} else if q.Get("list-type") != "2" {
					t.Fatal("IBM V2 missing")
				}
				body := `<ListBucketResult><EncodingType>url</EncodingType><IsTruncated>true</IsTruncated><` + next + `>` + value + `</` + next + `><Contents><Key>` + escape(key) + `</Key><Size>10</Size></Contents></ListBucketResult>`
				if q.Get(queryKey) != "" {
					if q.Get(queryKey) != marker {
						t.Fatal("marker changed", q)
					}
					body = `<ListBucketResult><IsTruncated>false</IsTruncated></ListBucketResult>`
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			list, err := c.List(context.Background(), "sync/", "")
			if err != nil || list.NextMarker != marker || len(list.Keys) != 1 || list.Keys[0].Key != key {
				t.Fatal(list, err)
			}
			last, err := c.List(context.Background(), "sync/", list.NextMarker)
			if err != nil || last.HasMore {
				t.Fatal(last, err)
			}
			if _, err := c.Head(context.Background(), key); err != nil {
				t.Fatal(err)
			}
			r, err := c.Get(context.Background(), key, 3, `"opaque"`)
			if err != nil {
				t.Fatal(err)
			}
			r.Body.Close()
			if calls != 4 {
				t.Fatal("unexpected requests", calls)
			}
		})
	}
}

func TestV1FallbackAndInvalidMarkers(t *testing.T) {
	for _, tc := range []struct {
		xml, want string
		fail      bool
	}{
		{`<Contents><Key>a%2B%252F</Key><Size>1</Size></Contents>`, "a+%2F", false},
		{`<NextMarker>same</NextMarker>`, "", true},
		{`<NextMarker>%GG</NextMarker>`, "", true},
		{`<CommonPrefixes><Prefix>folder/</Prefix></CommonPrefixes>`, "", true},
	} {
		c, _ := New(config.Config{S3Preset: "gcs", Endpoint: "https://storage.googleapis.com", Bucket: "books", Region: "auto"})
		c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`<ListBucketResult><EncodingType>url</EncodingType><IsTruncated>true</IsTruncated>` + tc.xml + `</ListBucketResult>`))}, nil
		})
		got, err := c.List(context.Background(), "", "same")
		if (err != nil) != tc.fail || (!tc.fail && got.NextMarker != tc.want) {
			t.Fatal(got, err)
		}
	}
}
