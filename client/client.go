package main

import (
	"context"
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
	shared.Manager
}

func newClient(watcher *fsnotify.Watcher) *client {
	return &client{
		registry: newRegistry(watcher, bufferSize),
	}
}

func (c *client) Run(ctx context.Context) error {
	log.Println("client starting...")
	info, err := os.Stat(storage)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("storage file is not a directory.")
	}
	if err := c.registry.addDir(storage); err != nil {
		return err
	}

	go c.registry.ListenForEvents(ctx)

	return c.ConnectToServer(ctx)
}

func (c *client) ConnectToServer(ctx context.Context) error {
	db, err := shared.ConnectToDB(sqlite3, harmonyDB)
	if err != nil {
		return err
	}
	defer db.Close()

	c.registry.SetDB(database.New(db))

	conn, _, err := websocket.Dial(ctx, localhost, nil)
	if err != nil {
		return err
	}
	defer conn.CloseNow()

	log.Println("client connected to the server.")

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
			mType, msg, err := conn.Read(ctx)
			if err != nil {
				if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
					log.Print(err)
					return
				}
			}
			if mType == websocket.MessageBinary {
				if err := c.Receive(msg, ctx); err != nil {
					log.Print(err)
					return
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
