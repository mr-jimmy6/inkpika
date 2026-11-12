package web

import (
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mr-jimmy6/inkpika/internal/buildinfo"
	"github.com/mr-jimmy6/inkpika/internal/config"
	"github.com/mr-jimmy6/inkpika/internal/diagnostics"
	"github.com/mr-jimmy6/inkpika/internal/download"
	"github.com/mr-jimmy6/inkpika/internal/i18n"
	"github.com/mr-jimmy6/inkpika/internal/storage"
	"github.com/mr-jimmy6/inkpika/internal/syncer"
)

//go:embed assets/*
var assets embed.FS

type Server struct {
	cfg        config.Config
	store      storage.Store
	downloads  *download.Manager
	token      string
	sync       *syncer.Engine
	checkProbe func(context.Context, config.Config) error
}

func New(c config.Config, store storage.Store, downloads *download.Manager, token string, engines ...*syncer.Engine) http.Handler {
	s := &Server{cfg: c, store: store, downloads: downloads, token: token, checkProbe: diagnostics.Probe}
	if len(engines) > 0 {
		s.sync = engines[0]
	}
	return http.HandlerFunc(s.serve)
}

func jsonResponse(w http.ResponseWriter, code int, v interface{}) {
	i18n.JSON(w, code, v)
}
func failure(w http.ResponseWriter, code int, message string) {
	jsonResponse(w, code, map[string]string{"error": message})
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Language", i18n.Resolve(s.cfg.Language))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'")
	if r.Host != s.cfg.Listen && r.Host != s.cfg.BrowserAddress() {
		failure(w, 403, "Only local addresses are allowed")
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-InkPika-Token")), []byte(s.token)) != 1 {
			failure(w, 401, "Session expired; reopen the cloud library from KUAL")
			return
		}
		if r.Method == "POST" && r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+r.Host {
			failure(w, 403, "Invalid request origin")
			return
		}
		s.api(w, r)
		return
	}
	if r.Method != "GET" {
		failure(w, 405, "Unsupported request method")
		return
	}
	file, typ := "", ""
	switch r.URL.Path {
	case "/i18n.js":
		i18n.ServeScript(w)
		return
	case "/favicon.ico":
		w.WriteHeader(http.StatusNoContent)
		return
	case "/":
		file = "index.html"
		typ = "text/html; charset=utf-8"
	case "/app.js":
		file = "app.js"
		typ = "application/javascript; charset=utf-8"
	case "/style.css":
		file = "style.css"
		typ = "text/css; charset=utf-8"
	default:
		http.NotFound(w, r)
		return
	}
	data, _ := assets.ReadFile("assets/" + file)
	if file == "index.html" {
		data = i18n.HTML(data, w.Header().Get("Content-Language"))
	}
	w.Header().Set("Content-Type", typ)
	_, _ = w.Write(data)
}

func decode(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		failure(w, 400, "Invalid request content")
		return false
	}
	if err := d.Decode(new(interface{})); err != io.EOF {
		failure(w, 400, "Request must contain exactly one JSON object")
		return false
	}
	return true
}

func (s *Server) api(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/sync/") {
		if s.sync == nil {
			failure(w, 503, "Sync service is not enabled")
			return
		}
		command := strings.TrimPrefix(r.URL.Path, "/api/sync/")
		if command == "status" && r.Method == "GET" {
			jsonResponse(w, 200, s.sync.Snapshot())
			return
		}
		if r.Method != "POST" {
			failure(w, 405, "Unsupported request method")
			return
		}
		var err error
		switch command {
		case "refresh":
			err = s.sync.Refresh()
		case "scan":
			err = s.sync.Start(false)
		case "start":
			err = s.sync.Start(true)
		case "cancel":
			s.sync.Cancel()
		default:
			failure(w, 404, "Unknown sync action")
			return
		}
		if err != nil {
			failure(w, 409, err.Error())
			return
		}
		jsonResponse(w, 202, s.sync.Snapshot())
		return
	}
	switch r.Method + " " + r.URL.Path {
	case "GET /api/config":
		s.configuration(w)
	case "GET /api/connection-check", "POST /api/connection-check":
		s.connectionCheck(w, r)
	case "GET /api/info":
		free, _ := download.FreeBytes(s.cfg.DownloadDir)
		jsonResponse(w, 200, map[string]interface{}{"bucket": s.cfg.Bucket, "zone": s.cfg.Zone, "prefix": s.cfg.Prefix, "download_dir": s.cfg.DownloadDir, "free_bytes": free, "max_download_mb": s.cfg.MaxDownloadMB, "version": buildinfo.Version(), "packaged_at": buildinfo.PackagedAt(), "provider": s.cfg.Provider, "provider_label": s.cfg.ProviderLabel(), "region": s.cfg.Region})
	case "GET /api/objects":
		prefix := r.URL.Query().Get("prefix")
		marker := r.URL.Query().Get("marker")
		if prefix == "" {
			prefix = s.cfg.Prefix
		}
		if !strings.HasPrefix(prefix, s.cfg.Prefix) || len(prefix) > 4096 || len(marker) > 4096 {
			failure(w, 400, "Folder is outside the configured scope")
			return
		}
		listing, err := s.store.List(r.Context(), prefix, marker)
		if err != nil {
			failure(w, 502, err.Error())
			return
		}
		type entry struct {
			storage.Object
			Supported     bool   `json:"supported"`
			Downloaded    bool   `json:"downloaded"`
			ModifiedLocal string `json:"modified_local"`
		}
		entries := make([]entry, 0, len(listing.Keys))
		folders := make([]string, 0, len(listing.Folders))
		for _, o := range listing.Keys {
			if strings.HasPrefix(o.Key, prefix) && !strings.HasSuffix(o.Key, "/") && s.cfg.Supports(o.Key) {
				// Mesquite's chroot uses UTC even when the Kindle system clock has
				// a local timezone. Format here so the UI matches the device clock.
				modifiedLocal := ""
				if o.Modified > 0 {
					modifiedLocal = time.Unix(o.Modified, 0).Local().Format("2006-01-02 15:04")
				}
				entries = append(entries, entry{Object: o, Supported: true, Downloaded: s.downloads.IsDownloaded(o.Key), ModifiedLocal: modifiedLocal})
			}
		}
		for _, f := range listing.Folders {
			if f != prefix && strings.HasPrefix(f, prefix) && strings.HasSuffix(f, "/") {
				folders = append(folders, f)
			}
		}
		jsonResponse(w, 200, map[string]interface{}{"keys": entries, "common_prefixes": folders, "next_marker": listing.NextMarker, "has_more": listing.HasMore || listing.NextMarker != "", "prefix": prefix})
	case "GET /api/jobs":
		type entry struct {
			download.Job
			SavedLocally bool   `json:"saved_locally"`
			LocalState   string `json:"local_state"`
		}
		jobs := s.downloads.Snapshot()
		entries := make([]entry, 0, len(jobs))
		for _, j := range jobs {
			j.TransferComplete = j.TransferComplete || j.Status == "completed"
			entries = append(entries, entry{j, s.downloads.IsDownloaded(j.Key), s.downloads.LocalState(j)})
		}
		jsonResponse(w, 200, entries)
	case "POST /api/download":
		var in struct {
			Key string `json:"key"`
		}
		if !decode(w, r, &in) {
			return
		}
		job, err := s.downloads.Add(in.Key)
		if err != nil {
			failure(w, 400, err.Error())
			return
		}
		jsonResponse(w, 202, job)
	case "POST /api/jobs/action":
		var in struct {
			ID     string `json:"id"`
			Action string `json:"action"`
		}
		if !decode(w, r, &in) {
			return
		}
		if err := s.downloads.Action(in.ID, in.Action); err != nil {
			failure(w, 400, err.Error())
			return
		}
		jsonResponse(w, 200, map[string]bool{"ok": true})
	case "POST /api/jobs/clear":
		var in struct{}
		if !decode(w, r, &in) {
			return
		}
		if err := s.downloads.Clear(); err != nil {
			failure(w, 500, err.Error())
			return
		}
		jsonResponse(w, 200, map[string]bool{"ok": true})
	default:
		failure(w, 404, "Unknown endpoint or unsupported request method")
	}
}
