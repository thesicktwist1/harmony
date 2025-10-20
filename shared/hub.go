package shared

import (
	"context"
	"errors"
	"os"
	"path"
)

type Hub interface {
	Process(context.Context, *FileEvent) error
}

func write(event *FileEvent) error {
	if _, err := os.Stat(event.Path); err != nil {
		return err
	}
	return os.WriteFile(event.Path, event.Data, 0777)
}

func create(event *FileEvent) error {
	if stat, err := os.Stat(path.Dir(event.Path)); err == nil {
		if !stat.IsDir() {
			return ErrInvalidDest
		}
	} else {
		return err
	}
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
				file.Close()
			}
		} else {
			return err
		}
	} else {
		return os.ErrExist
	}
	return nil
}
