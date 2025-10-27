package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"math"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/thesicktwist1/harmony/shared"
	"github.com/thesicktwist1/harmony/shared/database"
)

var (
	ErrNotExist = errors.New("registry: path doesn't exist")
)

const (
	storage    = "storage"
	backup     = "backup"
	bufferSize = 32
)

type directory struct {
	name   string
	path   string
	childs childDirs
}

type WatchedDir map[string]*directory

type childDirs map[string]struct{}

type registry struct {
	// Database queries
	DB *database.Queries
	// fsnotify watcher watches for
	// any changes on a given directory
	watcher *fsnotify.Watcher

	// keeps track of the registered directories
	watchedDir WatchedDir

	// message channel used for writing to the connection
	msgBuffer chan []byte

	// mutex used to keep things safe
	sync.Mutex
}

func newRegistry(watcher *fsnotify.Watcher, db *database.Queries) *registry {
	return &registry{
		watcher:    watcher,
		watchedDir: make(WatchedDir),
		msgBuffer:  make(chan []byte, bufferSize),
		DB:         db,
	}
}

func newDirectory(name, path string) *directory {
	return &directory{
		name:   name,
		path:   path,
		childs: make(childDirs),
	}
}

func (r *registry) appendDir(path string) error {
	dir := newDirectory(filepath.Base(path), path)
	childs, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, child := range childs {
		if child.IsDir() {
			newPath := filepath.Join(path, child.Name())
			if err := r.appendDir(newPath); err != nil {
				return err
			}
			dir.childs[newPath] = struct{}{}
		}
	}

	r.Lock()
	r.watchedDir[path] = dir
	r.Unlock()
	if err := r.watcher.Add(path); err != nil {
		return err
	}
	log.Printf("%v directory added to the watchlist\n", filepath.Base(path))
	return nil
}

func (r *registry) removeDir(path string) error {
	r.Lock()
	dic, exist := r.watchedDir[path]
	if !exist {
		r.Unlock()
		return ErrNotExist
	}
	r.Unlock()

	for child := range dic.childs {
		if err := r.removeDir(child); err != nil {
			return err
		}
	}

	r.Lock()
	delete(r.watchedDir, path)
	r.Unlock()

	log.Printf("%v directory removed from the watchlist\n", filepath.Base(path))
	return nil
}

func (r *registry) ListenForEvents(ctx context.Context) {
	var (
		waitFor  = 150 * time.Millisecond
		slowWait = 250 * time.Millisecond
		mu       sync.Mutex

		timers = make(map[string]*time.Timer)

		sendEvent = func(event fsnotify.Event) {
			name := strings.Join([]string{event.Name, event.Op.String()}, "")
			mu.Lock()
			delete(timers, name)
			mu.Unlock()
			if event.Has(fsnotify.Rename) {
				event.Op = fsnotify.Remove
			}
			if event.Has(fsnotify.Create) && event.RenamedFrom != "" {
				event.Op = fsnotify.Rename
				rn := strings.Join([]string{event.RenamedFrom, fsnotify.Rename.String()}, "")
				mu.Lock()
				delete(timers, rn)
				mu.Unlock()
			}
			if event.Has(fsnotify.Write) {
				stat, err := os.Stat(event.Name)
				if err != nil {
					slog.Error("error getting fileinfo", "err", err)
					return
				}
				if stat.IsDir() {
					// Write on directory is a
					// dirty event we just return
					return
				}
			}
			if err := r.Receive(ctx, event); err != nil {
				slog.Error("registry receive error", "err", err)
			}
		}
	)
	for {
		select {
		case err, ok := <-r.watcher.Errors:
			if !ok {
				slog.Error("watcher channel closed")
				return
			}
			slog.Error("fsnotify watcher error", "err", err)

		case event, ok := <-r.watcher.Events:
			if !ok {
				slog.Error("watcher channel closed")
				return
			}
			name := strings.Join([]string{event.Name, event.Op.String()}, "")
			mu.Lock()
			t, ok := timers[name]
			mu.Unlock()
			if !ok {
				t = time.AfterFunc(math.MaxInt64, func() { sendEvent(event) })
				t.Stop()
				mu.Lock()
				timers[name] = t
				mu.Unlock()
			}
			if event.Has(fsnotify.Rename) {
				t.Reset(slowWait)
			} else {
				t.Reset(waitFor)
			}
		case <-ctx.Done():
			return
		}
	}
}

// Receive processes a single fsnotify.Event,
// handles directory creation, renaming,
// removal events and broadcasting.
func (r *registry) Receive(ctx context.Context, event fsnotify.Event) error {
	switch event.Op {
	case fsnotify.Create:
		stat, err := os.Stat(event.Name)
		if err != nil {
			return err
		}
		if stat.IsDir() {
			if err := r.appendDir(event.Name); err != nil {
				return err
			}
			if err := r.handleDir(ctx, event); err != nil {
				return err
			}
		} else {
			if err := r.handleFile(ctx, event); err != nil {
				return err
			}
		}
	case fsnotify.Remove:
		if err := r.handleRemove(ctx, event); err != nil {
			return err
		}
	case fsnotify.Rename:
		if err := r.handleRename(ctx, event); err != nil {
			return err
		}
	case fsnotify.Write:
		if err := r.handleFile(ctx, event); err != nil {
			return err
		}
	}
	log.Printf("Event: %s, Path: %s", event.Op.String(), event.Name)
	return nil
}

func (r *registry) handleRename(ctx context.Context, e fsnotify.Event) error {
	if _, err := r.DB.GetFile(ctx, e.RenamedFrom); err != nil {
		return err
	}
	stat, err := os.Stat(e.Name)
	if err != nil {
		return err
	}
	if err := r.broadcastEvent(&shared.FileEvent{
		NewPath: e.Name,
		Path:    e.RenamedFrom,
		Op:      e.Op.String(),
		IsDir:   stat.IsDir(),
	}); err != nil {
		return err
	}
	if stat.IsDir() {
		if err := r.appendDir(e.Name); err != nil {
			return err
		}
	}
	return nil
}

func (r *registry) isDir(path string) bool {
	r.Lock()
	defer r.Unlock()
	_, exists := r.watchedDir[path]
	return exists
}

func (r *registry) handleRemove(ctx context.Context, e fsnotify.Event) error {
	isDir := r.isDir(e.Name)
	if isDir {
		if err := r.removeDir(e.Name); err != nil {
			return err
		}
	}
	if _, err := r.DB.GetFile(ctx, e.Name); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	} else {
		if err := r.broadcastEvent(&shared.FileEvent{
			Path:  e.Name,
			Op:    fsnotify.Remove.String(),
			IsDir: isDir,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *registry) handleFile(ctx context.Context, e fsnotify.Event) error {
	var exists bool
	fileinfo, err := r.DB.GetFile(ctx, e.Name)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	} else {
		exists = true
	}
	file, err := os.Open(e.Name)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	newHash := sha256.Sum256(data)
	hash := hex.EncodeToString(newHash[:])
	stat, err := file.Stat()
	if err != nil {
		return err
	}
	timestamp := stat.ModTime()
	f := &shared.FileEvent{
		Path: e.Name,
		Op:   e.Op.String(),
	}
	if exists {
		updatedAt, err := time.Parse(shared.TimeLayout, fileinfo.Updatedat)
		if err != nil {
			return err
		}
		if fileinfo.Hash != hash {
			if timestamp.After(updatedAt) {
				f.Data = data
				f.Hash = hash
			} else {
				f.Op = shared.Update
			}
			if err := r.broadcastEvent(f); err != nil {
				return err
			}
		}
	} else {
		f.Data = data
		if err := r.broadcastEvent(f); err != nil {
			return err
		}
	}
	return nil
}

func (r *registry) handleDir(ctx context.Context, e fsnotify.Event) error {
	if _, err := r.DB.GetFile(ctx, e.Name); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		} else {
			f := &shared.FileEvent{
				Path:  e.Name,
				Op:    e.Op.String(),
				IsDir: true,
			}
			if err := r.broadcastEvent(f); err != nil {
				return err
			}
		}
	}
	childs, err := os.ReadDir(e.Name)
	if err != nil {
		return err
	}
	for _, child := range childs {
		childPath := path.Join(e.Name, child.Name())
		if !child.IsDir() {
			if err := r.handleFile(ctx, fsnotify.Event{
				Name: childPath,
				Op:   fsnotify.Create,
			}); err != nil {
				return err
			}
		} else {
			if err := r.handleDir(ctx, fsnotify.Event{
				Name: childPath,
				Op:   e.Op,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *registry) broadcastEvent(event *shared.FileEvent) error {
	payload, err := shared.MarshalEnvl(event, shared.Event)
	if err != nil {
		return err
	}
	select {
	case r.msgBuffer <- payload:
	default:
		return fmt.Errorf("unable to reach message buffer")
	}
	return nil
}
