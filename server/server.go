package main

import (
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
)

type clientList map[*Client]struct{}

type server struct {
	clients clientList

	mux *chi.Mux

	*opts

	sync.Mutex

	shared.Manager
}

func NewServer(db *database.Queries, optsfunc ...optsFunc) *server {
	o := defaultOpts()
	for _, opt := range optsfunc {
		opt(o)
	}
	s := &server{
		mux:     chi.NewMux(),
		clients: make(clientList),
		opts:    o,
		Manager: shared.NewManager(true, db),
	}
	s.mux.HandleFunc("ws", s.serveWS)
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

	ctx := r.Context()

	s.addClient(c)

	go c.readMessages(ctx)
	c.writeMessages(ctx)
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
	if len(s.clients) >= s.maxConn {
		return true
	}
	return false
}

func (s *server) broadcastMessage(msg []byte, client *Client) {
	s.Lock()
	clients := make([]*Client, len(s.clients))
	var i int
	for c := range s.clients {
		clients[i] = c
		i++
	}
	s.Unlock()
	for _, c := range clients {
		if client == c {
			continue
		}
		select {
		case c.msgBuffer <- msg:
		default:
			log.Print("unable to send message to ", c.name)
		}
	}
}
