package main

import "golang.org/x/net/websocket"

type client struct {
	Conn      *websocket.Conn
	msgBuffer chan []byte
}

func newClient(bufferSize int, conn *websocket.Conn) *client {
	return &client{
		Conn:      conn,
		msgBuffer: make(chan []byte, bufferSize),
	}
}
