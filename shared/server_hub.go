package shared

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/thesicktwist1/harmony/shared/database"
)

const (
	TimeLayout = "2006-01-02 15:04:05.999999999 -0700 MST"
	perm       = 0777
	storage    = "storage"
	sep        = "/"
)

type serverHub struct {
	DB *database.Queries
}

func NewServerHub(db *database.Queries) serverHub {
	return serverHub{DB: db}
}

func (s serverHub) Create(ctx context.Context, event *FileEvent) error {
	if err := create(event); err != nil {
		return err
	}
	if err := s.DB.CreateFile(ctx, database.CreateFileParams{
		Path:      event.Path,
		Hash:      event.Hash,
		Updatedat: time.Now().Format(TimeLayout),
		Createdat: time.Now().Format(TimeLayout),
		Isdir:     event.IsDir,
	}); err != nil {
		return err
	}
	return nil
}

func (s serverHub) Process(ctx context.Context, event *FileEvent) error {
	if err := isValidPath(event.Path); err != nil {
		return EventError{err: err, data: event}
	}
	fmt.Printf("Processing event: %s, path: %s\n", event.Op, event.Path)
	switch event.Op {
	case fsnotify.Create.String():
		if err := s.Create(ctx, event); err != nil {
			return EventError{err: err, data: event}
		}
	case fsnotify.Remove.String():
		if err := s.Remove(ctx, event); err != nil {
			return EventError{err: err, data: event}
		}
	case fsnotify.Rename.String():
		if err := s.Rename(ctx, event); err != nil {
			return EventError{err: err, data: event}
		}
	case fsnotify.Write.String():
		if err := s.Write(ctx, event); err != nil {
			return EventError{err: err, data: event}
		}
	case Update:
		if err := s.Update(ctx, event); err != nil {
			return EventError{err: err, data: event}
		}
	default:
		return EventError{err: ErrUnsupportedEvent, data: event.Op}
	}
	return nil
}

func (s serverHub) Update(ctx context.Context, event *FileEvent) error {
	stat, err := os.Stat(event.Path)
	if err != nil {
		return err
	}
	if stat.IsDir() {
		return ErrInvalidPath
	}
	data, err := os.ReadFile(event.Path)
	if err != nil {
		return err
	}
	event.New(data)
	return nil
}

func (s serverHub) Rename(ctx context.Context, event *FileEvent) error {
	if err := rename(event); err != nil {
		return err
	}
	if event.IsDir {
		if err := s.renameDir(ctx, event); err != nil {
			return err
		}
	} else {
		if err := s.renameFile(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (s serverHub) Remove(ctx context.Context, event *FileEvent) error {
	stat, err := os.Stat(event.Path)
	if err != nil {
		return err
	}
	if stat.IsDir() != event.IsDir {
		return ErrMalformedEvent
	}
	if stat.IsDir() {
		if err := s.removeDirFromDB(ctx, event.Path); err != nil {
			return err
		}
	} else {
		if err := s.removeFileFromDB(ctx, event.Path); err != nil {
			return err
		}
	}
	return os.RemoveAll(event.Path)
}

func (s serverHub) removeDirFromDB(ctx context.Context, path string) error {
	childs, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, child := range childs {
		childPath := filepath.Join(path, child.Name())
		if child.IsDir() {
			if err := s.removeDirFromDB(ctx, childPath); err != nil {
				return err
			}
		} else {
			if err := s.removeFileFromDB(ctx, childPath); err != nil {
				return err
			}
		}
	}
	if err := s.DB.DeleteFile(ctx, path); err != nil {
		return err
	}
	return nil
}

func (s serverHub) removeFileFromDB(ctx context.Context, path string) error {
	if err := s.DB.DeleteFile(ctx, path); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return nil
}

func (s serverHub) addDirToDB(ctx context.Context, path string) error {
	childs, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, child := range childs {
		childPath := filepath.Join(path, child.Name())
		if child.IsDir() {
			if err := s.addDirToDB(ctx, childPath); err != nil {
				return err
			}
		} else {
			if err := s.addFileToDB(ctx, childPath); err != nil {
				return err
			}
		}
	}
	if err := s.DB.CreateFile(ctx, database.CreateFileParams{
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

func (s serverHub) addFileToDB(ctx context.Context, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	newHash := sha256.Sum256(data)
	if err := s.DB.CreateFile(ctx, database.CreateFileParams{
		Path:      path,
		Hash:      hex.EncodeToString(newHash[:]),
		Updatedat: time.Now().Format(TimeLayout),
		Createdat: time.Now().Format(TimeLayout),
		Isdir:     false,
	}); err != nil {
		return err
	}
	return nil
}

func (s serverHub) renameDir(ctx context.Context, event *FileEvent) error {
	if err := s.removeDirFromDB(ctx, event.Path); err != nil {
		return err
	}
	if err := os.Rename(event.Path, event.NewPath); err != nil {
		return err
	}
	if err := s.addDirToDB(ctx, event.NewPath); err != nil {
		return err
	}
	return nil
}

func (s serverHub) Write(ctx context.Context, event *FileEvent) error {
	stat, err := os.Stat(event.Path)
	if err != nil {
		return err
	}
	if stat.IsDir() {
		return ErrMalformedEvent
	}
	if err := os.WriteFile(event.Path, event.Data, 0777); err != nil {
		return err
	}
	if err := s.DB.UpdateFile(ctx, database.UpdateFileParams{
		Hash:      event.Hash,
		Updatedat: time.Now().Format(TimeLayout),
		Path:      event.Path,
	}); err != nil {
		return err
	}
	return nil
}

func (s serverHub) renameFile(ctx context.Context, event *FileEvent) error {
	if err := s.removeFileFromDB(ctx, event.Path); err != nil {
		return err
	}
	if err := os.Rename(event.Path, event.NewPath); err != nil {
		return err
	}
	if err := s.addFileToDB(ctx, event.NewPath); err != nil {
		return err
	}
	return nil
}

func (s serverHub) CreateStorage() error {
	return createStorage()
}
