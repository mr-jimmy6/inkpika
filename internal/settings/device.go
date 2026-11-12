package settings

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/mr-jimmy6/inkpika/internal/config"
	"github.com/mr-jimmy6/inkpika/internal/diagnostics"
	"github.com/mr-jimmy6/inkpika/internal/providers"
)

// device is reached only after the portal validates its loopback session.
// Editing remains on the separately paired LAN listener.
func (s *Server) device(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" && r.URL.Path == "/api/config" {
		s.ServeHTTP(w, r)
		return
	}
	if r.URL.Path != "/api/connection-check" || (r.Method != "GET" && r.Method != "POST") {
		failure(w, 404, "Unknown endpoint or unsupported request method")
		return
	}
	if r.Method == "POST" {
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		var in struct{}
		if d.Decode(&in) != nil || d.Decode(new(interface{})) != io.EOF {
			failure(w, 400, "Invalid configuration content")
			return
		}
	}
	s.mu.Lock()
	c, err := config.Load(s.filename)
	s.mu.Unlock()
	if err != nil {
		failure(w, 400, "Save valid cloud settings from a phone or computer before running a connection test.")
		return
	}
	var result diagnostics.Result
	if r.Method == "POST" {
		store, createErr := providers.New(c)
		if createErr != nil {
			err = createErr
		} else {
			result, err = diagnostics.Run(r.Context(), c, store, diagnostics.Probe)
		}
	} else {
		result, err = diagnostics.Read(c)
	}
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, diagnostics.ErrRunning) {
			status = http.StatusConflict
		}
		failure(w, status, diagnostics.Redact(c, err.Error()))
		return
	}
	reply(w, 200, result.Public(c))
}
