package diagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mr-jimmy6/inkpika/internal/config"
	"github.com/mr-jimmy6/inkpika/internal/statefile"
	"github.com/mr-jimmy6/inkpika/internal/storage"
)

var ErrRunning = errors.New("Connection test in progress; refresh results shortly")

// Run shares a process-independent lock with the CLI and bounds the whole check.
func Run(ctx context.Context, c config.Config, store storage.Store, probe func(context.Context, config.Config) error) (Result, error) {
	if err := os.MkdirAll(c.StateDir, 0700); err != nil {
		return Result{}, err
	}
	f, err := os.OpenFile(filepath.Join(c.StateDir, "connection-check.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return Result{}, err
	}
	defer f.Close()
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if err == syscall.EWOULDBLOCK {
			return Result{}, ErrRunning
		}
		return Result{}, err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	save := func(r Result) error {
		b, err := json.Marshal(r)
		if err != nil {
			return err
		}
		return statefile.Write(filepath.Join(c.StateDir, "connection-check.json"), b)
	}
	r := Result{Source: c.SourceIdentity(), Running: true, Updated: time.Now().Unix()}
	if err = save(r); err != nil {
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	r = Check(ctx, c, store, probe)
	return r, save(r)
}

// Read never contacts the provider. Read under the shared lock when idle, so a
// completed run cannot be mistaken for an interrupted one using an old snapshot.
func Read(c config.Config) (Result, error) {
	active := false
	f, err := os.OpenFile(filepath.Join(c.StateDir, "connection-check.lock"), os.O_RDONLY, 0600)
	if err == nil {
		defer f.Close()
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB)
		if err == nil {
			defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		} else if err == syscall.EWOULDBLOCK {
			active = true
		} else {
			return Result{}, err
		}
	} else if !os.IsNotExist(err) {
		return Result{}, err
	}
	b, err := os.ReadFile(filepath.Join(c.StateDir, "connection-check.json"))
	if os.IsNotExist(err) {
		return Result{}, nil
	}
	if err != nil {
		return Result{}, err
	}
	var r Result
	if err = json.Unmarshal(b, &r); err != nil {
		return Result{}, err
	}
	if r.Source != c.SourceIdentity() {
		return Result{}, nil
	}
	if r.Running && !active {
		r.Running = false
		r.Network, r.Access, r.Read = "Incomplete", "Not checked", "Not verified"
		r.Detail = "Previous test was interrupted; run the test again"
	}
	return r, nil
}
