package main

import (
	"context"
	"log"

	"github.com/fsnotify/fsnotify"
)

const (
	sqlite3    = "sqlite3"
	schema     = "internal/sql/schema"
	bufferSize = 32
)

func main() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	ctx := context.Background()
	c := newClient(watcher)
	if err := c.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
