package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	shared "github.com/thesicktwist1/harmony-shared"
	"github.com/thesicktwist1/harmony-shared/database"
)

const (
	storage = "internal/client/storage"
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

	sync.Mutex
}

func newRegistry(watcher *fsnotify.Watcher, size int) *registry {
	return &registry{
		watcher:    watcher,
		watchedDir: make(WatchedDir),
		msgBuffer:  make(chan []byte, size),
	}
}

func newDirectory(name, path string) *directory {
	return &directory{
		name:   name,
		path:   path,
		childs: make(childDirs),
	}
}

func (r *registry) SetDB(db *database.Queries) {
	r.DB = db
}

func (r *registry) addDir(path string) error {
	dir := newDirectory(filepath.Base(path), path)
	childs, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, child := range childs {
		newPath := filepath.Join(path, child.Name())
		if child.IsDir() {
			if err := r.addDir(newPath); err != nil {
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
	dic, exists := r.watchedDir[path]
	if !exists {
		r.Unlock()
		return nil
	}
	r.Unlock()
	for name := range dic.childs {
		if err := r.removeDir(name); err != nil {
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
		waitFor  = 100 * time.Millisecond
		slowWait = 200 * time.Millisecond
		mu       sync.Mutex

		timers = make(map[string]*time.Timer)

		sendEvent = func(event fsnotify.Event) {
			mu.Lock()
			delete(timers, event.Name)
			mu.Unlock()
			if event.Has(fsnotify.Rename) {
				event.Op = fsnotify.Remove
			}
			if event.Has(fsnotify.Create) && event.RenamedFrom != "" {
				event.Op = fsnotify.Rename
				mu.Lock()
				delete(timers, event.RenamedFrom)
				mu.Unlock()
			}
			if event.Has(fsnotify.Write) {
				stat, err := os.Stat(event.Name)
				if err != nil {
					log.Print(err)
					return
				}
				if stat.IsDir() {
					// Write on directory is a dirty
					// event we just return
					return
				}
			}
			if err := r.Receive(event, ctx); err != nil {
				log.Print(err)
			}
		}
	)
	for {
		select {
		case err, ok := <-r.watcher.Errors:
			if !ok {
				return
			}
			r.HandleErrors(err)
		case event, ok := <-r.watcher.Events:
			if !ok {
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
func (r *registry) Receive(event fsnotify.Event, ctx context.Context) error {
	switch event.Op {
	case fsnotify.Create:
		file, err := os.Stat(event.Name)
		if err != nil {
			return err
		}
		if file.IsDir() {
			if err := r.addDir(event.Name); err != nil {
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
		if r.isDir(event.Name) {
			if err := r.removeDir(event.Name); err != nil {
				return err
			}
		}
		if err := r.handleDelete(ctx, event); err != nil {
			return err
		}
	case fsnotify.Rename:
		if err := r.handleRename(ctx, event); err != nil {
			return err
		}
		stat, err := os.Stat(event.Name)
		if err != nil {
			return err
		}
		if stat.IsDir() {
			if err := r.addDir(event.Name); err != nil {
				return err
			}
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
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	stat, err := os.Stat(e.Name)
	if err != nil {
		return err
	}
	f := &shared.FileEvent{
		NewPath: e.Name,
		Path:    e.RenamedFrom,
		Op:      e.Op.String(),
		IsDir:   stat.IsDir(),
	}
	if err := r.broadcastEvent(f); err != nil {
		return err
	}
	return nil
}

func (r *registry) HandleErrors(err error) {
	log.Print(err)
	// TODO: error handling
}

func (r *registry) isDir(path string) bool {
	r.Lock()
	defer r.Unlock()
	_, exists := r.watchedDir[path]
	return exists
}

func (r *registry) handleDelete(ctx context.Context, e fsnotify.Event) error {
	if _, err := r.DB.GetFile(ctx, e.Name); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	} else {
		f := &shared.FileEvent{
			Path: e.Name,
			Op:   e.Op.String(),
		}
		if err := r.broadcastEvent(f); err != nil {
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
	newHash := sha256.New()
	if _, err := newHash.Write(data); err != nil {
		return err
	}
	hash := hex.EncodeToString(newHash.Sum(nil))
	stat, err := file.Stat()
	if err != nil {
		return err
	}
	timestamp := stat.ModTime()
	f := &shared.FileEvent{
		Path: e.Name,
		Hash: hash,
	}
	if exists {
		updatedAt, err := time.Parse(shared.TimeLayout, fileinfo.Updatedat)
		if err != nil {
			return err
		}
		if fileinfo.Hash != hash {
			if timestamp.After(updatedAt) {
				f.Data = data
				f.Op = e.Op.String()
			} else {
				f.Op = shared.Update
			}
			if err := r.broadcastEvent(f); err != nil {
				return err
			}
		}
	} else {
		f.Data = data
		f.Op = e.Op.String()
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
		childPath := filepath.Join(e.Name, child.Name())
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
	envl, err := shared.NewEnvelope(event, shared.File)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(envl)
	if err != nil {
		return fmt.Errorf("payload marshaling error: %v", err)
	}
	r.msgBuffer <- payload
	return nil
}

func (r *registry) Update(ctx context.Context) error {
	// TODO
	return nil
}
