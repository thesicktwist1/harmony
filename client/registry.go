package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"math"
	"os"
	"path"
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
	storage      = "storage"
	backup       = "backup"
	backupSep    = "_"
	bufferSize   = 48
	backUpFormat = "January 2, 2006 15:04:05"
)

type directory struct {
	name   string
	path   string
	childs childDirs
}

var once sync.Once

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

	// message channel used to write to the connection
	msgBuffer chan []byte

	handlers map[fsnotify.Op]FSEventHandler

	// mutex used to keep things safe
	sync.Mutex
}

type FSEventHandler func(context.Context, fsnotify.Event) error

func newRegistry(watcher *fsnotify.Watcher, db *database.Queries) *registry {
	r := &registry{
		watcher:    watcher,
		watchedDir: make(WatchedDir),
		msgBuffer:  make(chan []byte, bufferSize),
		DB:         db,
	}
	r.setupFSEventHandler()
	return r
}

func newDirectory(name, path string) *directory {
	return &directory{
		name:   name,
		path:   path,
		childs: make(childDirs),
	}
}

func (r *registry) SyncTree(root *shared.FSNode) {
	if root == nil {
		return
	}
	fileinfo, err := os.Stat(root.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if root.IsDir {
				if err := os.Mkdir(root.Path, 0777); err != nil {
					slog.Error("error creating dir : %v", "err", err)
					return
				}
			} else {
				if err := r.broadcastEvent(&shared.FileEvent{
					Path: root.Path,
					Op:   shared.Update,
				}); err != nil {
					slog.Error("error broadcasting event : %v", "err", err)
					return
				}
				return
			}
		} else {
			slog.Error("error :%v", "err", err)
			return
		}
	} else {
		if root.IsDir != fileinfo.IsDir() {
			// move file to backup
			if err := r.MoveToBackUp(root.Path, fileinfo.Name()); err != nil {
				slog.Error("error file to backup : ", "err", err)
				return
			}
			if root.IsDir {
				if err := os.Mkdir(root.Path, 0777); err != nil {
					slog.Error("error %v", "err", err)
					return
				}
			} else {
				if err := r.broadcastEvent(&shared.FileEvent{
					Path: root.Path,
					Op:   shared.Update,
				}); err != nil {
					slog.Error("error broadcasting event : %v", "err", err)
					return
				}
				return
			}
		}
	}
	if !root.IsDir {
		nodeTimestamp, err := time.Parse(shared.TimeLayout, root.ModTime)
		if err != nil {
			slog.Error("error parsing time : %v", "err", err)
			return
		}
		data, err := os.ReadFile(root.Path)
		if err != nil {
			slog.Error("error reading file : %v", "err", err)
			return
		}
		sumBytes := sha256.Sum256(data)
		hash := hex.EncodeToString(sumBytes[:])
		if root.Hash != hash {
			if fileinfo.ModTime().After(nodeTimestamp) {
				if err := r.broadcastEvent(&shared.FileEvent{
					Path: root.Path,
					Op:   fsnotify.Write.String(),
					Hash: hash,
					Data: data,
				}); err != nil {
					slog.Error("error broadcasting event : %v", "err", err)
					return
				}
			}
		}
	} else {
		entry, err := os.ReadDir(root.Path)
		if err != nil {
			slog.Error("error reading directory : ", "err", err)
			return
		}
		for _, child := range entry {
			_, exists := root.Childs[child.Name()]
			if !exists {
				childPath := path.Join(root.Path, child.Name())
				if err := r.MoveToBackUp(childPath, child.Name()); err != nil {
					slog.Error("error : ", "err", err)
				}
			}
		}
		for _, child := range root.Childs {
			r.SyncTree(child)
		}
	}
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
	handlers, exist := r.handlers[event.Op]
	if !exist {
		return shared.ErrUnsupportedEvent
	}
	if err := handlers(ctx, event); err != nil {
		return err
	}
	log.Printf("Event: %s, Path: %s", event.Op.String(), event.Name)
	return nil
}

func (r *registry) isDir(path string) bool {
	r.Lock()
	defer r.Unlock()
	_, exists := r.watchedDir[path]
	return exists
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

func (r *registry) MoveToBackUp(src, destName string) error {
	fileinfo, err := os.Stat(backup)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Mkdir(backup, 0777); err != nil {
			return err
		}
	}
	if !fileinfo.IsDir() {
		if err := os.RemoveAll(backup); err != nil {
			return err
		}
		if err := os.Mkdir(backup, 0777); err != nil {
			return err
		}
	}
	destName = strings.Join([]string{
		time.Now().Format(backUpFormat),
		destName,
	}, backupSep)
	dest := path.Join(backup, destName)
	if _, err := os.Stat(dest); err == nil {
		return os.ErrExist
	}
	return os.Rename(src, dest)
}

func (r *registry) setupFSEventHandler() {
	r.handlers = map[fsnotify.Op]FSEventHandler{
		fsnotify.Create: r.handleCreate,
		fsnotify.Remove: r.handleRemove,
		fsnotify.Write:  r.handleWrite,
		fsnotify.Rename: r.handleRename,
	}
}
