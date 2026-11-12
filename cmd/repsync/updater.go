package main

import (
	"errors"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
)

func configureUpdater() {
	mygo.App.SetLanguages("en", "zh-CN")
	mygo.Use(updater.Plugin)
	mygo.App.WhenReady(func() {
		mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
			{Label: "repsync", Submenu: []*mygo.MenuItem{
				{Role: mygo.RoleAbout},
				updater.MenuItem(),
				mygo.Separator(),
				{Role: mygo.RoleHide},
				{Role: mygo.RoleHideOthers},
				{Role: mygo.RoleUnhide},
				mygo.Separator(),
				{Role: mygo.RoleQuit},
			}},
			{Role: mygo.RoleFileMenu},
			{Role: mygo.RoleEditMenu},
			{Role: mygo.RoleViewMenu},
			{Role: mygo.RoleWindowMenu},
		}))
	})
}

// CheckForUpdates opens MyGo's signed updater; development builds cannot install updates.
func (*Desktop) CheckForUpdates(language string) error {
	if language != "en" && language != "zh-CN" {
		return errors.New("unsupported update window language")
	}
	mygo.RunOnMain(func() {
		mygo.App.SetLocale(language)
		updater.CheckForUpdates()
	})
	return nil
}
