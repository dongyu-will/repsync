package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/egoist/mygo"

	"repsync/internal/control"
	"repsync/internal/workspace"
)

func main() {
	if len(os.Args) > 1 {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		code := control.Run(ctx)
		cancel()
		os.Exit(code)
	}

	config, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	service := workspace.NewService(filepath.Join(config, "repsync", "projects.json"))
	var local *control.Server
	// MyGo runs main to generate bindings; generation must not acquire the live socket.
	if os.Getenv("MYGO_GENERATE") == "" {
		socket, err := control.SocketPath()
		if err != nil {
			log.Fatal(err)
		}
		local, err = control.Listen(socket, service)
		if err != nil {
			log.Fatal(err)
		}
		defer local.Close()
		service.StartGitCleanup()
	}
	configureUpdater()
	mygo.Bind(service)
	mygo.Bind(&Desktop{})
	mygo.App.OnQuit(func() {
		service.Shutdown()
		if local != nil {
			local.Close()
		}
	})
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title: "repsync", URL: "/", Width: 960, Height: 640,
			MinWidth: 760, MinHeight: 500,
			TitleBarStyle: mygo.TitleBarHidden, TitleBarHeight: 32,
			TrafficLightPosition: &mygo.Point{X: 16, Y: 10},
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
