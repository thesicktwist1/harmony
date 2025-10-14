package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/thesicktwist1/harmony/shared"
	"github.com/thesicktwist1/harmony/shared/database"
)

const (
	storage          = "storage"
	defaultMaxConn   = 8
	defaultReadLimit = -1
	port             = ":8080"
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

	sync.Mutex

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
	if s.MaxCapacity() {
		http.Error(w, ErrMaxCapacity.Error(), http.StatusBadGateway)
		log.Print(ErrMaxCapacity)
		return
	}
	conn, err := websocket.Accept(w, r, s.acceptOpts)
	if err != nil {
		log.Print(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer conn.CloseNow()

	conn.SetReadLimit(s.readLimit)

	c := newClient(conn, s)
	c.name = r.RemoteAddr

	s.addClient(c)

	go c.readMessages(s.ctx)
	c.writeMessages(s.ctx)
}

func (s *server) addClient(c *Client) {
	s.Lock()
	defer s.Unlock()
	s.clients[c] = struct{}{}
}

func (s *server) removeClient(c *Client) {
	s.Lock()
	defer s.Unlock()
	delete(s.clients, c)
}

func (s *server) MaxCapacity() bool {
	s.Lock()
	defer s.Unlock()
	maxCap := len(s.clients) >= s.maxConn
	return maxCap
}

func (s *server) broadcast(msg []byte, sender *Client) {
	s.Lock()
	clients := s.clients
	s.Unlock()
	for client := range clients {
		if sender == client {
			continue
		}
		select {
		case client.msgBuffer <- msg:
		default:
			log.Print("unable to send message to ", client.name)
		}
	}
}

func (s *server) send(msg []byte, client *Client) {
	s.Lock()
	_, ok := s.clients[client]
	s.Unlock()
	if !ok {
		log.Print("attempted to send message to a nil client: ", client.name)
		return
	}
	select {
	case client.msgBuffer <- msg:
	default:
		log.Print("unable to send message to ", client.name)
	}
}

func (s *server) Receive(ctx context.Context, msg message) error {
	var env shared.Envelope
	if err := json.Unmarshal(msg.payload, &env); err != nil {
		return err
	}
	switch env.Type {
	case shared.File:
		var event shared.FileEvent
		if err := json.Unmarshal(env.Message, &event); err != nil {
			return err
		}
		if err := s.Process(ctx, &event); err != nil {
			return err
		}
		if event.Op == shared.Update {
			newEnv, err := shared.NewEnvelope(event, shared.File)
			if err != nil {
				return err
			}
			newPayload, err := json.Marshal(newEnv)
			if err != nil {
				return err
			}
			s.send(newPayload, msg.sender)
		} else {
			s.broadcast(msg.payload, msg.sender)
		}
	}
	return nil
}
