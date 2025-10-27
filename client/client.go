package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"log/slog"
	"os"

	"github.com/coder/websocket"
	"github.com/fsnotify/fsnotify"
	"github.com/thesicktwist1/harmony/shared"
	"github.com/thesicktwist1/harmony/shared/database"
)

const (
	localhost = "ws://localhost:8080/ws"
)

type client struct {
	registry *registry
	shared.Hub
}

func newClient(watcher *fsnotify.Watcher, db *sql.DB) *client {
	return &client{
		registry: newRegistry(watcher, database.New(db)),
		Hub:      shared.NewClientHub(),
	}
}

func (c *client) Run(ctx context.Context) error {
	log.Println("client starting...")
	if err := c.CreateStorage(); err != nil {
		return err
	}
	conn, _, err := websocket.Dial(ctx, localhost, nil)
	if err != nil {
		return err
	}
	defer conn.CloseNow()
	go c.writeMessages(ctx, conn)
	c.readMessages(ctx, conn)
	return nil
}

func (c *client) CreateStorage() error {
	info, err := os.Stat(storage)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Mkdir(storage, 0777); err != nil {
			return err
		}
	} else {
		if !info.IsDir() {
			if err := os.Remove(storage); err != nil {
				return err
			}
			if err := os.Mkdir(storage, 0777); err != nil {
				return err
			}
		}
	}
	if err := c.registry.appendDir(storage); err != nil {
		return err
	}
	return nil
}

func (c *client) readMessages(ctx context.Context, conn *websocket.Conn) {

	for {
		select {
		case <-ctx.Done():
			return
		default:
			mType, msg, err := conn.Read(ctx)
			if err != nil {
				if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
					slog.Error("error abnormal closure", "err", err)
					return
				}
			}
			if mType == websocket.MessageBinary {
				var env shared.Envelope
				if err := json.Unmarshal(msg, &env); err != nil {
					slog.Error("unmarshal envelope error: %v", "err", err)
					return
				}
				switch env.Type {
				case shared.Event:
					var event shared.FileEvent
					if err := json.Unmarshal(env.Message, &event); err != nil {
						slog.Error("unmarshal file event error: %v", "err", err)
						return
					}
					if err := c.Process(ctx, &event); err != nil {
						slog.Error("error processing event: %v", "err", err)
					}
				}
			}
		}
	}
}

func (c *client) writeMessages(ctx context.Context, conn *websocket.Conn) {
	for {
		select {
		case msg, ok := <-c.registry.msgBuffer:
			if !ok {
				slog.Error("client message buffer closed")
				return
			}
			if err := conn.Write(ctx, websocket.MessageBinary, msg); err != nil {
				slog.Error("connection closed error", "err", err)
				return
			}
		case <-ctx.Done():
			return
		}
	}
}
