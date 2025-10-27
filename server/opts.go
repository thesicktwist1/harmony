package main

import (
	"github.com/coder/websocket"
)

type optsFunc func(*opts)

func defaultOpts() *opts {
	return &opts{
		maxConn:    defaultMaxConn,
		readLimit:  defaultReadLimit,
		acceptOpts: nil,
	}
}

type opts struct {
	maxConn    int
	readLimit  int64
	acceptOpts *websocket.AcceptOptions
}

func WithMaxConn(n int) optsFunc {
	return func(o *opts) {
		o.maxConn = n
	}
}

func WithReadLimit(n int64) optsFunc {
	return func(o *opts) {
		o.readLimit = n
	}
}

func WithAcceptOpts(aOpts *websocket.AcceptOptions) optsFunc {
	return func(o *opts) {
		o.acceptOpts = aOpts
	}
}
