package main

import (
	"context"
	"errors"

	"github.com/mr-jimmy6/inkpika/internal/config"
	"github.com/mr-jimmy6/inkpika/internal/diagnostics"
	"github.com/mr-jimmy6/inkpika/internal/providers"
	"github.com/mr-jimmy6/inkpika/internal/syncer"
)

func menuControl(configFile, menu, action string) error {
	if menu == "" || (action != "refresh" && action != "check") {
		return errors.New("Menu action requires -kual-menu and refresh/check")
	}
	c, err := config.Load(configFile)
	if err != nil {
		return syncer.RefreshMenu(menu, c, diagnostics.Redact(c, err.Error()))
	}
	if action == "refresh" {
		return syncer.RefreshMenu(menu, c, "")
	}
	store, err := providers.New(c)
	if err != nil {
		return err
	}
	if _, err = diagnostics.Run(context.Background(), c, store, diagnostics.Probe); err != nil && !errors.Is(err, diagnostics.ErrRunning) {
		return err
	}
	return syncer.RefreshMenu(menu, c, "")
}
