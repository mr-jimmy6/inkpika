// Package azureblob implements read-only Azure Blob REST operations with Shared Key.
package azureblob

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
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
	key  []byte
}

func New(c config.Config) (*Client, error) {
	client, err := cloudhttp.New(c)
	if err != nil {
		return nil, err
	}
	key, err := base64.StdEncoding.DecodeString(c.SecretAccessKey)
	if err != nil || len(key) != 64 {
		return nil, errors.New("Invalid Azure account key format")
	}
	return &Client{cfg: c, http: client, key: key}, nil
}

func canonical(req *http.Request, account string) string {
	parts := []string{req.Method}
	for _, name := range []string{"Content-Encoding", "Content-Language", "Content-Length", "Content-MD5", "Content-Type", "Date", "If-Modified-Since", "If-Match", "If-None-Match", "If-Unmodified-Since", "Range"} {
		value := req.Header.Get(name)
		if name == "Date" || (name == "Content-Length" && value == "0") {
			value = ""
		}
		parts = append(parts, value)
	}
	var names []string
	headers := map[string]string{}
	for name, values := range req.Header {
		name = strings.ToLower(name)
		if strings.HasPrefix(name, "x-ms-") {
			var normalized []string
			for _, value := range values {
				normalized = append(normalized, strings.Join(strings.Fields(value), " "))
			}
			names = append(names, name)
			headers[name] = strings.Join(normalized, ",")
		}
	}
	sort.Strings(names)
	result := strings.Join(parts, "\n") + "\n"
	for _, name := range names {
		result += name + ":" + headers[name] + "\n"
	}
	result += "/" + account + req.URL.EscapedPath()
	query := map[string][]string{}
	for name, values := range req.URL.Query() {
		name = strings.ToLower(name)
		query[name] = append(query[name], values...)
	}
	names = nil
	for name := range query {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		sort.Strings(query[name])
		result += "\n" + name + ":" + strings.Join(query[name], ",")
	}
	return result
}

func sign(req *http.Request, account string, key []byte, now time.Time) {
	req.Header.Set("X-Ms-Date", now.UTC().Format(http.TimeFormat))
	req.Header.Set("X-Ms-Version", "2021-12-02")
	h := hmac.New(sha256.New, key)
	h.Write([]byte(canonical(req, account)))
	req.Header.Set("Authorization", "SharedKey "+account+":"+base64.StdEncoding.EncodeToString(h.Sum(nil)))
}

func (c *Client) request(ctx context.Context, method, key string, query url.Values, headers http.Header) (*http.Response, error) {
	u, err := url.Parse(c.cfg.Endpoint)
	if err != nil {
		return nil, errors.New("Invalid Azure endpoint")
	}
	u.Path = "/" + c.cfg.Bucket
	if query.Get("comp") != "list" {
		u.Path += "/" + key
	}
	u.RawPath = strings.ReplaceAll(strings.ReplaceAll(url.QueryEscape(u.Path), "+", "%20"), "%2F", "/")
	u.RawQuery = query.Encode()
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
	sign(req, c.cfg.AccessKeyID, c.key, time.Now())
	resp, err := c.http.Do(req)
	if err != nil {
		c.http.CloseIdleConnections()
		return nil, fmt.Errorf("Cannot connect to Azure Blob; check network, endpoint and device clock: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8192))
		hint := "Check the container and endpoint"
		switch resp.StatusCode {
		case 301, 302, 307, 308:
			hint = "Redirects are disabled; enter the account's Blob HTTPS endpoint"
		case 401, 403:
			hint = "Check the account name, account key, Shared Key access settings and device clock"
		case 404:
			hint = "Container or file not found; refresh the list"
		case 412:
			hint = "Remote file changed; try again"
		case 416:
			hint = "Invalid resume offset; try again"
		}
		return nil, fmt.Errorf("Azure Blob HTTP %d: %s", resp.StatusCode, hint)
	}
	return resp, nil
}

type nameValue struct {
	Value   string `xml:",chardata"`
	Encoded bool   `xml:"Encoded,attr"`
}

func (n nameValue) decoded() (string, error) {
	if n.Encoded {
		return url.PathUnescape(n.Value)
	}
	return n.Value, nil
}

func (c *Client) List(ctx context.Context, prefix, marker string) (storage.Listing, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	query := url.Values{"restype": {"container"}, "comp": {"list"}, "prefix": {prefix}, "delimiter": {"/"}, "maxresults": {"6"}}
	if marker != "" {
		query.Set("marker", marker)
	}
	resp, err := c.request(ctx, "GET", "", query, nil)
	if err != nil {
		return storage.Listing{}, err
	}
	defer resp.Body.Close()
	var data struct {
		XMLName xml.Name `xml:"EnumerationResults"`
		Next    string   `xml:"NextMarker"`
		Blobs   []struct {
			Name       nameValue `xml:"Name"`
			Properties struct {
				Size     *int64 `xml:"Content-Length"`
				ETag     string `xml:"Etag"`
				Modified string `xml:"Last-Modified"`
			} `xml:"Properties"`
		} `xml:"Blobs>Blob"`
		Folders []struct {
			Name nameValue `xml:"Name"`
		} `xml:"Blobs>BlobPrefix"`
	}
	if xml.NewDecoder(io.LimitReader(resp.Body, 2*1024*1024)).Decode(&data) != nil {
		return storage.Listing{}, errors.New("Invalid Azure Blob file list XML")
	}
	if data.Next != "" && data.Next == marker {
		return storage.Listing{}, errors.New("Azure Blob returned a repeated pagination marker")
	}
	out := storage.Listing{Keys: []storage.Object{}, Folders: []string{}, HasMore: data.Next != "", NextMarker: data.Next}
	for _, object := range data.Blobs {
		key, err := object.Name.decoded()
		if err != nil || object.Properties.Size == nil || *object.Properties.Size < 0 {
			return storage.Listing{}, errors.New("Invalid Azure Blob filename or size")
		}
		modified := int64(0)
		if t, err := http.ParseTime(object.Properties.Modified); err == nil {
			modified = t.Unix()
		}
		out.Keys = append(out.Keys, storage.Object{Key: key, Size: *object.Properties.Size, ETag: object.Properties.ETag, Modified: modified})
	}
	for _, folder := range data.Folders {
		name, err := folder.Name.decoded()
		if err != nil {
			return storage.Listing{}, errors.New("Invalid Azure Blob folder encoding")
		}
		out.Folders = append(out.Folders, name)
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
		return nil, errors.New("Azure Blob resume offset cannot be negative")
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
