// Command photobooth is the ApexVoid Photobooth desktop application: a thin
// Wails 3 shell around internal/app. It needs no server, Enterprise
// installation, Docker or external database and makes no network connections.
package main

import (
	"embed"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/st-mich43l/apexvoid-photobooth/internal/app"
	"github.com/st-mich43l/apexvoid-photobooth/internal/settings"
	"github.com/st-mich43l/apexvoid-photobooth/internal/storage"
)

//go:embed all:frontend_dist
var assets embed.FS

// version is set at build time: -ldflags "-X main.version=..."
var version = "0.2.0-dev"

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "--notices" {
		if len(os.Args) != 3 {
			startupError("Usage: ApexVoidPhotobooth.exe --notices <new-file>")
			os.Exit(1)
		}
		if err := app.WriteThirdPartyNotices(os.Args[2]); err != nil {
			startupError("Could not write notices: " + err.Error())
			os.Exit(1)
		}
		return
	}
	if len(os.Args) >= 2 && os.Args[1] == "--selftest" {
		os.Exit(app.SelfTest(version, os.Args[2:]))
	}
	if len(os.Args) >= 2 && os.Args[1] == "--probe-cameras" {
		os.Exit(app.ProbeCameras(version, os.Args[2:]))
	}
	if len(os.Args) >= 2 && os.Args[1] == "--canon-validate" {
		os.Exit(app.CanonValidate(version, os.Args[2:]))
	}
	if len(os.Args) >= 2 && os.Args[1] == "--print-selftest" {
		os.Exit(app.PrintSelfTest(version, os.Args[2:]))
	}
	if err := checkWebViewRuntime(); err != nil {
		startupError(err.Error())
		os.Exit(1)
	}

	// Claim the user-level application instance before loading settings, opening
	// a library, or touching any devices. The library lock also protects shared
	// libraries opened from other user accounts or configurations.
	instance, err := storage.AcquireOwnership(filepath.Join(filepath.Dir(settings.DefaultPath()), ".instance"))
	if err != nil {
		startupError("ApexVoid Photobooth could not start. Another instance may already be running: " + err.Error())
		os.Exit(1)
	}
	defer instance.Close()
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
		startupError("ApexVoid Photobooth could not start: " + err.Error())
		_ = instance.Close()
		os.Exit(1)
	}

	defer svc.Shutdown()
	dist, err := fs.Sub(assets, "frontend_dist")
	if err != nil {
		log.Print(err)
		return
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
	// Windows uses COLORREF values (0x00BBGGRR) for native title bar colours.
	barColour, textColour, borderColour := uint32(0x001a0b0f), uint32(0x00faeff3), uint32(0x0050242f)
	windowTheme := &application.WindowTheme{TitleBarColour: &barColour, TitleTextColour: &textColour, BorderColour: &borderColour}
	wapp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:       "main",
		Title:      "ApexVoid Photobooth",
		Width:      1360,
		Height:     860,
		StartState: application.WindowStateMaximised,
		Windows:    application.WindowsWindow{Theme: application.Dark, CustomTheme: application.ThemeSettings{DarkModeActive: windowTheme, DarkModeInactive: windowTheme}},
		// Keep the window usable at 200% scaling on compact 1366×768 displays.
		MinWidth:         640,
		MinHeight:        360,
		BackgroundColour: application.NewRGB(15, 11, 26),
		URL:              "/",
	})
	if err := wapp.Run(); err != nil {
		log.Print(err)
		return
	}
}
