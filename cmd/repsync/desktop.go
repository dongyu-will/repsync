package main

import "github.com/egoist/mygo"

// Desktop provides the system directory picker for project registration and cloning.
type Desktop struct{}

func (*Desktop) ChooseDirectory(defaultPath, title, buttonLabel string) (string, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: mygo.FocusedWindow(), Title: title, ButtonLabel: buttonLabel,
		DefaultPath: defaultPath, Directory: true, CreateDirectories: true,
	})
	if err != nil || len(paths) == 0 {
		return "", err
	}
	return paths[0], nil
}
