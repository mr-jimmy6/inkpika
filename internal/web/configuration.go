package web

import (
	"errors"
	"github.com/mr-jimmy6/inkpika/internal/bookformat"
	"github.com/mr-jimmy6/inkpika/internal/buildinfo"
	"net/http"
	"strings"

	"github.com/mr-jimmy6/inkpika/internal/diagnostics"
	"github.com/mr-jimmy6/inkpika/internal/syncer"
)

func maskedCredential(s string) string {
	if s == "" {
		return "Not configured"
	}
	r := []rune(s)
	if len(r) <= 4 {
		return "****"
	}
	return string(r[:2]) + "****" + string(r[len(r)-2:])
}

func (s *Server) configuration(w http.ResponseWriter) {
	formats, _ := bookformat.Normalize(s.cfg.EbookFormats)
	separator := ", "
	if w.Header().Get("Content-Language") == "zh" {
		separator = "、"
	}
	proxy := "Direct"
	if s.cfg.SOCKS5Proxy != "" {
		proxy = "SOCKS5 (configured)"
	}
	// An explicit allowlist: raw credentials must never cross the API boundary.
	jsonResponse(w, 200, map[string]string{
		"version": buildinfo.Version(), "packaged_at": buildinfo.PackagedAt(),
		"language": s.cfg.Language, "effective_language": w.Header().Get("Content-Language"),
		"provider": s.cfg.Provider, "bucket": s.cfg.Bucket,
		"provider_label": s.cfg.ProviderLabel(), "connection": proxy,
		"download_dir": s.cfg.DownloadDir, "sync_prefix": syncer.Prefix,
		"ebook_formats":            strings.ToUpper(strings.Join(formats, separator)),
		"access_key_id_masked":     maskedCredential(s.cfg.AccessKeyID),
		"secret_access_key_masked": maskedCredential(s.cfg.SecretAccessKey),
	})
}

func (s *Server) connectionCheck(w http.ResponseWriter, req *http.Request) {
	var result diagnostics.Result
	var err error
	if req.Method == "POST" {
		var in struct{}
		if !decode(w, req, &in) {
			return
		}
		result, err = diagnostics.Run(req.Context(), s.cfg, s.store, s.checkProbe)
	} else {
		result, err = diagnostics.Read(s.cfg)
	}
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, diagnostics.ErrRunning) {
			code = http.StatusConflict
		}
		failure(w, code, diagnostics.Redact(s.cfg, err.Error()))
		return
	}
	jsonResponse(w, 200, result.Public(s.cfg))
}
