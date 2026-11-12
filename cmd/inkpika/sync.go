package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mr-jimmy6/inkpika/internal/config"
)

// Local IPC: the token is read from a private file, never from shell arguments
// and never printed. Requests can only target the configured loopback listener.
func syncControl(cfg config.Config, command string) error {
	method := "POST"
	switch command {
	case "start", "scan", "cancel", "refresh":
	case "status":
		method = "GET"
	default:
		return errors.New("Sync command must be refresh/scan/start/status/cancel")
	}
	b, err := os.ReadFile(filepath.Join(cfg.StateDir, "launch.url"))
	if err != nil {
		return errors.New("Service is not running; retry from KUAL")
	}
	u, err := url.Parse(strings.TrimSpace(string(b)))
	if err != nil || u.Scheme != "http" || u.Host != cfg.BrowserAddress() || !strings.HasPrefix(u.Fragment, "token=") {
		return errors.New("Invalid local session file")
	}
	token := strings.TrimPrefix(u.Fragment, "token=")
	req, err := http.NewRequest(method, "http://"+cfg.Listen+"/api/sync/"+command, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-InkPika-Token", token)
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r, err := client.Do(req)
	if err != nil {
		return errors.New("Cannot connect to the local service; restart it")
	}
	defer r.Body.Close()
	if r.StatusCode < 300 {
		// Stream the full catalogue without truncating it or buffering it twice.
		_, err = io.Copy(os.Stdout, r.Body)
		return err
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		return err
	}
	if r.StatusCode >= 300 {
		var out struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &out)
		return fmt.Errorf("Sync action failed: %s", out.Error)
	}
	fmt.Println(string(data))
	return nil
}
