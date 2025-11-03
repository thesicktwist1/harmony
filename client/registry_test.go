package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path"
	"strings"
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

	defer os.Chdir(wd)
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

	defer os.Chdir(wd)

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
				Data: []byte{},
			},
		},
		{
			name: "writing to a previous version of a file",
			event: func(s string, fm os.FileMode) error {
				file, err := os.OpenFile(s, os.O_RDWR, fm)
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
		{
			name: "renaming a file",
			event: func(s string, fm os.FileMode) error {
				return os.Rename(s, path.Join(storage, "renamed.txt"))
			},
			path: path.Join(storage, "test-2.txt"),
			wantFileEvent: &shared.FileEvent{
				Path:    path.Join(storage, "test-2.txt"),
				NewPath: path.Join(storage, "renamed.txt"),
				Op:      fsnotify.Rename.String(),
			},
		},
		{
			name: "moving file to watched dir from unwatched source",
			event: func(s string, fm os.FileMode) error {
				return os.Rename(s, path.Join(storage, "unwatched_file.go"))
			},
			path: "unwatched_file.go",
			wantFileEvent: &shared.FileEvent{
				Path: path.Join(storage, "unwatched_file.go"),
				Op:   fsnotify.Create.String(),
				Data: []byte("foo"),
			},
		},
		{
			name: "moving directory",
			event: func(s string, fm os.FileMode) error {
				return os.Rename(s, path.Join(storage, "dir-2", "dir-1"))
			},
			path: path.Join(storage, "dir-1"),
			wantFileEvent: &shared.FileEvent{
				Path:    path.Join(storage, "dir-1"),
				NewPath: path.Join(storage, "dir-2", "dir-1"),
				Op:      fsnotify.Rename.String(),
				IsDir:   true,
			},
		},
		{
			name: "delete directory",
			event: func(s string, fm os.FileMode) error {
				return os.RemoveAll(s)
			},
			path: path.Join(storage, "dir-2"),
			wantFileEvent: &shared.FileEvent{
				Path:  path.Join(storage, "dir-2"),
				Op:    fsnotify.Remove.String(),
				IsDir: true,
			},
		},
		{
			name: "delete file",
			event: func(s string, fm os.FileMode) error {
				return os.Remove(s)
			},
			path: path.Join(storage, "test-2.txt"),
			wantFileEvent: &shared.FileEvent{
				Path: path.Join(storage, "test-2.txt"),
				Op:   fsnotify.Remove.String(),
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

		err = os.WriteFile(path.Join(tmp, "unwatched_file.go"), []byte("foo"), perm)
		require.NoError(t, err)

		db, err := makeDB(dbPath, "sqlite")
		require.NoError(t, err)

		require.NoError(t, initDB(db))

		require.NoError(t, initTMP(tmp))

		watcher, err := fsnotify.NewWatcher()
		require.NoError(t, err)
		defer watcher.Close()

		require.NoError(t, os.Chdir(tmp))

		registry := newRegistry(watcher, db)

		err = registry.appendDir(storage)
		require.NoError(t, err)

		ctx := context.Background()

		go registry.ListenForEvents(ctx)

		require.NoError(t, tc.event(tc.path, perm))

		select {
		case msg := <-registry.msgBuffer:
			var envelope shared.Envelope

			require.NoError(t, json.Unmarshal(msg, &envelope))

			var got shared.FileEvent

			require.NoError(t, json.Unmarshal(envelope.Message, &got))

			require.Equal(t, tc.wantFileEvent, &got)

		case <-time.After(300 * time.Millisecond):
			t.Fatal("error receiving message:", tc.name)
		}

		require.NoError(t, os.Chdir(wd))
	}
}

func TestCreateStorage(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)

	defer os.Chdir(wd)

	watcher, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	defer watcher.Close()

	var (
		tmp = t.TempDir()
	)

	err = os.Chdir(tmp)
	require.NoError(t, err)

	//test 1 - storage doesn't exist so we create it
	err = shared.MakeStorage()
	require.NoError(t, err)

	got, err := os.Stat(storage)
	require.NoErrorf(t, err, "file doesn't exist (test 1)")

	require.Truef(t, got.IsDir(), "storage is not a directory (test 1)")

	err = os.Remove(storage)
	require.NoError(t, err)

	//test 2 - storage already exist as a directory we just return
	err = os.Mkdir(storage, 0777)
	require.NoError(t, err)

	err = shared.MakeStorage()
	require.NoError(t, err)

	got, err = os.Stat(storage)
	require.NoErrorf(t, err, "file doesn't exist (test 2)")

	require.Truef(t, got.IsDir(), "storage is not a directory (test 2)")

	err = os.Remove(storage)
	require.NoError(t, err)

	//test 3 - storage already exist as a file (delete and create as a directory)
	err = os.WriteFile(storage, nil, 0777)
	require.NoError(t, err)

	err = shared.MakeStorage()
	require.NoError(t, err)

	got, err = os.Stat(storage)
	require.NoErrorf(t, err, "file doesn't exist (test 3)")

	require.Truef(t, got.IsDir(), "storage is not a directory (test 3)")

	require.True(t, got.IsDir())

	require.NoError(t, os.Chdir(wd))
}

// AI Generated (with some tweaking)
func TestSyncTreeCases(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)

	defer os.Chdir(wd)

	const helloHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	type tc struct {
		name             string
		node             *shared.FSNode
		wantBackupAsFile bool
		expectEvent      bool
		wantFileEvent    *shared.FileEvent
		wantExists       map[string]bool
	}

	tests := []tc{
		{
			name: "create missing file emits Update",

			node: &shared.FSNode{
				Path:  path.Join(storage, "newfile.txt"),
				IsDir: false,
			},
			expectEvent: true,
			wantFileEvent: &shared.FileEvent{
				Path: path.Join(storage, "newfile.txt"),
				Op:   shared.Update,
			},
		},
		{
			name: "create missing dir no event",

			node: &shared.FSNode{
				Path:  path.Join(storage, "newdir"),
				IsDir: true,
			},
			expectEvent: false,
			wantExists: map[string]bool{
				path.Join(storage, "newdir"): true,
			},
		},
		{
			name: "existing dir replaced by file moves to backup and emits Update",

			node: &shared.FSNode{
				Path:  path.Join(storage, "dir-1"),
				IsDir: false,
			},
			expectEvent: true,
			wantFileEvent: &shared.FileEvent{
				Path: path.Join(storage, "dir-1"),
				Op:   shared.Update,
			},
			wantExists: map[string]bool{
				path.Join(backup, "dir-1"): true,
			},
		},
		{
			name: "file changed on disk after node timestamp emits Write with data and hash",
			node: &shared.FSNode{
				Path:    path.Join(storage, "test-2.txt"),
				IsDir:   false,
				ModTime: time.Now().Add(-2 * time.Hour).Format(shared.TimeLayout),
				Hash:    "oldhash",
			},
			expectEvent: true,
			wantFileEvent: &shared.FileEvent{
				Path: path.Join(storage, "test-2.txt"),
				Op:   fsnotify.Write.String(),
				Hash: helloHash,
				Data: []byte{},
			},
			wantExists: map[string]bool{
				path.Join(storage, "test-2.txt"): true,
			},
		},
		{
			name: "moves dir to backup (with existing backup as a file)",

			node: &shared.FSNode{
				Path:  path.Join(storage, "dir-1"),
				IsDir: false,
			},
			expectEvent: true,
			wantFileEvent: &shared.FileEvent{
				Path: path.Join(storage, "dir-1"),
				Op:   shared.Update,
			},
			wantExists: map[string]bool{
				path.Join(backup, "dir-1"): true,
			},
			wantBackupAsFile: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()

			require.NoError(t, initTMP(tmp))

			require.NoError(t, os.Chdir(tmp))

			if tc.wantBackupAsFile {
				require.NoError(t, os.WriteFile(backup, nil, 0777))
			} else {
				require.NoError(t, os.MkdirAll(backup, 0777))
			}

			watcher, err := fsnotify.NewWatcher()
			require.NoError(t, err)
			defer watcher.Close()

			r := newRegistry(watcher, nil)

			r.SyncTree(tc.node)

			// Helper to receive and decode event from msgBuffer with timeout
			var fe *shared.FileEvent
			select {
			case msg := <-r.msgBuffer:
				var env shared.Envelope
				require.NoError(t, json.Unmarshal(msg, &env))
				fe = &shared.FileEvent{}
				require.NoError(t, json.Unmarshal(env.Message, fe))
			case <-time.After(300 * time.Millisecond):
				// timeout - no event received
			}

			if tc.expectEvent {
				require.NotNil(t, fe, "expected event but none received")
				if tc.wantFileEvent.Op != "" {
					require.Equal(t, tc.wantFileEvent.Op, fe.Op)
				}
				if tc.wantFileEvent.Path != "" {
					require.Equal(t, tc.wantFileEvent.Path, fe.Path)
				}
				if tc.wantFileEvent.NewPath != "" {
					require.Equal(t, tc.wantFileEvent.NewPath, fe.NewPath)
				}
				if tc.wantFileEvent.Hash != "" {
					require.Equal(t, tc.wantFileEvent.Hash, fe.Hash)
				}
				if tc.wantFileEvent.Data != nil {
					require.Equal(t, tc.wantFileEvent.Data, fe.Data)
				}
			} else {
				require.Nil(t, fe, "did not expect event but received one")
			}

			gotExists := make(map[string]bool)
			// verify filesystem expectations
			entries, err := os.ReadDir(storage)
			require.NoError(t, err)

			backupEnt, err := os.ReadDir(backup)
			require.NoError(t, err)

			for _, file := range backupEnt {
				var entryName string
				splited := strings.Split(file.Name(), backupSep)
				if len(splited) == 1 {
					entryName = splited[0]
				} else {
					entryName = strings.Join(splited[1:], "")
				}
				entryPath := path.Join(backup, entryName)
				gotExists[entryPath] = true
			}

			for _, file := range entries {
				var entryName string
				splited := strings.Split(file.Name(), backupSep)
				if len(splited) == 1 {
					entryName = splited[0]
				} else {
					entryName = strings.Join(splited[1:], "")
				}
				entryPath := path.Join(storage, entryName)
				gotExists[entryPath] = true
			}
			for want := range tc.wantExists {
				_, exists := gotExists[want]
				require.True(t, exists)
			}
			require.NoError(t, os.Chdir(wd))
		})
	}
}
