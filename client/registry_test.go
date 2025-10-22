package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"github.com/thesicktwist1/harmony/shared"
	"github.com/thesicktwist1/harmony/shared/database"
	_ "modernc.org/sqlite"
)

func makeDB(dbPath, driverName string) (*database.Queries, error) {
	db, err := sql.Open(driverName, dbPath)
	if err != nil {
		return nil, err
	}
	if err := goose.SetDialect("sqlite3"); err != nil {
		return nil, err
	}
	if err := goose.Up(db, "sql/schema"); err != nil {
		return nil, err
	}
	return database.New(db), nil
}

var files = []database.CreateFileParams{
	{
		Path:      path.Join(storage, "dir-1"),
		Isdir:     true,
		Hash:      "",
		Updatedat: "2125-10-22 14:32:45.123456789 -0400 EDT",
		Createdat: "2125-10-22 13:30:45.324291621 -0400 EDT",
	},
	{
		Path:      path.Join(storage, "dir-1", "subdir1"),
		Hash:      "",
		Isdir:     true,
		Updatedat: "2125-10-22 14:32:45.123456789 -0400 EDT",
		Createdat: "2125-10-22 13:30:45.324291621 -0400 EDT",
	},
	{
		Path:      path.Join(storage, "dir-2"),
		Hash:      "",
		Createdat: "2024-10-22 14:32:45.123456789 -0400 EDT",
		Updatedat: "2024-10-22 14:36:45.123543789 -0400 EDT",
		Isdir:     true,
	},
	{
		Path:      path.Join(storage, "dir-1", "test-1.txt"),
		Hash:      "56464",
		Isdir:     false,
		Updatedat: "2125-10-22 14:32:45.123456789 -0400 EDT",
		Createdat: "2125-10-22 13:30:45.324291621 -0400 EDT",
	},
	{
		Path:      path.Join(storage, "test-2.txt"),
		Isdir:     false,
		Hash:      "",
		Createdat: "2024-10-22 14:32:45.123456789 -0400 EDT",
		Updatedat: "2024-10-22 14:36:45.123543789 -0400 EDT",
	},
}

func initDB(db *database.Queries) error {
	ctx := context.Background()
	for _, f := range files {
		if err := db.CreateFile(ctx, f); err != nil {
			return err
		}
	}
	return nil
}

func initTMP(tmp string) error {
	if err := os.Chdir(tmp); err != nil {
		return err
	}
	for _, file := range files {
		if file.Isdir {
			if err := os.MkdirAll(file.Path, 0777); err != nil {
				return err
			}
		} else {
			if err := os.MkdirAll(path.Dir(file.Path), 0777); err != nil {
				return err
			}
			if _, err := os.Create(file.Path); err != nil {
				return err
			}
		}
	}
	return nil
}

func TestRegistryRemoveAndAppend(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	var (
		tmp = t.TempDir()
	)

	err = os.Mkdir(path.Join(tmp, storage), 0777)
	require.NoError(t, err)

	err = initTMP(tmp)
	require.NoError(t, err)

	watcher, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	defer watcher.Close()

	registry := newRegistry(watcher, nil)

	err = os.Chdir(tmp)
	require.NoError(t, err)

	// test: append

	err = registry.appendDir(storage)
	require.NoErrorf(t, err, "%v", storage)

	wantWatched := map[string]struct{}{
		storage:                                {},
		path.Join(storage, "dir-1"):            {},
		path.Join(storage, "dir-1", "subdir1"): {},
		path.Join(storage, "dir-2"):            {},
	}
	for want := range wantWatched {
		_, exist := registry.watchedDir[want]
		require.Truef(t, exist, "path %v is not watched", want)
	}

	gotLen := len(registry.watchedDir)
	wantLen := len(wantWatched)

	require.Equal(t, wantLen, gotLen)

	err = registry.appendDir("invalid path")
	require.Error(t, err, "path should not exists")

	err = registry.appendDir(path.Join(storage, "test-2.txt"))
	require.Errorf(t, err, "shouldn't be able to watch a normal file :%v")

	// test: remove

	err = registry.removeDir(path.Join(storage, "dir-1"))
	require.NoErrorf(t, err, "path should exists")

	wantWatched = map[string]struct{}{
		storage:                     {},
		path.Join(storage, "dir-2"): {},
	}

	for p := range registry.watchedDir {
		_, exists := wantWatched[p]
		require.Truef(t, exists, "exist : %v", p)
	}

	gotLen = len(registry.watchedDir)
	wantLen = len(wantWatched)

	require.Equal(t, wantLen, gotLen)

	err = registry.removeDir("invalid path")
	require.Errorf(t, err, "path should't exists")

	err = registry.removeDir(path.Join(storage, "test-2.txt"))
	require.Errorf(t, err, "path shouldn't exists in watched directory")

	err = os.Chdir(wd)
	require.NoError(t, err)
}

func TestRegistry(t *testing.T) {
	perm := os.FileMode(0777)
	wd, err := os.Getwd()
	require.NoError(t, err)
	tests := []struct {
		name          string
		event         func(string, os.FileMode) error
		path          string
		wantFileEvent *shared.FileEvent
		wantErr       bool
		errType       error
	}{
		{
			name:  "create directory",
			event: os.Mkdir,
			path:  path.Join(storage, "dir-3"),
			wantFileEvent: &shared.FileEvent{
				Path:  path.Join(storage, "dir-3"),
				Op:    fsnotify.Create.String(),
				IsDir: true,
			},
		},
		{
			name: "create file",
			event: func(s string, fm os.FileMode) error {
				return os.WriteFile(s, nil, fm)
			},
			path: path.Join(storage, "file.txt"),
			wantFileEvent: &shared.FileEvent{
				Path: path.Join(storage, "file.txt"),
				Op:   fsnotify.Create.String(),
			},
		},
		{
			name: "writing to a previous version of a file",
			event: func(s string, fm os.FileMode) error {
				file, err := os.OpenFile(s, os.O_RDWR, 0777)
				if err != nil {
					return err
				}
				defer file.Close()
				if _, err := file.WriteString("hello world"); err != nil {
					return err
				}
				return err
			},
			path: path.Join(storage, "test-2.txt"),
			wantFileEvent: &shared.FileEvent{
				Path: path.Join(storage, "test-2.txt"),
				Op:   fsnotify.Write.String(),
				Hash: "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
				Data: []byte("hello world"),
			},
		},
		{
			name: "writing to a futur version of a file (update request)",
			event: func(s string, fm os.FileMode) error {
				return os.WriteFile(s, []byte("hello world"), fm)
			},
			path: path.Join(storage, "dir-1", "test-1.txt"),
			wantFileEvent: &shared.FileEvent{
				Path: path.Join(storage, "dir-1", "test-1.txt"),
				Op:   shared.Update,
			},
		},
	}
	for _, tc := range tests {
		var (
			tmp    = t.TempDir()
			dbPath = path.Join(tmp, "test.db")
		)
		_, err = os.Create(dbPath)
		require.NoError(t, err)

		db, err := makeDB(dbPath, "sqlite")
		require.NoError(t, err)

		err = initDB(db)
		require.NoError(t, err)

		err = initTMP(tmp)
		require.NoError(t, err)

		watcher, err := fsnotify.NewWatcher()
		require.NoError(t, err)
		defer watcher.Close()

		err = os.Chdir(tmp)
		require.NoError(t, err)

		registry := newRegistry(watcher, db)

		err = registry.appendDir(storage)
		require.NoError(t, err)

		ctx := context.Background()

		go registry.ListenForEvents(ctx)

		time.Sleep(time.Millisecond * 200)
		err = tc.event(tc.path, perm)
		require.NoError(t, err)
		time.Sleep(time.Millisecond * 200)

		select {
		case msg := <-registry.msgBuffer:
			var envelope shared.Envelope
			err := json.Unmarshal(msg, &envelope)
			require.NoError(t, err)

			var got shared.FileEvent
			err = json.Unmarshal(envelope.Message, &got)
			require.NoError(t, err)

			require.Equal(t, tc.wantFileEvent, &got)
		default:
			t.Fatalf("error receiving message %v", tc.wantFileEvent)
		}

		err = os.Chdir(wd)
		require.NoError(t, err)
	}
}
