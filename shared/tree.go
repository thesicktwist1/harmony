package shared

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"os"
	"path"
	"slices"
	"strings"
)

type FSNode struct {
	Path    string
	ModTime string
	Hash    string
	IsDir   bool
	Childs  []*FSNode
}

func BuildTree(p string, parent *FSNode) *FSNode {
	info, err := os.Stat(p)
	if err != nil || !info.IsDir() {
		slog.Error("error fetching info : %v", "err", err)
		return nil
	}
	childs, err := os.ReadDir(p)
	if err != nil {
		slog.Error("error reading dir: %v", "err", err)
		return nil
	}
	currNode := &FSNode{
		Path:    p,
		ModTime: info.ModTime().Format(TimeLayout),
		Childs:  make([]*FSNode, len(childs)),
		IsDir:   true,
	}
	for i, child := range childs {
		childPath := path.Join(p, child.Name())
		if child.IsDir() {
			currNode.Childs[i] = BuildTree(childPath, currNode)
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
					currNode.Childs[i] = &FSNode{
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

func SortChilds(node *FSNode) {
	if node == nil {
		return
	}
	slices.SortFunc(node.Childs, func(a, b *FSNode) int {
		return strings.Compare(a.Hash, b.Hash)
	})
	for _, child := range node.Childs {
		SortChilds(child)
	}
}
