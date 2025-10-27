package shared

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

type EnvelopeType int

const (
	Event EnvelopeType = iota
)

const (
	Update = "UPDATE"
)

var (
	ErrMalformedEvent   = errors.New("shared: malformed event")
	ErrUnsupportedEvent = errors.New("shared: unsupported event")
	ErrEmptyPath        = errors.New("shared: empty path")
	ErrInvalidPath      = errors.New("shared: invalid path")
	ErrInvalidDest      = errors.New("shared: invalid destination ")
)

type EventError struct {
	err  error
	data any
}

func (e EventError) Error() string {
	return fmt.Sprintf("%v : %+v", e.err, e.data)
}

func (e EventError) Unwrap() error {
	return e.err
}

type Envelope struct {
	Type    EnvelopeType `json:"type"`
	Message []byte       `json:"message"`
}

type FileEvent struct {
	Path    string `json:"path"`
	NewPath string `json:"newpath"`
	Op      string `json:"op"`
	Hash    string `json:"hash"`
	Data    []byte `json:"data"`
	IsDir   bool   `json:"isDir"`
}

func MarshalEnvl(msg any, Type EnvelopeType) ([]byte, error) {
	p, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return json.Marshal(&Envelope{
		Message: p,
		Type:    Type,
	})
}

func (f *FileEvent) New(data []byte) {
	f.Data = data
	newHash := sha256.Sum256(data)
	f.Hash = hex.EncodeToString(newHash[:])
}
