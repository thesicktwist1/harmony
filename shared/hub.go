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
	CreateStorage() error
}

func write(event *FileEvent) error {
	stat, err := os.Stat(event.Path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	} else {
		if stat.IsDir() {
			return ErrMalformedEvent
		}
	}
	return os.WriteFile(event.Path, event.Data, 0777)
}

func isValidPath(p string) error {
	if p == "" {
		return ErrEmptyPath
	}
	cleanPath := path.Clean(p)
	if strings.Split(cleanPath, sep)[0] != storage {
		return ErrInvalidPath
	}
	return nil
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
				return os.Mkdir(event.Path, perm)
			} else {
				return os.WriteFile(event.Path, event.Data, perm)
			}
		} else {
			return err
		}
	} else {
		return os.ErrExist
	}
}

func createStorage() error {
	info, err := os.Stat(storage)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return os.Mkdir(storage, 0777)
	} else {
		if !info.IsDir() {
			if err := os.Remove(storage); err != nil {
				return err
			}
			return os.Mkdir(storage, 0777)
		}
	}
	return nil
}
