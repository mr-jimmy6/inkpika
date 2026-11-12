package config

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// ParseSOCKS5Proxy never echoes the supplied URL: it may contain credentials.
// Go's HTTP transport resolves target hostnames at the SOCKS5 proxy for both
// socks5 and socks5h. Ambient HTTP(S)_PROXY variables are intentionally ignored.
func ParseSOCKS5Proxy(value string) (*url.URL, error) {
	if value == "" {
		return nil, nil
	}
	bad := errors.New("socks5_proxy must be socks5://host:port or socks5h://host:port with optional username and password; no path, query or fragment")
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "socks5" && u.Scheme != "socks5h") || u.Opaque != "" || u.Hostname() == "" || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, bad
	}
	_, port, err := net.SplitHostPort(u.Host)
	n, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || n < 1 || n > 65535 || strings.ContainsAny(u.Host, "\r\n\t /\\") {
		return nil, bad
	}
	if u.User != nil {
		password, present := u.User.Password()
		if !present || len(u.User.Username()) < 1 || len(u.User.Username()) > 255 || len(password) < 1 || len(password) > 255 || strings.ContainsAny(u.User.Username()+password, "\x00\r\n") {
			return nil, errors.New("Provide both SOCKS5 username and password (1–255 bytes each, no line breaks or NUL); URL-encode special characters")
		}
	}
	return u, nil
}
