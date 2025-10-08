package main

import (
	"errors"
	"fmt"
)

var (
	ErrMaxCapacity = errors.New("server: max capacity reached")
)

type ServerError struct {
	err  error
	msg  string
	data any
}

func (s *ServerError) Error() string {
	return fmt.Sprintf("%v : %s : %v",
		s.err,
		s.msg,
		s.data,
	)
}

func (s *ServerError) Unwrap() error {
	return s.err
}
