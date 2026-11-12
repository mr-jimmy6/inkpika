// Package storage defines the provider-independent, read-only object store API.
package storage

import (
	"context"
	"net/http"
)

type Object struct {
	Key      string `json:"key"`
	Size     int64  `json:"size"`
	ETag     string `json:"etag"`
	Modified int64  `json:"modified"`
}

type Listing struct {
	Keys       []Object `json:"keys"`
	Folders    []string `json:"common_prefixes"`
	HasMore    bool     `json:"has_more"`
	NextMarker string   `json:"next_marker"`
}

// Store exposes read-only object operations. Markers are opaque.
// Callers close successful Head/Get response bodies; implementations may return
// http.NoBody for Head. Implementations must close bodies before returning errors.
type Store interface {
	List(ctx context.Context, prefix, marker string) (Listing, error)
	Head(ctx context.Context, key string) (*http.Response, error)
	Get(ctx context.Context, key string, offset int64, etag string) (*http.Response, error)
}
