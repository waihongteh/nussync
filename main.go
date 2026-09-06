package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIconPNG []byte

//go:embed build/windows/icon.ico
var appIconICO []byte

const cliHelp = `NUSSync - Canvas LMS desktop sync

Usage:
  nussync                 start the desktop app
  nussync --sync          run one sync to stdout and exit
  nussync --check         print upcoming deadlines and exit
  nussync --pair          wait for /start on Telegram and save the chat id
  nussync --notify-test   send a Telegram test message
  nussync --papers-test "<query>" [--digest-send]
                          search arXiv + Semantic Scholar, build today's
                          digest and download one PDF
`

func main() {
	// CLI modes run headless and exit; useful for testing without the GUI.
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--sync", "--check", "--notify-test", "--pair", "--study-test", "--papers-test":
			os.Exit(runCLI(arg))
		case "-h", "--help":
			fmt.Print(cliHelp)
			return
		}
	}

	app := NewApp()

	err := wails.Run(&options.App{
		Title:     "NUSSync",
		Width:     1200,
		Height:    780,
		MinWidth:  900,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
			// Intercepts /local/... for the in-app file viewer and passes
			// everything else through; see assets.go.
			Middleware: LocalAssetMiddleware(app),
		},
		BackgroundColour:  &options.RGBA{R: 15, G: 17, B: 21, A: 1},
		HideWindowOnClose: true,
		OnStartup: func(ctx context.Context) {
			app.startup(ctx)
			startTray(app)
		},
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			Theme:                windows.SystemDefault,
		},
	})
	if err != nil {
		log.Printf("nussync: %v", err)
	}
}
