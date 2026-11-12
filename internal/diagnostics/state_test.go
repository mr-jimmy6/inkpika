package diagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mr-jimmy6/inkpika/internal/config"
)

func TestRunSingleFlightCancellationAndSavedResult(t *testing.T) {
	c := config.Config{StateDir: t.TempDir(), Provider: config.AliyunOSS, Bucket: "books", Prefix: "allowed/"}
	started := make(chan struct{})
	finished := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	probe := func(ctx context.Context, _ config.Config) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}
	go func() {
		_, err := Run(ctx, c, &testStore{}, probe)
		finished <- err
	}()
	<-started
	if r, err := Read(c); err != nil || !r.Running {
		t.Fatal("active check was not retained", r, err)
	}
	if _, err := Run(ctx, c, &testStore{}, probe); !errors.Is(err, ErrRunning) {
		t.Fatal("concurrent check accepted", err)
	}
	cancel()
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if r, err := Read(c); err != nil || r.Running || r.Network != "Failed" || !strings.Contains(r.Detail, "canceled") {
		t.Fatal("cancelled check was not saved", r, err)
	}
	if _, err := Run(context.Background(), c, &testStore{}, func(context.Context, config.Config) error { return nil }); err != nil {
		t.Fatal("lock was not released", err)
	}
	if r, err := Read(c); err != nil || r.Read != "Passed (sample)" {
		t.Fatal(r, err)
	}
}

func TestReadDetectsInterruptedAndDifferentSource(t *testing.T) {
	c := config.Config{StateDir: t.TempDir(), Bucket: "books"}
	r := Result{Source: c.SourceIdentity(), Updated: 123, Running: true}
	b, _ := json.Marshal(r)
	if err := os.WriteFile(filepath.Join(c.StateDir, "connection-check.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	if result, err := Read(c); err != nil || result.Running || !strings.Contains(result.Detail, "interrupted") {
		t.Fatal(result, err)
	}
	c.Bucket = "different"
	if result, err := Read(c); err != nil || result.Updated != 0 {
		t.Fatal("another source's result exposed", result, err)
	}
}
