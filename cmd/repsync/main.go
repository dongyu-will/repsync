package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"

	"repsync/internal/workspace"
)

func main() {
	config, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	mygo.Bind(workspace.NewService(filepath.Join(config, "repsync", "projects.json")))
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title: "repsync", URL: "/", Width: 960, Height: 640,
			MinWidth: 760, MinHeight: 500,
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
