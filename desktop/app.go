package main

import (
	"context"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx context.Context
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) GetDefaultServerUrl() string {
	return "http://192.168.1.19:9800"
}

func (a *App) OpenExternalBrowser(targetUrl string) {
	if a.ctx != nil && targetUrl != "" {
		wailsRuntime.BrowserOpenURL(a.ctx, targetUrl)
	}
}

func (a *App) ToggleFullscreen() {
	if a.ctx != nil {
		wailsRuntime.WindowToggleMaximise(a.ctx)
	}
}
