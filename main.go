package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

// Widget size. The window grows to expandedHeight when "Show additional
// details" is open.
const (
	widgetWidth     = 400
	collapsedHeight = 612
	expandedHeight  = 900
)

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:            "Exo",
		Width:            widgetWidth,
		Height:           collapsedHeight,
		MinWidth:         widgetWidth,
		MinHeight:        collapsedHeight,
		DisableResize:    true,
		Frameless:        true,
		BackgroundColour: &options.RGBA{R: 12, G: 8, B: 21, A: 255}, // matches the app's base colour
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		Bind:             []interface{}{app},
		Windows: &windows.Options{
			Theme:                windows.Dark,
			WebviewGpuIsDisabled: true,
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
