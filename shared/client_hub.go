package shared

import (
	"context"
	"errors"
	"os"
	"path"

	"github.com/fsnotify/fsnotify"
)

type clientHub struct {
	storage string
}

func NewClientHub(storage string) clientHub {
	return clientHub{
		storage: storage,
	}
}

func (c clientHub) Process(ctx context.Context, event *FileEvent) error {
	if event.Path == "" {
		return EventError{err: ErrEmptyPath, data: event}
	}
	switch event.Op {
	case fsnotify.Create.String():
		return create(event)
	case fsnotify.Remove.String():
		if err := os.RemoveAll(event.Path); err != nil {
			return EventError{err: err, data: event}
		}
	case fsnotify.Rename.String():
		if event.NewPath == "" {
			return EventError{err: ErrEmptyPath, data: event}
		}
		stat, err := os.Stat(event.Path)
		if err != nil {
			return EventError{err: err, data: event}
		}
		if stat.IsDir() != event.IsDir {
			return EventError{err: err, data: event}
		}
		stat, err = os.Stat(path.Dir(event.NewPath))
		if err != nil {
			return EventError{err: err, data: event}
		}
		if !stat.IsDir() {
			return EventError{err: ErrInvalidPath, data: event.NewPath}
		}
		if _, err := os.Stat(event.NewPath); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return EventError{err: err}
			}
		} else {
			return EventError{err: os.ErrExist, data: event.NewPath}
		}
		if err := os.Rename(event.Path, event.NewPath); err != nil {
			return EventError{err: err, data: event}
		}
	case fsnotify.Write.String():
		return write(event)
	default:
		return EventError{err: ErrUnsupportedEvent, data: event}
	}
	return nil
}
