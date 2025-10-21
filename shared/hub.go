package shared

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type Hub interface {
	Process(context.Context, *FileEvent) error
}

func write(event *FileEvent) error {
	stat, err := os.Stat(event.Path)
	if err != nil {
		return err
	}
	if stat.IsDir() {
		return ErrMalformedEvent
	}
	return os.WriteFile(event.Path, event.Data, 0777)
}

func rename(event *FileEvent) error {
	if event.NewPath == "" {
		return ErrEmptyPath
	}
	rel, err := filepath.Rel(event.Path, event.NewPath)
	if err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
		return ErrInvalidDest
	}
	stat, err := os.Stat(event.Path)
	if err != nil {
		return err
	} else {
		if stat.IsDir() != event.IsDir {
			return ErrMalformedEvent
		}
	}
	stat, err = os.Stat(path.Dir(event.NewPath))
	if err != nil {
		return err
	}
	if !stat.IsDir() {
		return ErrInvalidDest
	}
	_, err = os.Stat(event.NewPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	} else {
		return os.ErrExist
	}
	return nil
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
				if _, err := os.Create(event.Path); err != nil {
					return err
				}
			}
		} else {
			return err
		}
	} else {
		return os.ErrExist
	}
	return nil
}
