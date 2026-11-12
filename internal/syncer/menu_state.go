package syncer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/mr-jimmy6/inkpika/internal/config"
)

// RefreshMenu reads persisted state, without starting the service or a scan.
// Serialize menu writers, then read the latest snapshots rather than publishing
// an older snapshot captured before a concurrent sync update.
func RefreshMenu(filename string, c config.Config, configError string) error {
	lock, err := os.OpenFile(filename+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	s := Snapshot{Phase: "idle"}
	opt := MenuOptions{Language: c.Language}
	if b, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "settings-state", "server.pid")); err == nil {
		pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
		if pid > 0 {
			exe, _ := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "exe"))
			args, _ := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
			if exe == filepath.Join(filepath.Dir(filename), "bin/inkpika") && strings.Contains(string(args), "\x00-settings\x00") {
				opt.SettingsPID = pid
			}
		}
	}
	if b, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "transfer-state", "server.pid")); err == nil {
		pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
		if pid > 0 {
			exe, _ := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "exe"))
			args, _ := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
			if exe == filepath.Join(filepath.Dir(filename), "bin/inkpika") && strings.Contains(string(args), "\x00-transfer\x00") {
				opt.TransferPID = pid
			}
		}
	}
	if configError != "" {
		s.Phase, s.Error = "error", "Configuration error: "+configError
	}
	if configError == "" {
		var disk diskState
		if b, err := os.ReadFile(filepath.Join(c.StateDir, "sync.json")); err == nil {
			if err = json.Unmarshal(b, &disk); err != nil {
				s.Error = "Cannot read sync state; check sync.json"
			} else if disk.Source == c.SourceIdentity() {
				s = disk.Snapshot
			}
		}
		if b, err := os.ReadFile(filepath.Join(c.StateDir, "server.pid")); err == nil {
			pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
			if pid > 0 {
				exe, _ := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "exe"))
				args, _ := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
				if exe == filepath.Join(filepath.Dir(filename), "bin/inkpika") && strings.Contains(string(args), "\x00-config\x00") && !strings.Contains(string(args), "\x00-transfer\x00") && !strings.Contains(string(args), "\x00-settings\x00") {
					opt.ServicePID = pid
				}
			}
		}

	}
	if opt.ServicePID == 0 {
		if s.Running {
			s.Phase = "idle"
		}
		s.Running = false
		s.Current = ""
		for i := range s.Books {
			if s.Books[i].Status == "downloading" {
				s.Books[i].Status = "pending"
			}
		}
	}
	return WriteMenu(filename, s, opt)
}
