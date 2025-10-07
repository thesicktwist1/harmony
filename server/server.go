package main

import (
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	shared "github.com/thesicktwist1/harmony-shared"
	"github.com/thesicktwist1/harmony-shared/database"
)

const (
	storage = "/storage"
)

type clientList map[*client]struct{}

type opts struct {
}

type server struct {
	DB      database.Queries
	clients clientList
	mux     *chi.Mux

	opts *opts
	sync.Mutex
	manager shared.Manager
}

type optsFunc func(*opts)

func NewServer(db database.Queries, optsfunc ...optsFunc) *server {
	o := &opts{}
	for _, opt := range optsfunc {
		opt(o)
	}
	s := &server{
		mux:     chi.NewMux(),
		clients: make(clientList),
		manager: shared.NewManager(),
		opts:    o,
	}
	return s
}

func (s *server) ConnectToWS(w http.ResponseWriter, r *http.Request) {

}
