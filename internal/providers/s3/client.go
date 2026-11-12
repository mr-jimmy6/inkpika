// Package s3 implements the read-only S3 API, including AWS Signature Version 4.
package s3

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
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
	cfg  config.Config
	http *http.Client
}

func New(c config.Config) (*Client, error) {
	client, err := cloudhttp.New(c)
	if err != nil {
		return nil, err
	}
	return &Client{cfg: c, http: client}, nil
}

func escape(s string) string     { return strings.ReplaceAll(url.QueryEscape(s), "+", "%20") }
func escapePath(s string) string { return strings.ReplaceAll(escape(s), "%2F", "/") }

func canonicalQuery(query url.Values) string {
	var keys, pairs []string
	encoded := make(map[string][]string)
	for key, values := range query {
		key = escape(key)
		keys = append(keys, key)
		for _, value := range values {
			encoded[key] = append(encoded[key], escape(value))
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		sort.Strings(encoded[key])
		for _, value := range encoded[key] {
			pairs = append(pairs, key+"="+value)
		}
	}
	return strings.Join(pairs, "&")
}

func mac(key []byte, value string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(value))
	return h.Sum(nil)
}

// S3 does not normalize paths or double-encode the canonical URI. A literal
// percent, duplicate slash or dot segment belongs to the object key.
func sign(req *http.Request, region, accessKey, secret, token string, now time.Time) {
	stamp, date := now.UTC().Format("20060102T150405Z"), now.UTC().Format("20060102")
	empty := sha256.Sum256(nil)
	payload := hex.EncodeToString(empty[:])
	req.Header.Set("X-Amz-Date", stamp)
	req.Header.Set("X-Amz-Content-Sha256", payload)
	if token != "" {
		req.Header.Set("X-Amz-Security-Token", token)
	}
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	values := map[string]string{"host": host}
	for key, entries := range req.Header {
		name := strings.ToLower(key)
		if strings.HasPrefix(name, "x-amz-") || name == "range" || name == "if-match" {
			var normalized []string
			for _, value := range entries {
				normalized = append(normalized, strings.Join(strings.Fields(value), " "))
			}
			values[name] = strings.Join(normalized, ",")
		}
	}
	var names []string
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	var headers strings.Builder
	for _, name := range names {
		headers.WriteString(name + ":" + values[name] + "\n")
	}
	signedHeaders := strings.Join(names, ";")
	path := req.URL.Path
	if path == "" {
		path = "/"
	}
	canonical := req.Method + "\n" + escapePath(path) + "\n" + canonicalQuery(req.URL.Query()) + "\n" + headers.String() + "\n" + signedHeaders + "\n" + payload
	digest := sha256.Sum256([]byte(canonical))
	scope := date + "/" + region + "/s3/aws4_request"
	toSign := "AWS4-HMAC-SHA256\n" + stamp + "\n" + scope + "\n" + hex.EncodeToString(digest[:])
	key := mac([]byte("AWS4"+secret), date)
	for _, part := range []string{region, "s3", "aws4_request"} {
		key = mac(key, part)
	}
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+accessKey+"/"+scope+", SignedHeaders="+signedHeaders+", Signature="+hex.EncodeToString(mac(key, toSign)))
}

func (c *Client) request(ctx context.Context, method, key string, query url.Values, headers http.Header) (*http.Response, error) {
	u, err := url.Parse(c.cfg.Endpoint)
	if err != nil {
		return nil, errors.New("Invalid S3 endpoint")
	}
	u.Path = "/" + key
	if c.cfg.S3Addressing == "virtual" {
		u.Host = c.cfg.Bucket + "." + u.Host
	} else {
		u.Path = "/" + c.cfg.Bucket + "/" + key
	}
	u.RawPath = escapePath(u.Path)
	u.RawQuery = canonicalQuery(query)
	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return nil, err
	}
	for name, values := range headers {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	req.Header.Set("User-Agent", "InkPika/"+buildinfo.Version())
	sign(req, c.cfg.Region, c.cfg.AccessKeyID, c.cfg.SecretAccessKey, c.cfg.SecurityToken, time.Now())
	resp, err := c.http.Do(req)
	if err != nil {
		// Go's transport may keep dialing after request cancellation. Release
		// abandoned SOCKS handshakes as well as idle connections on failure.
		c.http.CloseIdleConnections()
		return nil, fmt.Errorf("Cannot connect to S3; check network, SOCKS5 proxy, HTTPS endpoint and device clock: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		var detail struct {
			Code string `xml:"Code"`
		}
		_ = xml.NewDecoder(io.LimitReader(resp.Body, 8192)).Decode(&detail)
		hint := "Check the endpoint, region and bucket"
		switch resp.StatusCode {
		case 301, 302, 307, 308:
			hint = "Redirects are disabled; enter the HTTPS endpoint for the bucket's actual region"
		case 401, 403:
			hint = "Check credentials, list/read permissions and the Kindle date and time"
		case 404:
			hint = "Bucket or file not found; refresh the list"
		case 412:
			hint = "Remote file changed; try again"
		case 416:
			hint = "Invalid resume offset; try again"
		}
		switch detail.Code {
		case "AccessDenied":
			hint = "Requires s3:ListBucket / s3:GetObject or the platform's equivalent list/read permissions"
		case "SignatureDoesNotMatch", "AuthorizationHeaderMalformed":
			hint = "Signature mismatch; check the secret, region, endpoint and addressing mode"
		case "InvalidAccessKeyId":
			hint = "AccessKey ID is invalid or disabled"
		case "RequestTimeTooSkewed", "RequestExpired":
			hint = "Correct the Kindle date and time"
		case "ExpiredToken", "InvalidToken", "TokenRefreshRequired":
			hint = "Temporary credentials expired or invalid; update credentials and security_token"
		case "InvalidObjectState":
			hint = "Restore the archived object on the server or use standard storage before reading"
		}
		// Do not echo upstream error fields: they can contain signed requests.
		return nil, fmt.Errorf("S3 HTTP %d: %s", resp.StatusCode, hint)
	}
	return resp, nil
}

func (c *Client) List(ctx context.Context, prefix, marker string) (storage.Listing, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	query := url.Values{"list-type": {"2"}, "prefix": {prefix}, "delimiter": {"/"}, "max-keys": {"6"}, "encoding-type": {"url"}}
	// These compatibility APIs document marker-based ListObjects. Do not assume
	// that accepting AWS signatures also implies ListObjectsV2 support.
	v1 := c.cfg.S3Preset == "tos" || c.cfg.S3Preset == "gcs" || c.cfg.S3Preset == "cos"
	if v1 {
		query.Del("list-type")
	}
	if marker != "" {
		if v1 {
			query.Set("marker", marker)
		} else {
			query.Set("continuation-token", marker)
		}
	}
	resp, err := c.request(ctx, "GET", "", query, nil)
	if err != nil {
		return storage.Listing{}, err
	}
	defer resp.Body.Close()
	var data struct {
		XMLName  xml.Name `xml:"ListBucketResult"`
		Encoding string   `xml:"EncodingType"`
		More     bool     `xml:"IsTruncated"`
		Next     string   `xml:"NextContinuationToken"`
		NextV1   string   `xml:"NextMarker"`
		Objects  []struct {
			Key      string `xml:"Key"`
			Size     int64  `xml:"Size"`
			ETag     string `xml:"ETag"`
			Modified string `xml:"LastModified"`
		} `xml:"Contents"`
		Folders []struct {
			Prefix string `xml:"Prefix"`
		} `xml:"CommonPrefixes"`
	}
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 2*1024*1024)).Decode(&data); err != nil {
		return storage.Listing{}, errors.New("Invalid S3 file list XML")
	}
	if data.Encoding != "" && data.Encoding != "url" {
		return storage.Listing{}, errors.New("Unsupported S3 file list encoding")
	}
	out := storage.Listing{Keys: []storage.Object{}, Folders: []string{}, HasMore: data.More}
	decode := func(value string) (string, error) {
		if data.Encoding == "url" {
			return url.PathUnescape(value)
		}
		return value, nil
	}
	if data.More {
		out.NextMarker = data.Next // V2 tokens are opaque; never URL-decode them.
		if v1 {
			next := data.NextV1
			if next == "" && len(data.Objects) > 0 {
				next = data.Objects[len(data.Objects)-1].Key
			}
			out.NextMarker, err = decode(next) // V1 markers are encoded object keys.
			if err != nil {
				return storage.Listing{}, errors.New("Invalid S3 pagination marker encoding")
			}
		}
		if out.NextMarker == "" || out.NextMarker == marker {
			return storage.Listing{}, errors.New("S3 returned an invalid pagination marker")
		}
	}
	for _, object := range data.Objects {
		key, err := decode(object.Key)
		if err != nil || object.Size < 0 {
			return storage.Listing{}, errors.New("Invalid S3 filename encoding or size")
		}
		modified := int64(0)
		if t, err := time.Parse(time.RFC3339Nano, object.Modified); err == nil {
			modified = t.Unix()
		}
		out.Keys = append(out.Keys, storage.Object{Key: key, Size: object.Size, ETag: object.ETag, Modified: modified})
	}
	for _, folder := range data.Folders {
		prefix, err := decode(folder.Prefix)
		if err != nil {
			return storage.Listing{}, errors.New("Invalid S3 folder encoding")
		}
		out.Folders = append(out.Folders, prefix)
	}
	return out, nil
}

func (c *Client) Head(ctx context.Context, key string) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	resp, err := c.request(ctx, "HEAD", key, nil, nil)
	if err == nil {
		resp.Body.Close()
		resp.Body = http.NoBody
	}
	return resp, err
}

func (c *Client) Get(ctx context.Context, key string, offset int64, etag string) (*http.Response, error) {
	if offset < 0 {
		return nil, errors.New("S3 resume offset cannot be negative")
	}
	headers := http.Header{}
	if offset > 0 {
		headers.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	if etag != "" {
		headers.Set("If-Match", etag)
	}
	return c.request(ctx, "GET", key, nil, headers)
}
