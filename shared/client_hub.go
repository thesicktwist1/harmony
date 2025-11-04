package shared

import (
	"context"
	"os"

	"github.com/fsnotify/fsnotify"
)

type clientHub struct {
	handlers map[string]EventHandler
}

func NewClientHub() clientHub {
	return clientHub{
		handlers: setupClientEventHandler(),
	}
}

type EventHandler func(context.Context, *FileEvent) error

func (c clientHub) Process(ctx context.Context, event *FileEvent) error {
	if err := isValidPath(event.Path); err != nil {
		return EventError{err: err, data: event}
	}
	handler, exist := c.handlers[event.Op]
	if !exist {
		return EventError{err: ErrUnsupportedEvent, data: event.Op}
	}
	if err := handler(nil, event); err != nil {
		return EventError{err: err, path: event.Path, data: event.Hash}
	}
	return nil
}

func setupClientEventHandler() map[string]EventHandler {
	handlers := make(map[string]EventHandler)
	handlers[fsnotify.Create.String()] = func(_ context.Context, fe *FileEvent) error {
		return create(fe)
	}
	handlers[fsnotify.Write.String()] = func(_ context.Context, fe *FileEvent) error {
		return write(fe)
	}
	handlers[fsnotify.Rename.String()] = func(_ context.Context, fe *FileEvent) error {
		if err := rename(fe); err != nil {
			return err
		}
		return os.Rename(fe.Path, fe.NewPath)
	}
	handlers[fsnotify.Remove.String()] = func(_ context.Context, fe *FileEvent) error {
		return os.RemoveAll(fe.Path)
	}
	handlers[Update] = func(_ context.Context, fe *FileEvent) error {
		return write(fe)
	}
	return handlers
}
