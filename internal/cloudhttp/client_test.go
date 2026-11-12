package cloudhttp

import (
	"context"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mr-jimmy6/inkpika/internal/config"
)

type proxyRequest struct {
	host, user, password string
	port                 uint16
}

// One real SOCKS5 connection, with optional RFC 1929 authentication. The target
// is a local TLS fixture; the requested DNS name must reach the proxy unchanged.
func proxyFixture(t *testing.T, target string, auth, reject bool) (string, <-chan proxyRequest) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	requests := make(chan proxyRequest, 1)
	go func() {
		defer close(requests)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		read := func(n int) []byte {
			b := make([]byte, n)
			if _, err := io.ReadFull(conn, b); err != nil {
				return nil
			}
			return b
		}
		greeting := read(2)
		if len(greeting) != 2 || greeting[0] != 5 {
			return
		}
		if read(int(greeting[1])) == nil {
			return
		}
		var request proxyRequest
		if auth {
			conn.Write([]byte{5, 2})
			header := read(2)
			if len(header) != 2 || header[0] != 1 {
				return
			}
			request.user = string(read(int(header[1])))
			size := read(1)
			if len(size) != 1 {
				return
			}
			request.password = string(read(int(size[0])))
			if reject {
				conn.Write([]byte{1, 1})
				requests <- request
				return
			}
			conn.Write([]byte{1, 0})
		} else {
			conn.Write([]byte{5, 0})
		}
		header := read(4)
		if len(header) != 4 || header[0] != 5 || header[1] != 1 {
			return
		}
		switch header[3] {
		case 3:
			size := read(1)
			if len(size) != 1 {
				return
			}
			request.host = string(read(int(size[0])))
		case 1:
			request.host = net.IP(read(4)).String()
		case 4:
			request.host = net.IP(read(16)).String()
		default:
			return
		}
		port := read(2)
		if len(port) != 2 {
			return
		}
		request.port = binary.BigEndian.Uint16(port)
		requests <- request
		upstream, err := net.DialTimeout("tcp", target, time.Second)
		if err != nil {
			return
		}
		defer upstream.Close()
		upstream.SetDeadline(time.Now().Add(5 * time.Second))
		conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
		done := make(chan struct{})
		go func() { io.Copy(upstream, conn); upstream.Close(); close(done) }()
		io.Copy(conn, upstream)
		conn.Close()
		<-done
	}()
	return listener.Addr().String(), requests
}

func caFile(t *testing.T, server *httptest.Server) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSOCKS5AuthRemoteDNSAndVerifiedTLS(t *testing.T) {
	for _, tc := range []struct {
		scheme string
		auth   bool
	}{{"socks5", false}, {"socks5", true}, {"socks5h", true}} {
		t.Run(fmt.Sprintf("%s/%t", tc.scheme, tc.auth), func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Proxy-Authorization") != "" {
					t.Error("proxy credentials forwarded to storage")
				}
				fmt.Fprint(w, "book")
			}))
			defer server.Close()
			address, requests := proxyFixture(t, server.Listener.Addr().String(), tc.auth, false)
			proxy := &url.URL{Scheme: tc.scheme, Host: address}
			if tc.auth {
				proxy.User = url.UserPassword("reader", "private:@ password")
			}
			client, err := New(config.Config{SOCKS5Proxy: proxy.String(), CABundle: caFile(t, server)})
			if err != nil {
				t.Fatal(err)
			}
			defer client.CloseIdleConnections()
			// example.com is in httptest's certificate. No public DNS/network is used.
			r, err := client.Get("https://example.com:9443/books/test.azw3")
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(r.Body)
			r.Body.Close()
			if err != nil || string(data) != "book" {
				t.Fatal(err, string(data))
			}
			request := <-requests
			if request.host != "example.com" || request.port != 9443 {
				t.Fatal("target name was not resolved at proxy", request)
			}
			if tc.auth && (request.user != "reader" || request.password != "private:@ password") {
				t.Fatal("RFC 1929 credentials were not decoded correctly")
			}
		})
	}
}

func TestSOCKS5FailuresNeverFallbackOrBypassTLS(t *testing.T) {
	for _, mode := range []string{"auth", "untrusted", "hostname", "unreachable"} {
		t.Run(mode, func(t *testing.T) {
			var hits atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
			defer server.Close()
			address, _ := proxyFixture(t, server.Listener.Addr().String(), true, mode == "auth")
			if mode == "unreachable" {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				address = listener.Addr().String()
				listener.Close()
			}
			cfg := config.Config{SOCKS5Proxy: "socks5://reader:private-password@" + address, CABundle: caFile(t, server)}
			if mode == "untrusted" {
				cfg.CABundle = ""
			}
			client, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer client.CloseIdleConnections()
			target := server.URL
			if mode == "hostname" {
				target = "https://wrong-certificate.invalid"
			}
			r, err := client.Get(target)
			if r != nil {
				r.Body.Close()
			}
			if err == nil || hits.Load() != 0 {
				t.Fatal("failure bypassed proxy or TLS")
			}
			if strings.Contains(err.Error(), "private-password") {
				t.Fatal("proxy password exposed")
			}
		})
	}
}

func TestProxyHandshakeCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(2 * time.Second))
		io.Copy(io.Discard, conn)
	}()
	client, err := New(config.Config{SOCKS5Proxy: "socks5://" + listener.Addr().String()})
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://example.invalid", nil)
	start := time.Now()
	_, err = client.Do(req)
	client.CloseIdleConnections() // Providers and diagnostics clean up failed requests.
	if err == nil || time.Since(start) > time.Second {
		t.Fatal("SOCKS handshake ignored cancellation", err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SOCKS connection leaked")
	}
}

func TestExplicitNetworkPolicyAndInvalidCA(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://unwanted.invalid:3128")
	client, err := New(config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	if client.Transport.(*http.Transport).Proxy != nil {
		t.Fatal("ambient proxy enabled")
	}
	path := filepath.Join(t.TempDir(), "bad.pem")
	os.WriteFile(path, []byte("not a certificate"), 0600)
	for _, ca := range []string{path, path + "-missing", filepath.Dir(path)} {
		if _, err := New(config.Config{CABundle: ca}); err == nil || strings.Contains(err.Error(), ca) {
			t.Fatal("invalid CA accepted or private path echoed", err)
		}
	}
	if _, err := New(config.Config{SOCKS5Proxy: "https://reader:private-password@proxy.invalid"}); err == nil || strings.Contains(err.Error(), "private-password") {
		t.Fatal("bad proxy accepted or leaked")
	}
}
