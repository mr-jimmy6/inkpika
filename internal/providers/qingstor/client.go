// Package qingstor implements the read-only QingStor API with native QS signing.
package qingstor

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/mr-jimmy6/inkpika/internal/buildinfo"
	"github.com/mr-jimmy6/inkpika/internal/cloudhttp"
	"github.com/mr-jimmy6/inkpika/internal/config"
	"github.com/mr-jimmy6/inkpika/internal/storage"
)

type Client struct {
	endpoint, bucket, accessKey, secret string
	http                                *http.Client
}

func New(c config.Config) (*Client, error) {
	client, err := cloudhttp.New(c)
	if err != nil {
		return nil, err
	}
	return &Client{endpoint: c.Endpoint, bucket: c.Bucket, accessKey: c.AccessKeyID, secret: c.SecretAccessKey, http: client}, nil
}

// escape matches QingStor SDK URLQueryEscape, including its handling of '='.
func escape(s string) string {
	return strings.NewReplacer("%2F", "/", "%3D", "=", "+", "%20").Replace(url.QueryEscape(s))
}

func sign(req *http.Request, accessKey, secret string) {
	date := req.Header.Get("Date")
	if req.Header.Get("X-QS-Date") != "" {
		date = ""
	}
	canonical := req.Method + "\n" + req.Header.Get("Content-MD5") + "\n" + req.Header.Get("Content-Type") + "\n" + date + "\n"
	var keys []string
	for key := range req.Header {
		if strings.HasPrefix(strings.ToLower(key), "x-qs-") {
			keys = append(keys, strings.ToLower(key))
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		canonical += k + ":" + strings.TrimSpace(req.Header.Get(k)) + "\n"
	}
	canonical += escape(req.URL.Path)
	// List query parameters (prefix, marker, delimiter, limit) are not signed.
	// This client never sends subresource / response override query parameters.
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(canonical))
	req.Header.Set("Authorization", "QS "+accessKey+":"+base64.StdEncoding.EncodeToString(h.Sum(nil)))
}

func (c *Client) request(ctx context.Context, method, key string, q url.Values, headers http.Header) (*http.Response, error) {
	u, _ := url.Parse(c.endpoint)
	u.Path = "/" + c.bucket + "/" + key
	u.RawPath = escape(u.Path)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return nil, err
	}
	for k, values := range headers {
		for _, v := range values {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	req.Header.Set("User-Agent", "InkPika/"+buildinfo.Version())
	sign(req, c.accessKey, c.secret)
	resp, err := c.http.Do(req)
	if err != nil {
		c.http.CloseIdleConnections()
		return nil, fmt.Errorf("Cannot connect to QingStor; check Wi-Fi, regional endpoint and device clock: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		var detail struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 8192)).Decode(&detail)
		hint := "Check the region, bucket and network"
		switch resp.StatusCode {
		case 401, 403:
			hint = "Check credentials, bucket read permissions and the Kindle date and time"
		case 404:
			hint = "Bucket or file not found; refresh the list"
		case 412:
			hint = "Remote file changed; try again"
		case 416:
			hint = "Invalid resume offset; try again"
		}
		// Do not echo upstream messages, signed requests or credentials into logs/UI.
		return nil, fmt.Errorf("QingStor HTTP %d (%s): %s", resp.StatusCode, detail.Code, hint)
	}
	return resp, nil
}

func (c *Client) List(ctx context.Context, prefix, marker string) (storage.Listing, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	resp, err := c.request(ctx, "GET", "", url.Values{"prefix": {prefix}, "marker": {marker}, "delimiter": {"/"}, "limit": {"6"}}, nil)
	if err != nil {
		return storage.Listing{}, err
	}
	defer resp.Body.Close()
	var out storage.Listing
	err = json.NewDecoder(io.LimitReader(resp.Body, 2*1024*1024)).Decode(&out)
	if err != nil {
		return out, fmt.Errorf("Invalid QingStor file list: %w", err)
	}
	if out.HasMore && out.NextMarker == "" {
		return out, fmt.Errorf("QingStor returned more files without a pagination marker")
	}
	return out, nil
}

func (c *Client) Head(ctx context.Context, key string) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	resp, err := c.request(ctx, "HEAD", key, nil, nil)
	if err == nil {
		resp.Body.Close()
	}
	return resp, err
}

func (c *Client) Get(ctx context.Context, key string, offset int64, etag string) (*http.Response, error) {
	h := http.Header{}
	if offset > 0 {
		h.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	if etag != "" {
		h.Set("If-Match", etag)
	}
	return c.request(ctx, "GET", key, nil, h)
}
