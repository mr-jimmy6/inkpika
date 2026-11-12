package transfer

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"math/big"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mr-jimmy6/inkpika/internal/bookformat"
	"github.com/mr-jimmy6/inkpika/internal/download"
	"github.com/mr-jimmy6/inkpika/internal/i18n"
	qrcode "github.com/skip2/go-qrcode"
)

//go:embed assets/*
var assets embed.FS

type Server struct {
	cfg                     Config
	token, localToken, code string
	addresses               func() []string
	upload                  chan struct{}
	mu                      sync.Mutex
	attempts                int
	window                  time.Time
	portal                  PortalOptions
	exports                 map[string]exportTicket
	wifi                    func() string
}

// PortalOptions reuses the paired LAN transport and device connection page for
// an isolated service. API is reached only after session/Host/Origin checks.
// The ordinary transfer constructor never receives configuration APIs.
type PortalOptions struct {
	Title, Description, StopLabel string
	Assets                        fs.FS
	API                           http.Handler
	// Device APIs use only the loopback session and are never routed over LAN.
	DeviceAssets fs.FS
	DeviceAPI    http.Handler
}

func New(c Config) (*Server, error) {
	return NewPortal(c, PortalOptions{Title: "Wireless transfer", Description: "Transfer ebooks between a phone or computer and Kindle without cloud storage.", StopLabel: "Stop wireless transfer"})
}

func NewPortal(c Config, options PortalOptions) (*Server, error) {
	b := make([]byte, 64)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	n, err := rand.Int(rand.Reader, big.NewInt(900000))
	if err != nil {
		return nil, err
	}
	return &Server{cfg: c, token: hex.EncodeToString(b[:32]), localToken: hex.EncodeToString(b[32:]), code: fmt.Sprint(n.Int64() + 100000), addresses: Addresses, upload: make(chan struct{}, 1), portal: options, exports: make(map[string]exportTicket), wifi: WiFiName}, nil
}

func (s *Server) LaunchURL() string {
	return fmt.Sprintf("http://localhost:%d/#token=%s", s.cfg.UIPort, s.localToken)
}

// Prefer Wi-Fi interfaces, but support wired/hotspot LANs as well. Only private
// IPv4 addresses are advertised or accepted by the LAN listener.
func Addresses() []string {
	interfaces, _ := net.Interfaces()
	sort.SliceStable(interfaces, func(i, j int) bool {
		return strings.HasPrefix(interfaces[i].Name, "wlan") && !strings.HasPrefix(interfaces[j].Name, "wlan")
	})
	var out []string
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err == nil && ip.To4() != nil && ip.IsPrivate() {
				out = append(out, ip.String())
			}
		}
	}
	return out
}

func headers(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
}

func jsonReply(w http.ResponseWriter, status int, value interface{}) {
	i18n.JSON(w, status, value)
}
func fail(w http.ResponseWriter, status int, message string) {
	jsonReply(w, status, map[string]string{"error": message})
}
func same(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

func (s *Server) asset(w http.ResponseWriter, r *http.Request, local bool) {
	name, typ := "", ""
	switch r.URL.Path {
	case "/":
		name, typ = "upload.html", "text/html; charset=utf-8"
		if local {
			name = "device.html"
		}
	case "/app.js":
		name, typ = "upload.js", "application/javascript; charset=utf-8"
		if local {
			name = "device.js"
		}
	case "/connection.js":
		name, typ = "device.js", "application/javascript; charset=utf-8"
	case "/device.css":
		name, typ = "device.css", "text/css; charset=utf-8"
	case "/style.css":
		name, typ = "style.css", "text/css; charset=utf-8"
	case "/i18n.js":
		if r.Method != "GET" {
			fail(w, 405, "Unsupported request method")
			return
		}
		i18n.ServeScript(w)
		return
	case "/favicon.ico":
		w.WriteHeader(http.StatusNoContent)
		return
	default:
		http.NotFound(w, r)
		return
	}
	if r.Method != "GET" {
		fail(w, 405, "Unsupported request method")
		return
	}
	b, err := assets.ReadFile("assets/" + name)
	if local && s.portal.DeviceAssets != nil && r.URL.Path != "/connection.js" && name != "style.css" {
		b, err = fs.ReadFile(s.portal.DeviceAssets, "assets/"+name)
	}
	if !local && s.portal.Assets != nil && name != "style.css" {
		b, err = fs.ReadFile(s.portal.Assets, "assets/"+name)
	}
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(name, ".html") {
		b = i18n.HTML(b, w.Header().Get("Content-Language"))
	}
	w.Header().Set("Content-Type", typ)
	_, _ = w.Write(b)
}

func (s *Server) LocalHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers(w)
		w.Header().Set("Content-Language", i18n.FromConfig(s.cfg.FormatsFile))
		if r.Host != fmt.Sprintf("localhost:%d", s.cfg.UIPort) && r.Host != fmt.Sprintf("127.0.0.1:%d", s.cfg.UIPort) {
			fail(w, 403, "Only local access is allowed")
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			s.asset(w, r, true)
			return
		}
		if !same(r.Header.Get("X-Transfer-Token"), s.localToken) {
			fail(w, 401, "Reopen this page from KUAL")
			return
		}
		if r.Method != "GET" && r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+r.Host {
			fail(w, 403, "Request origin does not match")
			return
		}
		if r.URL.Path != "/api/connection" {
			if s.portal.DeviceAPI != nil {
				s.portal.DeviceAPI.ServeHTTP(w, r)
			} else {
				fail(w, 404, "Unknown endpoint or unsupported request method")
			}
			return
		}
		if r.Method != "GET" {
			fail(w, 405, "Unsupported request method")
			return
		}
		type endpoint struct {
			IP  string `json:"ip"`
			URL string `json:"url"`
			QR  string `json:"qr"`
		}
		endpoints := []endpoint{}
		for _, ip := range s.addresses() {
			url := "http://" + net.JoinHostPort(ip, strconv.Itoa(s.cfg.Port)) + "/"
			png, err := qrcode.Encode(url+"#code="+s.code, qrcode.Medium, 300)
			if err != nil {
				fail(w, 500, "Cannot generate QR code; refresh and try again")
				return
			}
			endpoints = append(endpoints, endpoint{ip, url, "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)})
		}
		jsonReply(w, 200, map[string]interface{}{"endpoints": endpoints, "code": s.code, "port": s.cfg.Port, "wifi": s.wifi(), "title": s.portal.Title, "description": s.portal.Description, "stop_label": s.portal.StopLabel})
	})
}

func (s *Server) LANHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers(w)
		w.Header().Set("Content-Language", i18n.FromConfig(s.cfg.FormatsFile))
		host, port, err := net.SplitHostPort(r.Host)
		valid := false
		for _, addr := range s.addresses() {
			if host == addr {
				valid = true
				break
			}
		}
		if err != nil || port != strconv.Itoa(s.cfg.Port) || !valid {
			fail(w, 403, "Use the LAN address shown on Kindle")
			return
		}
		if r.Method != "GET" && r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+r.Host {
			fail(w, 403, "Request origin does not match")
			return
		}
		if r.URL.Path == "/api/pair" && r.Method == "POST" {
			s.pair(w, r)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			s.asset(w, r, false)
			return
		}
		token := r.Header.Get("X-Transfer-Token")
		if r.URL.Path == "/api/download" && r.Method == "GET" {
			if cookie, err := r.Cookie(s.cookieName()); err == nil {
				token = cookie.Value
			}
		}
		if !same(token, s.token) {
			fail(w, 401, "Connection expired; enter the code shown on Kindle or scan the QR code again")
			return
		}
		if s.portal.API != nil {
			s.portal.API.ServeHTTP(w, r)
			return
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /api/info":
			formats, err := bookformat.FromFile(s.cfg.FormatsFile)
			if err != nil {
				fail(w, 503, err.Error())
				return
			}
			free, _ := download.FreeBytes(s.cfg.DownloadDir)
			jsonReply(w, 200, map[string]interface{}{"max_upload_mb": s.cfg.MaxUploadMB, "free_bytes": free, "ebook_formats": formats})
		case "POST /api/upload":
			s.receive(w, r)
		case "GET /api/books":
			s.listBooks(w, r)
		case "POST /api/export":
			s.prepareExport(w, r)
		case "GET /api/download":
			s.exportBooks(w, r)
		default:
			fail(w, 404, "Unknown endpoint or unsupported request method")
		}
	})
}

func (s *Server) pair(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	var in struct {
		Code string `json:"code"`
	}
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil || d.Decode(new(interface{})) != io.EOF {
		fail(w, 400, "Enter the six-digit connection code")
		return
	}
	s.mu.Lock()
	if time.Since(s.window) >= time.Minute {
		s.window = time.Now()
		s.attempts = 0
	}
	if s.attempts >= 10 {
		s.mu.Unlock()
		fail(w, 429, "Too many connection code attempts; try again in one minute")
		return
	}
	if !same(in.Code, s.code) {
		s.attempts++
		s.mu.Unlock()
		fail(w, 401, "Incorrect connection code; check the current code on Kindle")
		return
	}
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: s.cookieName(), Value: s.token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	jsonReply(w, 200, map[string]string{"token": s.token})
}

func (s *Server) cookieName() string { return "inkpika_" + strconv.Itoa(s.cfg.Port) }
