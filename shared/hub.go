package shared

import (
	"context"
	"errors"
	"os"
)

type Hub interface {
	Process(context.Context, *FileEvent) error
}

func write(event *FileEvent) error {
	if _, err := os.Stat(event.Path); err != nil {
		return err
	}
	return os.WriteFile(event.Path, event.Data, 0750)
}

func remove(event *FileEvent) error {
	if _, err := os.Stat(event.Path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		} else {
			return err
		}
	}
	if err := os.RemoveAll(event.Path); err != nil {
		return err
	}
	return nil
}

func create(event *FileEvent) error {
	if _, err := os.Stat(event.Path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if event.IsDir {
				if err := os.Mkdir(event.Path, perm); err != nil {
					return err
				}
			} else {
				file, err := os.Create(event.Path)
				if err != nil {
					return err
				}
				defer file.Close()
				if _, err := file.Write(event.Data); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
