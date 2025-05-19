package main

import (
	"embed"
	// "io/ioutil" // Deprecated, replaced by os
	"log"
	"os"                // Added for os.ReadFile
	goruntime "runtime" // Alias for standard library runtime

	// Required for runtime.OpenDirectoryDialog wrapper if used
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu" // Import menu package
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	// Alias for Wails runtime package
)

//go:embed all:frontend/dist
var assets embed.FS

// Add this SelectDirectory method to the App struct in app.go
// Or, if you prefer to keep app.go clean of Wails runtime imports for some reason,
// you can define it here and pass `app.ctx` if needed, though it's better in app.go.
// For this example, let's assume it's added to app.go and `app.ctx` is used.
// Example of how it would look in app.go:
/*
func (a *App) SelectDirectory() (string, error) {
	selectedDirectory, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Select Project Directory",
	})
	if err != nil {
		return "", err
	}
	return selectedDirectory, nil
}
*/

func main() {
	app := NewApp() // Creates an instance of App from app.go
	// Load icons

	iconPNG, errPNG := os.ReadFile("appicon.png") // Changed from ioutil.ReadFile
	if errPNG != nil {
		log.Println("Warning: Could not load appicon.png:", errPNG)
	}

	appMenu := menu.NewMenu() // Create an empty menu

	if goruntime.GOOS == "darwin" { // Check if OS is macOS
		appMenu.Append(menu.AppMenu())  // Add standard AppMenu (Quit, About, Hide, etc.)
		appMenu.Append(menu.EditMenu()) // Add standard EditMenu (Copy, Paste, Cut, Select All)
	}

	err := wails.Run(&options.App{
		Title:  "Shotgun App",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app, // This binds all public methods of app
		},
		Menu: appMenu, // Set the application menu

		Linux: &linux.Options{
			Icon: iconPNG,
		},
	})

	if err != nil {
		log.Fatal(err)
	}
}
