package main

import (
	"embed"
	"log"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/w33ble/portunus/internal/profiles"
	"github.com/w33ble/portunus/internal/services"
)

//go:embed all:frontend/dist
var assets embed.FS

const appID = "com.w33ble.portunus"

func main() {
	configRoot, err := os.UserConfigDir()
	if err != nil {
		log.Fatalf("locate config dir: %v", err)
	}

	// app is assigned before any event can fire: services only emit after Run starts.
	var app *application.App
	emit := func(name string, data any) { app.Event.Emit(name, data) }

	core, err := services.New(services.Config{
		ConfigDir: filepath.Join(configRoot, "Portunus"),
		Emit:      emit,
		Secrets:   profiles.WithFallback(profiles.Keyring(appID)),
		Logger:    slog.Default(),
	})
	if err != nil {
		log.Fatal(err)
	}

	app = application.New(application.Options{
		Name:         "Portunus",
		Description:  "SFTP and S3 file transfer client",
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

	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:          "Portunus",
		Width:          1280,
		Height:         800,
		MinWidth:       900,
		MinHeight:      560,
		EnableFileDrop: true,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 38,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(24, 24, 27),
		URL:              "/",
	})

	win.OnWindowEvent(events.Common.WindowFilesDropped, func(e *application.WindowEvent) {
		var attrs map[string]string
		if target := e.Context().DropTargetDetails(); target != nil {
			attrs = target.Attributes
		}
		core.HandleDrop(e.Context().DroppedFiles(), attrs)
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
