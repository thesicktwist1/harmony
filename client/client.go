package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
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
	}
}

func (c *client) Run(ctx context.Context) error {
	log.Println("client starting...")
	info, err := os.Stat(storage)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("storage file is not a directory")
	}
	if err := c.registry.addDir(storage); err != nil {
		return err
	}

	go c.registry.ListenForEvents(ctx)

	if err := c.ConnectToServer(ctx); err != nil {
		return err
	}
	return nil
}

func (c *client) ConnectToServer(ctx context.Context) error {
	conn, _, err := websocket.Dial(ctx, localhost, nil)
	if err != nil {
		return err
	}
	defer conn.CloseNow()

	log.Println("Connected to the server.")

	go c.writeMessages(ctx, conn)
	c.readMessages(ctx, conn)
	return nil
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
					log.Print(err)
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
				log.Printf("client message buffer closed, stopping writer")
				return
			}
			if err := conn.Write(ctx, websocket.MessageBinary, msg); err != nil {
				log.Print(err)
				return
			}
		case <-ctx.Done():
			return
		}
	}
}
