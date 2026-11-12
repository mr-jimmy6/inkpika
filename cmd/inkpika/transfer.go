package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/mr-jimmy6/inkpika/internal/settings"
	"github.com/mr-jimmy6/inkpika/internal/statefile"
	"github.com/mr-jimmy6/inkpika/internal/transfer"
)

func settingsConfig(filename string) transfer.Config {
	full, _ := filepath.Abs(filename)
	return transfer.Config{Port: 8791, UIPort: 8792, StateDir: filepath.Join(filepath.Dir(full), "settings-state")}
}

func runLocalWeb(c transfer.Config, cloudConfig, menu, mode string) error {
	label := "Wireless transfer"
	if mode == "settings" {
		label = "Settings"
	}
	if err := os.MkdirAll(c.StateDir, 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(c.StateDir, "server.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("%s is already running: %w", label, err)
	}
	if mode == "transfer" {
		if err = os.MkdirAll(c.DownloadDir, 0755); err != nil {
			return err
		}
		// Remove only this service's unfinished streams after acquiring its lock.
		stale, _ := filepath.Glob(filepath.Join(c.DownloadDir, ".wireless-upload-*"))
		for _, name := range stale {
			_ = os.Remove(name)
		}
	}
	local, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", c.UIPort))
	if err != nil {
		return fmt.Errorf("%s loopback page port is unavailable: %w", label, err)
	}
	defer local.Close()
	lan, err := net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%d", c.Port))
	if err != nil {
		return fmt.Errorf("%s LAN port is unavailable: %w", label, err)
	}
	defer lan.Close()
	chain := transferChain
	if mode == "settings" {
		chain = "INKPIKA_SETTINGS"
	}
	clearFirewall, err := localWebFirewall(c.Port, chain)
	if err != nil {
		return err
	}
	defer clearFirewall()
	var s *transfer.Server
	if mode == "settings" {
		s, err = settings.New(c, cloudConfig, settings.Hooks{BeforeSave: func() (func() string, error) {
			app := filepath.Dir(cloudConfig)
			running := exec.Command("sh", filepath.Join(app, "service.sh"), "status").Run() == nil
			if running {
				if e := exec.Command("sh", filepath.Join(app, "stop.sh")).Run(); e != nil {
					return nil, e
				}
			}
			return func() string {
				if running {
					if exec.Command("sh", filepath.Join(app, "service.sh")).Run() != nil {
						return "Cloud library could not restart; check KUAL and retry."
					}
					return "Cloud library restarted with the saved configuration; resume unfinished downloads manually."
				}
				_ = exec.Command("sh", filepath.Join(app, "menu.sh"), "refresh").Run()
				return ""
			}, nil
		}})
	} else {
		s, err = transfer.New(c)
	}
	if err != nil {
		return err
	}
	pidPath, launchPath := filepath.Join(c.StateDir, "server.pid"), filepath.Join(c.StateDir, "launch.url")
	if err = statefile.Write(pidPath, []byte(strconv.Itoa(os.Getpid()))); err != nil {
		return err
	}
	defer os.Remove(pidPath)
	if err = statefile.Write(launchPath, []byte(s.LaunchURL()+"\n")); err != nil {
		return err
	}
	defer os.Remove(launchPath)
	if menu != "" {
		if err = menuControl(cloudConfig, menu, "refresh"); err != nil {
			return err
		}
		defer func() { os.Remove(pidPath); _ = menuControl(cloudConfig, menu, "refresh") }()
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	makeServer := func(handler http.Handler) *http.Server {
		return &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 20 * time.Minute, WriteTimeout: 20 * time.Minute, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 * 1024, BaseContext: func(net.Listener) context.Context { return ctx }}
	}
	localServer, lanServer := makeServer(s.LocalHandler()), makeServer(s.LANHandler())
	finished := make(chan error, 2)
	go func() { finished <- localServer.Serve(local) }()
	go func() { finished <- lanServer.Serve(lan) }()
	select {
	case <-ctx.Done():
	case err = <-finished:
	}
	cancel()
	var wg sync.WaitGroup
	for _, server := range []*http.Server{localServer, lanServer} {
		wg.Add(1)
		go func(server *http.Server) {
			defer wg.Done()
			stopCtx, stop := context.WithTimeout(context.Background(), 2*time.Second)
			defer stop()
			if server.Shutdown(stopCtx) != nil {
				_ = server.Close()
			}
		}(server)
	}
	wg.Wait()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
