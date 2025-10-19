package shared

import (
	"context"
	"errors"
	"os"

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
		return remove(event)
	case fsnotify.Rename.String():
		if event.NewPath == "" {
			return EventError{err: ErrEmptyPath, data: event}
		}
		if _, err := os.Stat(event.Path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return EventError{err: err, data: event}
			}
			return err
		}
		if _, err := os.Stat(event.NewPath); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return EventError{err: err}
			}
		} else {
			return EventError{err: os.ErrExist, data: event.NewPath}
		}
		return os.Rename(event.Path, event.NewPath)
	case fsnotify.Write.String():
		return write(event)
	default:
		return EventError{err: ErrUnsupportedEvent, data: event}
	}
}
