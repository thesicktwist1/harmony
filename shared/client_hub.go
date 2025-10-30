package shared

import (
	"context"
	"os"

	"github.com/fsnotify/fsnotify"
)

type clientHub struct {
}

func NewClientHub() clientHub {
	return clientHub{}
}

func (c clientHub) Process(ctx context.Context, event *FileEvent) error {
	if err := isValidPath(event.Path); err != nil {
		return EventError{err: err, data: event}
	}
	switch event.Op {
	case fsnotify.Create.String():
		if err := create(event); err != nil {
			return EventError{err: err, data: event}
		}
	case fsnotify.Remove.String():
		if err := os.RemoveAll(event.Path); err != nil {
			return EventError{err: err, data: event}
		}
	case fsnotify.Rename.String():
		if err := rename(event); err != nil {
			return EventError{err: err, data: event}
		}
		if err := os.Rename(event.Path, event.NewPath); err != nil {
			return EventError{err: err, data: event}
		}
	case fsnotify.Write.String():
		if err := write(event); err != nil {
			return EventError{err: err, data: event}
		}
	case Update:
		if err := write(event); err != nil {
			return EventError{err: err, data: event}
		}
	default:
		return EventError{err: ErrUnsupportedEvent, data: event}
	}
	return nil
}

func (c clientHub) CreateStorage() error {
	return createStorage()
}
