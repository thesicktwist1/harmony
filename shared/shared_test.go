package shared

import (
	"context"
	"database/sql"
	"os"
	"path"
	"testing"

	"github.com/fsnotify/fsnotify"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
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
	if err := goose.Up(db, schema); err != nil {
		return nil, err
	}
	return database.New(db), nil
}

func TestServerHubProcess(t *testing.T) {
	var (
		tmp    = t.TempDir()
		dbPath = path.Join(tmp, "test.db")
	)
	db, err := makeDB(dbPath, "sqlite")
	require.NoError(t, err)
	server := NewServerHub(db)
	ctx := context.Background()

	tests := []struct {
		name       string
		event      *FileEvent
		wantHash   string
		wantExists bool
		wantDir    bool
		wantData   []byte
		wantErr    bool
		errType    error
	}{
		{
			name: "test 1 - event create directory",
			event: &FileEvent{
				Path:  path.Join(tmp, "testdir1"),
				Op:    fsnotify.Create.String(),
				IsDir: true,
			},
			wantDir: true,
		},
		{
			name: "test 2 - duplicate event create directory (testdir1 dup)",
			event: &FileEvent{
				Path:  path.Join(tmp, "testdir1"),
				Op:    fsnotify.Create.String(),
				IsDir: true,
			},
			wantDir: true,
			wantErr: true,
			errType: os.ErrExist,
		},
		{
			name: "test 3 - event write on uncreated file",
			event: &FileEvent{
				Path: path.Join(tmp, "invalid.txt"),
				Op:   fsnotify.Write.String(),
			},
			wantErr: true,
			errType: os.ErrNotExist,
		},
		{
			name: "test 4 - create file in (testdir1)",
			event: &FileEvent{
				Path: path.Join(tmp, "testdir1", "test.txt"),
				Op:   fsnotify.Create.String(),
			},
		},
		{
			name: "test 5 - move/rename (test.txt)",
			event: &FileEvent{
				Path:    path.Join(tmp, "testdir1", "test.txt"),
				NewPath: path.Join(tmp, "renamed.txt"),
				Op:      fsnotify.Rename.String(),
			},
		},
		{
			name: "test 6 - write (renamed.txt)",
			event: &FileEvent{
				Path: path.Join(tmp, "renamed.txt"),
				Op:   fsnotify.Write.String(),
				Hash: "777",
				Data: []byte("hello"),
			},
			wantData: []byte("hello"),
			wantHash: "777",
		},
		{
			name: "test 7 - write (empty path)",
			event: &FileEvent{
				Path: "",
				Op:   fsnotify.Write.String(),
			},
			wantErr: true,
			errType: ErrEmptyPath,
		},
		{
			name: "test 8 - rename (testdir1)",
			event: &FileEvent{
				Path:    path.Join(tmp, "testdir1"),
				NewPath: path.Join(tmp, "testdir-rename"),
				Op:      fsnotify.Rename.String(),
				IsDir:   true,
			},
			wantDir: true,
		},
	}
	for _, tc := range tests {
		err := server.Process(ctx, tc.event)
		if tc.wantErr {
			require.Error(t, err)
			require.ErrorIs(t, err, tc.errType)
		} else {
			switch tc.event.Op {
			case fsnotify.Create.String():

				got, err := os.Stat(tc.event.Path)
				require.NoError(t, err)
				require.Equal(t, tc.wantDir, got.IsDir())

				_, err = db.GetFile(ctx, tc.event.Path)
				require.NoError(t, err)
			case fsnotify.Remove.String():

				_, err := os.Stat(tc.event.Path)
				require.ErrorAs(t, err, os.ErrNotExist)

			case fsnotify.Rename.String():

				_, err := os.Stat(tc.event.Path)
				require.ErrorIs(t, err, os.ErrNotExist)

				_, err = db.GetFile(ctx, tc.event.Path)
				require.ErrorIs(t, err, sql.ErrNoRows)

				stat, err := os.Stat(tc.event.NewPath)
				require.NoErrorf(t, err, tc.name)
				require.Equalf(t, tc.wantDir, stat.IsDir(), tc.name)

				_, err = db.GetFile(ctx, tc.event.NewPath)
				require.NoErrorf(t, err, tc.name)

			case fsnotify.Write.String():
				stat, err := os.Stat(tc.event.Path)
				require.NoError(t, err)
				require.Equal(t, stat.IsDir(), tc.wantDir)

				data, err := os.ReadFile(tc.event.Path)
				require.NoError(t, err)
				require.Equal(t, data, tc.wantData)

				got, err := db.GetFile(ctx, tc.event.Path)
				require.NoError(t, err)
				require.Equal(t, tc.wantHash, got.Hash)
				require.Equal(t, tc.wantDir, got.Isdir)

			}
		}
	}
}
