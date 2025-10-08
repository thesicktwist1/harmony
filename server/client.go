package main

import (
	"context"
	"log"

	"github.com/coder/websocket"
	"github.com/thesicktwist1/harmony/shared"
)

const (
	bufferSize = 32
)

type Client struct {
	name      string
	msgBuffer chan []byte
	conn      *websocket.Conn
	server    *server
}

func newClient(conn *websocket.Conn, server *server) *Client {
	return &Client{
		msgBuffer: make(chan []byte, bufferSize),
		conn:      conn,
		server:    server,
	}
}

func (c *Client) readMessages(ctx context.Context) {
	defer c.server.removeClient(c)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			mType, msg, err := c.conn.Read(ctx)
			if err != nil {
				if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
					log.Print(err)
				}
				return
			}
			if mType == websocket.MessageBinary {
				op, err := c.server.Receive(msg, ctx)
				if err != nil {
					log.Print(err)
					return
				}
				if op == shared.Update {
					continue
				}
				c.server.broadcastMessage(msg, c)
			}
		}
	}
}

func (c *Client) writeMessages(ctx context.Context) {
	defer c.server.removeClient(c)
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-c.msgBuffer:
			if !ok {
				if err := c.conn.Close(websocket.StatusAbnormalClosure, "channel closed"); err != nil {
					log.Print(err)
				}
				return
			}
			if err := c.conn.Write(ctx, websocket.MessageBinary, msg); err != nil {
				if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
					log.Print(err)
				}
				return
			}
		}
	}
}
