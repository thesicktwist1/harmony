package shared

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

type EnvelopeType int

const (
	File EnvelopeType = iota
)

const (
	Update = "UPDATE"
)

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

func NewEnvelope(msg any, Type EnvelopeType) (*Envelope, error) {
	p, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return &Envelope{
		Message: p,
		Type:    Type,
	}, nil
}

func (f *FileEvent) New(data []byte) {
	f.Data = data
	newHash := sha256.Sum256(data)
	f.Hash = hex.EncodeToString(newHash[:])
}
