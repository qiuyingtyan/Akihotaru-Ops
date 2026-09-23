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
	return "http://127.0.0.1:9800"
}

func (a *App) OpenExternalBrowser(targetUrl string) {
	if a.ctx != nil && targetUrl != "" {
		wailsRuntime.BrowserOpenURL(a.ctx, targetUrl)
	}
}

func (a *App) WindowMinimise() {
	if a.ctx != nil {
		wailsRuntime.WindowMinimise(a.ctx)
	}
}

func (a *App) WindowToggleMaximise() {
	if a.ctx != nil {
		wailsRuntime.WindowToggleMaximise(a.ctx)
	}
}

func (a *App) WindowClose() {
	if a.ctx != nil {
		wailsRuntime.Quit(a.ctx)
	}
}

func (a *App) IsWindowMaximised() bool {
	if a.ctx != nil {
		return wailsRuntime.WindowIsMaximised(a.ctx)
	}
	return false
}
