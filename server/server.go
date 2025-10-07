package main

import (
	"net/http"
	"sync"

	"github.com/go-chi/chi"
	"github.com/thesicktwist1/harmony/shared"
	"github.com/thesicktwist1/harmony/shared/database"
)

const (
	storage = "/storage"
)

type clientList map[*client]struct{}

type opts struct {
	maxConn int
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

func defaultOpts() *opts {
	return &opts{}
}

func NewServer(db database.Queries, optsfunc ...optsFunc) *server {
	o := defaultOpts()
	for _, opt := range optsfunc {
		opt(o)
	}
	s := &server{
		mux:     chi.NewMux(),
		clients: make(clientList),
		opts:    o,
		manager: shared.NewManager(true, db),
	}
	return s
}

func (s *server) ConnectToWS(w http.ResponseWriter, r *http.Request) {

}
