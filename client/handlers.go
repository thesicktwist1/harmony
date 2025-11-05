package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/thesicktwist1/harmony/shared"
)

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

func (r *registry) handleWrite(ctx context.Context, e fsnotify.Event) error {
	var exists bool
	fileinfo, err := r.DB.GetFile(ctx, e.Name)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	} else {
		if e.Op.Has(fsnotify.Create) {
			return r.broadcastEvent(&shared.FileEvent{
				Path: e.Name,
				Op:   shared.Update,
			})
		}
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

func (r *registry) handleCreate(ctx context.Context, event fsnotify.Event) error {
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
		if err := r.handleWrite(ctx, event); err != nil {
			return err
		}
	}
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
			if err := r.handleWrite(ctx, fsnotify.Event{
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
			slog.Error("error removing directory :", "err", err)
		}
	}

	r.Lock()
	delete(r.watchedDir, path)
	r.Unlock()

	log.Printf("%v directory removed from the watchlist\n", filepath.Base(path))
	return nil
}
