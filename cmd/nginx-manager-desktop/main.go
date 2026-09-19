package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"

	desktopkit "github.com/wanstu/wails-desktop-kit"
	kitui "github.com/wanstu/wails-desktop-kit/ui"
)

//go:embed all:frontend
var embeddedFrontend embed.FS

func main() {
	launch, err := desktopkit.ParseLaunchOptions(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "nginx-manager-desktop:", err)
		os.Exit(1)
	}
	app, err := NewApp()
	if err != nil {
		fmt.Fprintln(os.Stderr, "nginx-manager-desktop:", err)
		os.Exit(1)
	}
	assets, err := fs.Sub(embeddedFrontend, "frontend")
	if err != nil {
		fmt.Fprintln(os.Stderr, "nginx-manager-desktop:", err)
		os.Exit(1)
	}
	window := desktopkit.DefaultWindowConfig()
	window.Width = 1180
	window.Height = 760
	window.MinWidth = 900
	window.MinHeight = 620

	if err := desktopkit.Run(desktopkit.Config{
		ID:                   "nginx-manager-desktop-v1",
		Title:                "Nginx Manager",
		Assets:               kitui.Mount(assets),
		Bind:                 []interface{}{app},
		Theme:                desktopkit.DefaultThemeConfig(),
		Launch:               launch,
		Window:               window,
		SingleInstance:       true,
		SecondInstancePolicy: desktopkit.SecondInstanceWakeManual,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "nginx-manager-desktop:", err)
		os.Exit(1)
	}
}
