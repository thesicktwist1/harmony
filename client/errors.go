package main

import "fmt"

type ServerError struct {
	msg string
	err error
}

func (s ServerError) Error() string {
	return fmt.Sprintf("%s:%v", s.msg, s.err)
}

func (s ServerError) Unwrap() error {
	return s.err
}
