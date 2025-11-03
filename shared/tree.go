package shared

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"os"
	"path"
)

type FSNode struct {
	Path    string
	ModTime string
	Hash    string
	IsDir   bool
	Childs  map[string]*FSNode
}

func BuildTree(p string) *FSNode {
	info, err := os.Stat(p)
	if err != nil || !info.IsDir() {
		slog.Error("error fetching  directory : %v", "err", err)
		return nil
	}
	childs, err := os.ReadDir(p)
	if err != nil {
		slog.Error("error reading directory : %v", "err", err)
		return nil
	}
	currNode := &FSNode{
		Path:    p,
		ModTime: info.ModTime().Format(TimeLayout),
		Childs:  make(map[string]*FSNode),
		IsDir:   true,
	}
	for _, child := range childs {
		childPath := path.Join(p, child.Name())
		if child.IsDir() {
			currNode.Childs[child.Name()] = BuildTree(childPath)
		} else {
			info, err := child.Info()
			if err != nil {
				slog.Error("error fetching file info : %v", "err", err)
			} else {
				data, err := os.ReadFile(childPath)
				if err != nil {
					slog.Error("error reading file : %v", "err", err)
				} else {
					hash := sha256.Sum256(data)
					currNode.Childs[child.Name()] = &FSNode{
						Path:    childPath,
						Hash:    hex.EncodeToString(hash[:]),
						ModTime: info.ModTime().Format(TimeLayout),
						IsDir:   false,
					}
				}
			}
		}
	}
	return currNode
}
