package shared

import "encoding/json"

type EnvelopeType int

const (
	File EnvelopeType = iota
)

const (
	Update = "Update"
)

type Envelope struct {
	Type    EnvelopeType `json:"type"`
	Message []byte       `json:"message"`
}

type Message interface {
	SetData([]byte)
}
type FileEvent struct {
	Path    string `json:"path"`
	NewPath string `json:"newpath"`
	Op      string `json:"op"`
	Hash    string `json:"hash"`
	Data    []byte `json:"data"`
	IsDir   bool   `json:"isDir"`
}

func NewEnvelope(msg Message, Type EnvelopeType) (*Envelope, error) {
	p, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return &Envelope{
		Message: p,
		Type:    Type,
	}, nil
}

func (f *FileEvent) SetData(data []byte) {
	f.Data = data
}

func (f *FileEvent) SetHash(hash string) {
	f.Hash = hash
}
