package main

import (
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
)

const (
	storage = "/storage"
)

type clientList map[*client]struct{}

type opts struct {
	maxConn int
}

type server struct {
	clients clientList

	mux  *chi.Mux
	opts *opts
	sync.Mutex
}

type optsFunc func(*opts)

func defaultOpts() *opts {
	return &opts{}
}

func NewServer(optsfunc ...optsFunc) *server {
	o := defaultOpts()
	for _, opt := range optsfunc {
		opt(o)
	}
	s := &server{
		mux:     chi.NewMux(),
		clients: make(clientList),
		opts:    o,
	}
	return s
}

func (s *server) ConnectToWS(w http.ResponseWriter, r *http.Request) {

}
