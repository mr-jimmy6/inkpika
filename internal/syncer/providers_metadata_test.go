package syncer

import (
	"net/http"
	"testing"
)

func TestProviderMetadataVersionsAndChecksums(t *testing.T) {
	for _, tc := range []struct{ prefix, version string }{
		{"X-Ms-", "X-Ms-Version-Id"}, {"X-Goog-", "X-Goog-Generation"}, {"X-Cos-", "X-Cos-Version-Id"}, {"X-Tos-", "X-Tos-Version-Id"},
	} {
		t.Run(tc.prefix, func(t *testing.T) {
			r := &http.Response{StatusCode: 200, ContentLength: 5, Header: http.Header{}}
			r.Header.Set(tc.version, "v1")
			r.Header.Set(tc.prefix+"Meta-Title", "title")
			r.Header.Set(tc.prefix+"Meta-Md5", "d41d8cd98f00b204e9800998ecf8427e")
			first, err := metadataOf(r)
			if err != nil || !first.stable() || first.Version != "v1" || first.MD5 != "d41d8cd98f00b204e9800998ecf8427e" {
				t.Fatal(first, err)
			}
			r.Header.Set(tc.version, "v2")
			second, err := metadataOf(r)
			if err != nil || second == first {
				t.Fatal("version change ignored")
			}
			r.Header.Set(tc.prefix+"Meta-Title", "changed")
			third, err := metadataOf(r)
			if err != nil || third == second {
				t.Fatal("metadata change ignored")
			}
			r.Header.Set("Content-MD5", "00000000000000000000000000000000")
			if _, err := metadataOf(r); err == nil {
				t.Fatal("conflicting MD5 accepted")
			}
		})
	}
	r := &http.Response{StatusCode: 200, ContentLength: 5, Header: http.Header{}}
	r.Header.Set("X-Goog-Generation", "1")
	r.Header.Set("X-Goog-Metageneration", "1")
	a, _ := metadataOf(r)
	r.Header.Set("X-Goog-Metageneration", "2")
	b, _ := metadataOf(r)
	if a == b {
		t.Fatal("GCS metadata generation ignored")
	}
}
