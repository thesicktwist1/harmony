package shared

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"shared/database"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	sep = "/"

	TimeLayout = "2006-01-02 15:04:05.999999999 -0700 MST"

	perm            = 0777
	shortestPathLen = 2
)

type Manager struct {
	isServer bool
	DB       *database.Queries
}

func NewManager(isServ bool, db *database.Queries) Manager {
	return Manager{
		isServer: isServ,
		DB:       db,
	}
}

func (m Manager) handleCreate(ctx context.Context, event *FileEvent) error {
	if m.isServer {
		if err := m.DB.CreateFile(ctx, database.CreateFileParams{
			Path:      event.Path,
			Hash:      event.Hash,
			Updatedat: time.Now().Format(TimeLayout),
			Createdat: time.Now().Format(TimeLayout),
			Isdir:     event.IsDir,
		}); err != nil {
			return err
		}
	}
	if _, err := os.Stat(event.Path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if event.IsDir {
				if err := os.Mkdir(event.Path, perm); err != nil {
					return err
				}
			} else {
				file, err := os.Create(event.Path)
				if err != nil {
					return err
				}
				defer file.Close()
				if _, err := file.Write(event.Data); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (m Manager) handleMessage(ctx context.Context, event *FileEvent) error {
	if m.isServer {
		log.Printf("Event: %s, Path: %s", event.Op, event.Path)
	}
	switch event.Op {
	case fsnotify.Create.String():
		if err := m.handleCreate(ctx, event); err != nil {
			return err
		}
	case fsnotify.Remove.String():
		if m.isServer {
			if event.IsDir {
				if err := m.removeDirFromDB(ctx, event.Path); err != nil {
					return err
				}
			} else {
				if err := m.removeFileFromDB(ctx, event.Path); err != nil {
					return err
				}
			}
		}
		if _, err := os.Stat(event.Path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			} else {
				return err
			}
		}
		if err := os.RemoveAll(event.Path); err != nil {
			return err
		}
	case fsnotify.Rename.String():
		if m.isServer {
			if event.IsDir {
				if err := m.renameDir(ctx, event); err != nil {
					return err
				}
			} else {
				if err := m.renameFile(ctx, event); err != nil {
					return err
				}
			}
		} else {
			if err := os.Rename(event.Path, event.NewPath); err != nil {
				return err
			}
		}
	case fsnotify.Write.String():
		if m.isServer {
			if err := m.updateFile(ctx, event); err != nil {
				return err
			}
		} else {
			if err := writeFile(event, event.Path); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeFile(event *FileEvent, path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	return os.WriteFile(path, event.Data, 0750)
}

func (m Manager) Receive(message []byte, ctx context.Context) error {
	payload := &Envelope{}
	if err := json.Unmarshal(message, payload); err != nil {
		return err
	}
	switch payload.Type {
	case File:
		msg := &FileEvent{}
		if err := json.Unmarshal(payload.Message, msg); err != nil {
			return err
		}
		if err := m.handleMessage(ctx, msg); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported payload: %v", payload.Type)
	}
	return nil
}

func (m Manager) removeDirFromDB(ctx context.Context, path string) error {
	childs, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, child := range childs {
		childPath := filepath.Join(path, child.Name())
		if child.IsDir() {
			if err := m.removeDirFromDB(ctx, childPath); err != nil {
				return err
			}
		} else {
			if err := m.removeFileFromDB(ctx, childPath); err != nil {
				return err
			}
		}
	}
	if err := m.DB.DeleteFile(ctx, path); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return nil
}

func (m Manager) removeFileFromDB(ctx context.Context, path string) error {
	if err := m.DB.DeleteFile(ctx, path); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return nil
}

func (m Manager) addDirToDB(ctx context.Context, path string) error {
	childs, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, child := range childs {
		childPath := filepath.Join(path, child.Name())
		if child.IsDir() {
			if err := m.addDirToDB(ctx, childPath); err != nil {
				return err
			}
		} else {
			if err := m.addFileToDB(ctx, childPath); err != nil {
				return err
			}
		}
	}
	if err := m.DB.CreateFile(ctx, database.CreateFileParams{
		Path:      path,
		Hash:      "",
		Updatedat: time.Now().Format(TimeLayout),
		Createdat: time.Now().Format(TimeLayout),
		Isdir:     true,
	}); err != nil {
		return err
	}
	return nil
}

func (m Manager) addFileToDB(ctx context.Context, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	newHash := sha256.New()
	if _, err := newHash.Write(data); err != nil {
		return err
	}
	if err := m.DB.CreateFile(ctx, database.CreateFileParams{
		Path:      path,
		Hash:      hex.EncodeToString(newHash.Sum(nil)),
		Updatedat: time.Now().Format(TimeLayout),
		Createdat: time.Now().Format(TimeLayout),
		Isdir:     false,
	}); err != nil {
		return err
	}
	return nil
}

func (m Manager) renameDir(ctx context.Context, event *FileEvent) error {
	if err := m.removeDirFromDB(ctx, event.Path); err != nil {
		return err
	}
	if err := os.Rename(event.Path, event.NewPath); err != nil {
		return err
	}
	if err := m.addDirToDB(ctx, event.NewPath); err != nil {
		return err
	}
	return nil
}

func (m Manager) updateFile(ctx context.Context, event *FileEvent) error {
	if _, err := os.Stat(event.Path); err != nil {
		return err
	}
	file, err := os.Create(event.Path)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(event.Data); err != nil {
		return err
	}
	if err := m.DB.UpdateFile(ctx, database.UpdateFileParams{
		Hash:      event.Hash,
		Updatedat: time.Now().Format(TimeLayout),
		Path:      event.Path,
	}); err != nil {
		return err
	}
	log.Print("Event: WRITE successful, Path: ", event.Path)
	return nil
}

func (m Manager) renameFile(ctx context.Context, event *FileEvent) error {
	if err := m.removeFileFromDB(ctx, event.Path); err != nil {
		return err
	}
	if err := os.Rename(event.Path, event.NewPath); err != nil {
		return err
	}
	if err := m.addFileToDB(ctx, event.NewPath); err != nil {
		return err
	}
	return nil
}
