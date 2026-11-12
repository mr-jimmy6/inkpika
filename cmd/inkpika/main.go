package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/mr-jimmy6/inkpika/internal/buildinfo"
	"github.com/mr-jimmy6/inkpika/internal/config"
	"github.com/mr-jimmy6/inkpika/internal/download"
	"github.com/mr-jimmy6/inkpika/internal/providers"
	"github.com/mr-jimmy6/inkpika/internal/statefile"
	"github.com/mr-jimmy6/inkpika/internal/syncer"
	"github.com/mr-jimmy6/inkpika/internal/transfer"
	"github.com/mr-jimmy6/inkpika/internal/web"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	filename := flag.String("config", "config.json", "Configuration file path")
	check := flag.Bool("check", false, "Validate configuration and try listing cloud files")
	version := flag.Bool("version", false, "Show version")
	stateDir := flag.Bool("state-dir", false, "Print the configured state directory for KUAL scripts")
	syncCommand := flag.String("sync", "", "KUAL sync command: refresh/scan/start/status/cancel")
	menu := flag.String("kual-menu", "", "Dynamic KUAL menu.json path")
	menuAction := flag.String("menu-action", "", "Offline menu action: refresh/check, without starting the service")
	scanOnly := flag.Bool("scan-once", false, "Check remote and local files, then exit without starting the service")
	transferMode := flag.Bool("transfer", false, "Start the independent wireless transfer service")
	transferConfig := flag.String("transfer-config", "", "Optional wireless transfer configuration path")
	transferState := flag.Bool("transfer-state-dir", false, "Print the wireless transfer state directory")
	settingsMode := flag.Bool("settings", false, "Start the independent settings service")
	settingsState := flag.Bool("settings-state-dir", false, "Print the settings state directory")
	flag.Parse()
	if *version {
		fmt.Printf("InkPika %s\nPackaged at: %s\n", buildinfo.Version(), buildinfo.PackagedAt())
		return nil
	}
	debug.SetMemoryLimit(48 * 1024 * 1024)
	if *settingsMode || *settingsState {
		c := settingsConfig(*filename)
		if *settingsState {
			fmt.Println(c.StateDir)
			return nil
		}
		return runLocalWeb(c, *filename, *menu, "settings")
	}
	if *transferMode || *transferState {
		if *transferConfig == "" {
			*transferConfig = filepath.Join(filepath.Dir(*filename), "transfer.json")
		}
		c, err := transfer.Load(*transferConfig)
		if err != nil {
			return err
		}
		if *transferState {
			fmt.Println(c.StateDir)
			return nil
		}
		c.FormatsFile = *filename
		return runLocalWeb(c, *filename, *menu, "transfer")
	}
	if *menuAction != "" {
		return menuControl(*filename, *menu, *menuAction)
	}
	cfg, err := config.Load(*filename)
	if err != nil {
		return err
	}
	if *stateDir {
		fmt.Println(cfg.StateDir)
		return nil
	}
	if *syncCommand != "" {
		return syncControl(cfg, *syncCommand)
	}
	client, err := providers.New(cfg)
	if err != nil {
		return err
	}
	if *check {
		out, err := client.List(context.Background(), cfg.Prefix, "")
		if err != nil {
			return err
		}
		fmt.Printf("Connected: %s / %s, %d files and %d folders on this page\n", cfg.ProviderLabel(), cfg.Bucket, len(out.Keys), len(out.Folders))
		return nil
	}
	if err = os.MkdirAll(cfg.StateDir, 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(cfg.StateDir, "server.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("A client is already running for this configuration: %w", err)
	}
	manager, err := download.New(cfg, client)
	if err != nil {
		return err
	}
	defer manager.Close()
	syncEngine, err := syncer.New(cfg, client, manager, *menu)
	if err != nil {
		return err
	}
	defer syncEngine.Close()
	if *scanOnly {
		if err = syncEngine.Start(false); err != nil {
			return err
		}
		syncEngine.Wait()
		if s := syncEngine.Snapshot(); s.Error != "" {
			return fmt.Errorf("%s", s.Error)
		}
		return nil
	}
	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("Cannot start local service: %w", err)
	}
	defer listener.Close()
	tokenBytes := make([]byte, 32)
	if _, err = rand.Read(tokenBytes); err != nil {
		return err
	}
	token := hex.EncodeToString(tokenBytes)
	launchPath := filepath.Join(cfg.StateDir, "launch.url")
	if err = statefile.Write(launchPath, []byte("http://"+cfg.BrowserAddress()+"/#token="+token+"\n")); err != nil {
		return err
	}
	defer os.Remove(launchPath)
	pidPath := filepath.Join(cfg.StateDir, "server.pid")
	if err = statefile.Write(pidPath, []byte(fmt.Sprint(os.Getpid()))); err != nil {
		return err
	}
	defer os.Remove(pidPath)
	if *menu != "" {
		if err = syncer.RefreshMenu(*menu, cfg, ""); err != nil {
			return err
		}
		defer func() {
			syncEngine.Close()
			os.Remove(pidPath)
			_ = syncer.RefreshMenu(*menu, cfg, "")
		}()
	}
	server := &http.Server{Handler: web.New(cfg, client, manager, token, syncEngine), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	finished := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdownCtx)
		case <-finished:
		}
	}()
	log.Printf("Cloud library started; storage backend %s, listening on %s", cfg.ProviderLabel(), cfg.Listen)
	err = server.Serve(listener)
	close(finished)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
