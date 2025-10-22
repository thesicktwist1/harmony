package main

import (
	"context"
	"database/sql"
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
	info, err := os.Stat(storage)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Mkdir(storage, 0777); err != nil {
			return err
		}
	}
	if !info.IsDir() {
		if err := os.Remove(storage); err != nil {
			return err
		}
		if err := os.Mkdir(storage, 0777); err != nil {
			return err
		}
	}
	if err := c.registry.appendDir(storage); err != nil {
		return err
	}
	conn, err := c.Connect(ctx)
	if err != nil {
		return err
	}
	defer conn.CloseNow()
	go c.writeMessages(ctx, conn)
	c.readMessages(ctx, conn)
	return nil
}

func (c *client) Connect(ctx context.Context) (*websocket.Conn, error) {
	conn, _, err := websocket.Dial(ctx, localhost, nil)
	if err != nil {
		return nil, err
	}
	log.Println("Connected to the server.")
	return conn, nil
}

func (c *client) readMessages(ctx context.Context, conn *websocket.Conn) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			mType, _, err := conn.Read(ctx)
			if err != nil {
				if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
					slog.Error("error abnormal closure", "err", err)
					return
				}
			}
			if mType == websocket.MessageBinary {

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
