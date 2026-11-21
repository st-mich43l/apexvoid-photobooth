// Command photobooth is the ApexVoid Photobooth desktop application: a thin
// Wails 3 shell around internal/app. It needs no server, Enterprise
// installation, Docker or external database and makes no network connections.
package main

import (
	"embed"
	"io/fs"
	"log"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/st-mich43l/apexvoid-photobooth/internal/app"
)

//go:embed all:frontend_dist
var assets embed.FS

// version is set at build time: -ldflags "-X main.version=..."
var version = "0.2.0-dev"

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "--selftest" {
		os.Exit(app.SelfTest(version, os.Args[2:]))
	}
	var wapp *application.App

	svc, err := app.New(app.Options{
		Version: version,
		Emit: func(name string, v any) {
			if wapp != nil {
				wapp.Event.Emit(name, v)
			}
		},
		PickFolder: func() (string, error) {
			return wapp.Dialog.OpenFile().
				CanChooseDirectories(true).CanChooseFiles(false).CanCreateDirectories(true).
				SetTitle("Choose media folder").PromptForSingleSelection()
		},
	})
	if err != nil {
		log.Fatalf("ApexVoid Photobooth could not start: %v", err)
	}

	dist, err := fs.Sub(assets, "frontend_dist")
	if err != nil {
		log.Fatal(err)
	}
	wapp = application.New(application.Options{
		Name:        "ApexVoid Photobooth",
		Description: "Camera-operated photobooth software",
		Services:    []application.Service{application.NewService(svc)},
		Assets:      application.AssetOptions{Handler: application.AssetFileServerFS(dist)},
		// two instances would fight over the same camera
		SingleInstance: &application.SingleInstanceOptions{UniqueID: "net.apexvoid.photobooth"},
		// release the camera, SDK and database before the process ends
		OnShutdown: func() {
			if err := svc.Shutdown(); err != nil {
				log.Printf("shutdown: %v", err)
			}
		},
		Mac: application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
	})
	wapp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "main",
		Title:            "ApexVoid Photobooth",
		Width:            1360,
		Height:           860,
		MinWidth:         900,
		MinHeight:        620,
		BackgroundColour: application.NewRGB(15, 11, 26),
		URL:              "/",
	})
	if err := wapp.Run(); err != nil {
		log.Fatal(err)
	}
}
