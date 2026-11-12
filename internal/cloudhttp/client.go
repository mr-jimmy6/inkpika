// Package cloudhttp shares TLS and connection policy across storage providers.
package cloudhttp

import (
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/mr-jimmy6/inkpika/internal/config"
)

// Mozilla's CA bundle from https://curl.se/ca/cacert.pem; see THIRD_PARTY.md.
//
//go:embed cacert.pem
var roots []byte

func New(c config.Config) (*http.Client, error) {
	proxy, err := config.ParseSOCKS5Proxy(c.SOCKS5Proxy)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(roots)
	if c.CABundle != "" {
		pem, err := readCA(c.CABundle)
		if err != nil || !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("Cannot read ca_bundle or the file contains no valid PEM CA certificates")
		}
	}
	var proxyFunc func(*http.Request) (*url.URL, error)
	if proxy != nil {
		proxyFunc = http.ProxyURL(proxy)
	}
	return &http.Client{
		Transport: &http.Transport{
			Proxy:               proxyFunc,
			DialContext:         (&net.Dialer{Timeout: 20 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSClientConfig:     &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
			TLSHandshakeTimeout: 20 * time.Second, ResponseHeaderTimeout: 30 * time.Second,
			IdleConnTimeout: 60 * time.Second, MaxIdleConns: 2, MaxConnsPerHost: 3,
			DisableCompression: true,
		},
		// Never forward credentials to a redirect destination, including the same host.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, nil
}

func readCA(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	const maximum = 1024 * 1024
	if !info.Mode().IsRegular() || info.Size() > maximum {
		return nil, errors.New("invalid CA file")
	}
	data, err := io.ReadAll(io.LimitReader(f, maximum+1))
	if len(data) > maximum {
		return nil, errors.New("CA file too large")
	}
	return data, err
}
