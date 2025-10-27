package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/coder/websocket"
	"github.com/fsnotify/fsnotify"
	"github.com/go-chi/chi/v5"
	"github.com/thesicktwist1/harmony/shared"
	"github.com/thesicktwist1/harmony/shared/database"
)

var (
	ErrServerFull = fmt.Errorf("connection not allowed: server full")
)

const (
	defaultMaxConn   = 4
	defaultReadLimit = -1
	port             = ":8080"
	storage          = "storage"
)

type message struct {
	payload []byte
	sender  *Client
}

type clientList map[*Client]struct{}

type server struct {
	clients clientList

	ctx context.Context

	*opts

	sync.RWMutex

	shared.Hub

	http.Server
}

func NewServer(ctx context.Context, db *sql.DB, optsfunc ...optsFunc) *server {
	o := defaultOpts()
	for _, opt := range optsfunc {
		opt(o)
	}
	mux := chi.NewMux()

	s := &server{
		clients: make(clientList),
		opts:    o,
		Hub:     shared.NewServerHub(database.New(db)),
		ctx:     ctx,
		Server: http.Server{
			Addr:    port,
			Handler: mux,
		},
	}

	mux.HandleFunc("/ws", s.serveWS)

	return s
}

func (s *server) serveWS(w http.ResponseWriter, r *http.Request) {
	if s.AtMaxCapacity() {
		slog.Error("server full error", "err", ErrServerFull)
		http.Error(w, ErrServerFull.Error(), http.StatusBadGateway)
		return
	}
	conn, err := websocket.Accept(w, r, s.acceptOpts)
	if err != nil {
		slog.Error("accept error", "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	conn.SetReadLimit(s.readLimit)

	c := newClient(conn, s)
	c.name = r.RemoteAddr

	s.addClient(c)

	go c.readMessages(s.ctx)
	go c.writeMessages(s.ctx)
}

func (s *server) addClient(c *Client) {
	s.Lock()
	defer s.Unlock()
	s.clients[c] = struct{}{}
}

func (s *server) removeClient(c *Client) {
	s.Lock()
	defer s.Unlock()
	_, exists := s.clients[c]
	if exists {
		delete(s.clients, c)
		c.conn.CloseNow()
	}
}

func (s *server) AtMaxCapacity() bool {
	s.RLock()
	defer s.RUnlock()
	if s.maxConn <= defaultMaxConn {
		s.maxConn = defaultMaxConn
	}
	return len(s.clients) >= s.maxConn
}

func (s *server) broadcast(msg []byte, sender *Client) {
	s.RLock()
	clients := make(map[*Client]struct{})
	for client := range s.clients {
		if client == sender {
			continue
		}
		clients[client] = struct{}{}
	}
	s.RUnlock()
	for client := range clients {
		select {
		case client.msgBuffer <- msg:
		default:
			slog.Error("unable to send message to ", "err", client.name)
		}
	}
}

func (s *server) respond(msg []byte, client *Client) {
	if client == nil {
		slog.Error("respond called with nil client")
		return
	}
	s.Lock()
	defer s.Unlock()
	_, ok := s.clients[client]
	if !ok {
		slog.Error("impossible to reach client: ", "client", client.name)
		return
	}
	select {
	case client.msgBuffer <- msg:
	default:
		slog.Error("unable to send message to ", "client", client.name)
	}
}

func (s *server) Receive(ctx context.Context, msg message) error {
	var env shared.Envelope
	if err := json.Unmarshal(msg.payload, &env); err != nil {
		return err
	}
	switch env.Type {
	case shared.Event:
		var event shared.FileEvent
		if err := json.Unmarshal(env.Message, &event); err != nil {
			return err
		}
		if err := s.Process(ctx, &event); err != nil {
			return err
		}
		if event.Op == shared.Update {
			event.Op = fsnotify.Create.String()
			newPayload, err := shared.MarshalEnvl(event, shared.Event)
			if err != nil {
				return err
			}
			s.respond(newPayload, msg.sender)
		} else {
			s.broadcast(msg.payload, msg.sender)
		}
	}
	return nil
}
