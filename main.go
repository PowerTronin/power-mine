package main

import (
	"context"
	"embed"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if len(os.Args) > 1 && os.Args[1] == "headless" {
		os.Exit(runHeadless(context.Background(), os.Args[2:]))
	}

	// Create an instance of the app structure
	app := NewApp()
	app.logsWindow = isLogsWindowArgs(os.Args[1:])
	title := "Power Mine"
	width := 1180
	height := 760
	if app.logsWindow {
		title = "Power Mine Logs"
		width = 1040
		height = 720
	}

	// Create application with options
	err := wails.Run(&options.App{
		Title:  title,
		Width:  width,
		Height: height,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}

func isLogsWindowArgs(args []string) bool {
	for _, arg := range args {
		if arg == "--logs-window" {
			return true
		}
	}
	return false
}
