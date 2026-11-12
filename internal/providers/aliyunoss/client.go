// Package aliyunoss implements read-only Alibaba Cloud OSS using native V4 auth.
package aliyunoss

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
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
	endpoint, bucket, region, accessKey, secret, token string
	http                                               *http.Client
}

func New(c config.Config) (*Client, error) {
	client, err := cloudhttp.New(c)
	if err != nil {
		return nil, err
	}
	return &Client{endpoint: c.Endpoint, bucket: c.Bucket, region: c.Region, accessKey: c.AccessKeyID,
		secret: c.SecretAccessKey, token: c.SecurityToken, http: client}, nil
}

func escape(s string) string     { return strings.ReplaceAll(url.QueryEscape(s), "+", "%20") }
func escapePath(s string) string { return strings.ReplaceAll(escape(s), "%2F", "/") }

func mac(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

// V4 signs the logical /bucket/key resource even though OSS uses virtual hosting.
// Empty query values have no '=' in the canonical form. Every x-oss-* header is signed.
func sign(req *http.Request, bucket, key, region, accessKey, secret, token string, now time.Time) {
	stamp := now.UTC().Format("20060102T150405Z")
	date := now.UTC().Format("20060102")
	req.Header.Set("Date", now.UTC().Format(http.TimeFormat))
	req.Header.Set("X-Oss-Date", stamp)
	req.Header.Set("X-Oss-Content-Sha256", "UNSIGNED-PAYLOAD")
	if token != "" {
		req.Header.Set("X-Oss-Security-Token", token)
	}
	query := make([]string, 0)
	encoded := make(map[string][]string)
	var queryKeys []string
	for k, values := range req.URL.Query() {
		ek := escape(k)
		queryKeys = append(queryKeys, ek)
		for _, v := range values {
			encoded[ek] = append(encoded[ek], escape(v))
		}
	}
	sort.Strings(queryKeys)
	for _, k := range queryKeys {
		sort.Strings(encoded[k])
		for _, v := range encoded[k] {
			part := k
			if v != "" {
				part += "=" + v
			}
			query = append(query, part)
		}
	}
	var headers []string
	for k := range req.Header {
		low := strings.ToLower(k)
		if strings.HasPrefix(low, "x-oss-") || low == "content-type" || low == "content-md5" {
			headers = append(headers, low)
		}
	}
	sort.Strings(headers)
	var canonicalHeaders strings.Builder
	for _, k := range headers {
		values := append([]string(nil), req.Header.Values(k)...)
		for i := range values {
			values[i] = strings.TrimSpace(values[i])
		}
		canonicalHeaders.WriteString(k + ":" + strings.Join(values, ",") + "\n")
	}
	canonical := req.Method + "\n" + escapePath("/"+bucket+"/"+key) + "\n" + strings.Join(query, "&") + "\n" + canonicalHeaders.String() + "\n\nUNSIGNED-PAYLOAD"
	digest := sha256.Sum256([]byte(canonical))
	scope := date + "/" + region + "/oss/aliyun_v4_request"
	toSign := "OSS4-HMAC-SHA256\n" + stamp + "\n" + scope + "\n" + hex.EncodeToString(digest[:])
	keyBytes := mac([]byte("aliyun_v4"+secret), date)
	for _, part := range []string{region, "oss", "aliyun_v4_request"} {
		keyBytes = mac(keyBytes, part)
	}
	req.Header.Set("Authorization", "OSS4-HMAC-SHA256 Credential="+accessKey+"/"+scope+",Signature="+hex.EncodeToString(mac(keyBytes, toSign)))
}

func (c *Client) request(ctx context.Context, method, key string, query url.Values, headers http.Header) (*http.Response, error) {
	u, err := url.Parse(c.endpoint)
	if err != nil {
		return nil, err
	}
	u.Host = c.bucket + "." + u.Host
	u.Path = "/" + key
	u.RawPath = escapePath(u.Path)
	u.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return nil, err
	}
	for k, values := range headers {
		for _, v := range values {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("User-Agent", "InkPika/"+buildinfo.Version())
	sign(req, c.bucket, key, c.region, c.accessKey, c.secret, c.token, time.Now())
	resp, err := c.http.Do(req)
	if err != nil {
		c.http.CloseIdleConnections()
		return nil, fmt.Errorf("Cannot connect to Alibaba Cloud OSS; check Wi-Fi, public endpoint and device clock: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		var detail struct {
			Code string `xml:"Code"`
		}
		_ = xml.NewDecoder(io.LimitReader(resp.Body, 8192)).Decode(&detail)
		hint := "Check the region, bucket and public endpoint"
		switch resp.StatusCode {
		case 401, 403:
			hint = "Check credentials, oss:ListObjects / oss:GetObject permissions and the Kindle date and time"
		case 404:
			hint = "Bucket or file not found; refresh the list"
		case 412:
			hint = "Remote file changed; try again"
		case 416:
			hint = "Invalid resume offset; try again"
		}
		switch detail.Code {
		case "InvalidAccessKeyId":
			hint = "AccessKey ID does not exist or is disabled; check access_key_id in the configuration"
		case "SignatureDoesNotMatch":
			hint = "Request signature mismatch; check the secret, region and endpoint"
		case "AccessDenied", "AccessForbidden":
			hint = "Account or bucket policy denied this action; check oss:ListObjects / oss:GetObject permissions and access restrictions"
		case "RequestTimeTooSkewed", "RequestExpired":
			hint = "Invalid request time; correct the Kindle date, time and timezone"
		case "NoSuchBucket":
			hint = "Bucket does not exist; check its name and region"
		case "SecurityTokenExpired", "InvalidSecurityToken":
			hint = "STS credentials expired or invalid; update credentials and security_token, then restart"
		case "InvalidObjectState":
			hint = "This object is archived; restore it in the OSS console first"
		}
		return nil, fmt.Errorf("Alibaba Cloud OSS HTTP %d (%s): %s", resp.StatusCode, detail.Code, hint)
	}
	return resp, nil
}

func (c *Client) List(ctx context.Context, prefix, marker string) (storage.Listing, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	q := url.Values{"list-type": {"2"}, "prefix": {prefix}, "delimiter": {"/"}, "max-keys": {"6"}, "encoding-type": {"url"}}
	if marker != "" {
		q.Set("continuation-token", marker)
	}
	resp, err := c.request(ctx, "GET", "", q, nil)
	if err != nil {
		return storage.Listing{}, err
	}
	defer resp.Body.Close()
	var data struct {
		XMLName  xml.Name `xml:"ListBucketResult"`
		Encoding string   `xml:"EncodingType"`
		More     bool     `xml:"IsTruncated"`
		Next     string   `xml:"NextContinuationToken"`
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
	if err = xml.NewDecoder(io.LimitReader(resp.Body, 2*1024*1024)).Decode(&data); err != nil {
		return storage.Listing{}, fmt.Errorf("Invalid Alibaba Cloud OSS file list: %w", err)
	}
	if data.More && (data.Next == "" || data.Next == marker) {
		return storage.Listing{}, fmt.Errorf("Alibaba Cloud OSS returned an invalid pagination marker")
	}
	out := storage.Listing{Keys: []storage.Object{}, Folders: []string{}, HasMore: data.More}
	// Continuation tokens are opaque and must NOT be URL-decoded with object keys.
	if data.More {
		out.NextMarker = data.Next
	}
	decode := func(s string) (string, error) {
		if data.Encoding == "url" {
			return url.PathUnescape(s)
		}
		return s, nil
	}
	for _, o := range data.Objects {
		key, err := decode(o.Key)
		if err != nil {
			return out, fmt.Errorf("Invalid OSS filename encoding: %w", err)
		}
		modified := int64(0)
		if t, e := time.Parse(time.RFC3339Nano, o.Modified); e == nil {
			modified = t.Unix()
		}
		out.Keys = append(out.Keys, storage.Object{Key: key, Size: o.Size, ETag: o.ETag, Modified: modified})
	}
	for _, folder := range data.Folders {
		prefix, err := decode(folder.Prefix)
		if err != nil {
			return out, fmt.Errorf("Invalid OSS folder encoding: %w", err)
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
	}
	return resp, err
}

func (c *Client) Get(ctx context.Context, key string, offset int64, etag string) (*http.Response, error) {
	headers := http.Header{}
	if offset > 0 {
		headers.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	if etag != "" {
		headers.Set("If-Match", etag)
	}
	return c.request(ctx, "GET", key, nil, headers)
}
