package main

import (
    "embed"
    "log"

    "github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
    app := application.New(application.Options{
        Name:        "ApexVoid Photobooth",
        Description: "Standalone offline-first photobooth desktop application",
        Services: []application.Service{
            application.NewService(&DesktopService{}),
        },
        Assets: application.AssetOptions{
            Handler: application.AssetFileServerFS(assets),
        },
        Mac: application.MacOptions{
            ApplicationShouldTerminateAfterLastWindowClosed: true,
        },
    })

    app.Window.NewWithOptions(application.WebviewWindowOptions{
        Title:            "ApexVoid Photobooth — Desktop Preview",
        Width:            1280,
        Height:           800,
        MinWidth:         900,
        MinHeight:        620,
        BackgroundColour: application.NewRGB(13, 10, 23),
        URL:              "/",
    })

    if err := app.Run(); err != nil {
        log.Fatal(err)
    }
}
