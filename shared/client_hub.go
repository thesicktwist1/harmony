package shared

import (
	"context"
	"fmt"
	"os"

	"github.com/fsnotify/fsnotify"
)

type clientHub struct{}

func NewClientHub() clientHub {
	return clientHub{}
}

func (c clientHub) Process(ctx context.Context, event *FileEvent) error {
	switch event.Op {
	case fsnotify.Create.String():
		return create(event)
	case fsnotify.Remove.String():
		return remove(event)
	case fsnotify.Rename.String():
		return os.Rename(event.Path, event.NewPath)
	case fsnotify.Write.String():
		return write(event)
	default:
		return fmt.Errorf("TO DO")
	}
}
