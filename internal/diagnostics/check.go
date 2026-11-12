// Package diagnostics performs bounded, read-only object-storage checks.
package diagnostics

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mr-jimmy6/inkpika/internal/cloudhttp"
	"github.com/mr-jimmy6/inkpika/internal/config"
	"github.com/mr-jimmy6/inkpika/internal/storage"
)

type Result struct {
	Source  string `json:"source"`
	Running bool   `json:"running"`
	Updated int64  `json:"updated"`
	Network string `json:"network"`
	Access  string `json:"access"`
	Read    string `json:"read"`
	Detail  string `json:"detail,omitempty"`
}

// Public omits persistent source identity and redacts every diagnostic message.
func (r Result) Public(c config.Config) map[string]interface{} {
	updated := ""
	if r.Updated > 0 {
		updated = time.Unix(r.Updated, 0).Local().Format("2006-01-02 15:04:05")
	}
	return map[string]interface{}{
		"running": r.Running, "updated": r.Updated, "updated_local": updated,
		"network": Redact(c, r.Network), "access": Redact(c, r.Access),
		"read": Redact(c, r.Read), "detail": Redact(c, r.Detail),
	}
}

// Redact removes credentials even if an unexpected transport/provider error
// includes them. Only this sanitized result is persisted or shown in KUAL.
func Redact(c config.Config, s string) string {
	secrets := []string{c.SecretAccessKey, c.SecurityToken, c.AccessKeyID, c.SOCKS5Proxy}
	if proxy, err := url.Parse(c.SOCKS5Proxy); err == nil && proxy.User != nil {
		password, _ := proxy.User.Password()
		secrets = append(secrets, proxy.User.Username(), password)
	}
	for _, secret := range secrets {
		if secret != "" {
			s = strings.ReplaceAll(s, secret, "[redacted]")
			s = strings.ReplaceAll(s, url.QueryEscape(secret), "[redacted]")
		}
	}
	return s
}

// Probe checks DNS, TCP and verified TLS independently of Bucket permissions.
// An unsigned 403 still proves connectivity; Check tests signed access next.
func Probe(ctx context.Context, c config.Config) error {
	u, err := url.Parse(c.Endpoint)
	if err != nil {
		return err
	}
	if c.Provider == config.AliyunOSS || (c.Provider == config.S3 && c.S3Addressing == "virtual") {
		u.Host = c.Bucket + "." + u.Host
	} else {
		u.Path = "/" + c.Bucket + "/"
	}
	req, err := http.NewRequestWithContext(ctx, "HEAD", u.String(), nil)
	if err != nil {
		return err
	}
	client, err := cloudhttp.New(c)
	if err != nil {
		return err
	}
	defer client.CloseIdleConnections()
	r, err := client.Do(req)
	if err != nil {
		return err
	}
	return r.Body.Close()
}

func Check(ctx context.Context, c config.Config, store storage.Store, probe func(context.Context, config.Config) error) (out Result) {
	out = Result{Source: c.SourceIdentity(), Network: "Not checked", Access: "Not checked", Read: "Not verified"}
	defer func() { out.Updated = time.Now().Unix(); out.Detail = Redact(c, out.Detail) }()
	if err := probe(ctx, c); err != nil {
		out.Network = "Failed"
		out.Detail = "Network connection failed (DNS/TCP/TLS): " + err.Error()
		return
	}
	out.Network = "Passed (HTTPS)"
	// Look for one readable sample inside the configured prefix, following both
	// folders and opaque page markers. Bound requests and reject pagination loops.
	type page struct{ prefix, marker string }
	queue := []page{{prefix: c.Prefix}}
	seen := map[page]bool{}
	for n := 0; len(queue) > 0 && n < 32; n++ {
		p := queue[0]
		queue = queue[1:]
		if seen[p] {
			out.Detail = "Folder pagination loop; cannot complete the read check"
			return
		}
		seen[p] = true
		listing, err := store.List(ctx, p.prefix, p.marker)
		if err != nil {
			out.Access = "Failed"
			out.Detail = "Bucket listing failed: " + err.Error()
			return
		}
		out.Access = "Passed (current prefix)"
		for _, o := range listing.Keys {
			if !strings.HasPrefix(o.Key, c.Prefix) || strings.HasSuffix(o.Key, "/") {
				continue
			}
			r, err := store.Get(ctx, o.Key, 0, "")
			if err != nil {
				out.Read = "Failed"
				out.Detail = "File read failed: " + err.Error()
				return
			}
			// Read just one byte and close immediately; never create a local book.
			n, err := io.CopyN(io.Discard, r.Body, 1)
			r.Body.Close()
			if err != nil && !(err == io.EOF && o.Size == 0) {
				out.Read = "Failed"
				out.Detail = "File response read failed: " + err.Error()
				return
			}
			out.Read = "Passed (sample)"
			out.Detail = fmt.Sprintf("Read a %d-byte sample without saving it. This does not verify permissions for every object.", n)
			return
		}
		if listing.HasMore || listing.NextMarker != "" {
			if listing.NextMarker == "" {
				out.Detail = "Missing pagination marker; file reading not verified"
				return
			}
			queue = append(queue, page{p.prefix, listing.NextMarker})
		}
		for _, f := range listing.Folders {
			if f != p.prefix && strings.HasPrefix(f, p.prefix) && strings.HasSuffix(f, "/") {
				queue = append(queue, page{prefix: f})
			}
		}
	}
	if len(queue) > 0 {
		out.Detail = "Checked 32 pages without finding a file; read permission not verified"
	} else {
		out.Detail = "Current scope is empty; bucket listing works, file read permission not verified"
	}
	return
}
