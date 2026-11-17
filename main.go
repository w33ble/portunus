package main

import (
	"embed"
	"fmt"
	"log"
	"log/slog"
	"os"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/w33ble/portunus/internal/appdirs"
	"github.com/w33ble/portunus/internal/profiles"
	"github.com/w33ble/portunus/internal/services"
	"github.com/w33ble/portunus/internal/state"
	"github.com/w33ble/portunus/internal/swipe"
	"github.com/w33ble/portunus/internal/version"
)

//go:embed all:frontend/dist
var assets embed.FS

const appID = "com.w33ble.portunus"

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		v := version.Get()
		fmt.Printf("portunus %s (commit %s, built %s, %s)\n", v.Version, v.Commit, v.Date, v.Go)
		return
	}

	configDir, err := appdirs.ConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	stateDir, err := appdirs.StateDir()
	if err != nil {
		log.Fatal(err)
	}
	windowState, err := state.Open(stateDir)
	if err != nil {
		log.Fatal(err)
	}

	// app is assigned before any event can fire: services only emit after Run starts.
	var app *application.App
	emit := func(name string, data any) { app.Event.Emit(name, data) }

	// The native background shows before the page paints and while resizing, so it follows the theme.
	var mainWindow atomic.Pointer[application.WebviewWindow]
	paintWindow := func(id string) {
		if win := mainWindow.Load(); win != nil {
			win.SetBackgroundColour(windowColor(id, app.Env.IsDarkMode()))
		}
	}

	core, err := services.New(services.Config{
		ConfigDir:    configDir,
		Emit:         emit,
		Secrets:      profiles.WithFallback(profiles.Keyring(appID)),
		Logger:       slog.Default(),
		ThemeChanged: paintWindow,
	})
	if err != nil {
		log.Fatal(err)
	}

	app = application.New(application.Options{
		Name:         "Portunus",
		Description:  "SFTP and S3 file transfer client\nVersion " + version.Get().Version,
		Services:     core.Services(),
		MarshalError: services.MarshalError,
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		OnShutdown: core.Close,
	})

	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		saved := core.Theme()
		win := openMainWindow(app, windowState, application.WebviewWindowOptions{
			Title:          "Portunus",
			MinWidth:       minWidth,
			MinHeight:      minHeight,
			EnableFileDrop: true,
			Mac: application.MacWindow{
				InvisibleTitleBarHeight: 38,
				Backdrop:                application.MacBackdropTranslucent,
				TitleBar:                application.MacTitleBarHiddenInset,
				Appearance:              macAppearance(saved),
			},
			BackgroundColour: windowColor(saved, app.Env.IsDarkMode()),
			URL:              "/",
		})
		mainWindow.Store(win)
		app.Event.OnApplicationEvent(events.Common.ThemeChanged, func(*application.ApplicationEvent) {
			paintWindow(core.Theme())
		})

		win.OnWindowEvent(events.Common.WindowFilesDropped, func(e *application.WindowEvent) {
			var attrs map[string]string
			if target := e.Context().DropTargetDetails(); target != nil {
				attrs = target.Attributes
			}
			core.HandleDrop(e.Context().DroppedFiles(), attrs)
		})

		swipe.Watch(core.HandleSwipe)
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
