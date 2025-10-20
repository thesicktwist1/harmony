package shared

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path"
	"testing"

	"github.com/fsnotify/fsnotify"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thesicktwist1/harmony/shared/database"
	_ "modernc.org/sqlite"
)

var paths = map[string]bool{
	"dir-1":                        true,
	path.Join("dir-1", "subdir-1"): true,
	path.Join("dir-1", "subdir-1", "file-4.txt"): false,
	path.Join("dir-1", "subdir-1", "file-3.txt"): false,
	path.Join("dir-1", "file-1.txt"):             false,
	"dir-2":                                      true,
	path.Join("dir-2", "subdir-2"):               true,
	path.Join("dir-2", "file-2.txt"):             false,
	"dir-3":                                      true,
	path.Join("dir-3", "subdir-3"):               true,
	path.Join("dir-3", "subdir-3", "file-3.txt"): false,
}

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

func createTmps(tmp string, db *database.Queries) error {
	ctx := context.Background()
	if err := os.Chdir(tmp); err != nil {
		return err
	}
	for p, isDir := range paths {
		if isDir {
			if err := os.MkdirAll(p, 0777); err != nil {
				return err
			}
		} else {
			if err := os.MkdirAll(path.Dir(p), 0777); err != nil {
				return err
			}
			file, err := os.Create(p)
			if err != nil {
				return err
			}
			defer file.Close()
			if _, err := file.Write([]byte(p)); err != nil {
				return err
			}
		}
		if err := db.CreateFile(ctx, database.CreateFileParams{
			Path:  p,
			Hash:  p,
			Isdir: isDir,
		}); err != nil {
			return err
		}
	}

	return nil
}

func TestServerHubCreateEvent(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)

	tests := []struct {
		name    string
		event   *FileEvent
		wantErr bool
		// wantExists is map of path
		// bool represents isDir
		wantExists    map[string]bool
		wantNotExists []string
		errType       error
	}{
		{
			name: "test 1 - create directory",
			event: &FileEvent{
				Path:  path.Join("dir-1", "created-dir"),
				Op:    fsnotify.Create.String(),
				IsDir: true,
			},
			wantExists: map[string]bool{
				path.Join("dir-1", "created-dir"): true,
			},
		},
		{
			name: "test 2 - create file",
			event: &FileEvent{
				Path:  path.Join("dir-1", "created-file.go"),
				Op:    fsnotify.Create.String(),
				IsDir: false,
			},
			wantExists: map[string]bool{
				path.Join("dir-1", "created-file.go"): false,
			},
		},
		{
			name: "test 3 - invalid parent (not a directory)",
			event: &FileEvent{
				Path:  path.Join("dir-1", "file-1.txt", "created-file.go"),
				Op:    fsnotify.Create.String(),
				IsDir: false,
			},
			wantErr: true,
			errType: ErrInvalidDest,
		},
		{
			name: "test 4 - invalid parent (doesn't exists)",
			event: &FileEvent{
				Path:  path.Join("not exist", "created-file.go"),
				Op:    fsnotify.Create.String(),
				IsDir: false,
			},
			wantErr: true,
			errType: os.ErrNotExist,
		},
		{
			name: "test 4 - invalid create (already exists)",
			event: &FileEvent{
				Path:  path.Join("dir-1", "file-1.txt"),
				Op:    fsnotify.Create.String(),
				IsDir: false,
			},
			wantErr: true,
			errType: os.ErrExist,
		},
	}
	for _, tc := range tests {
		var (
			tmp    = t.TempDir()
			dbPath = path.Join(tmp, "test.db")
		)

		db, err := makeDB(dbPath, "sqlite")
		require.NoError(t, err)

		server := NewServerHub(db)
		ctx := context.Background()

		err = createTmps(tmp, db)
		require.NoError(t, err)

		err = os.Chdir(tmp)
		require.NoError(t, err)

		err = server.Process(ctx, tc.event)

		if tc.wantErr {
			var be EventError
			require.ErrorAs(t, err, &be)
			assert.True(t, errors.Is(be.Unwrap(), tc.errType))
		} else {

			require.NoErrorf(t, err, "%s", err)

			_, exists := paths[tc.event.Path]
			require.False(t, exists)

			for _, p := range tc.wantNotExists {
				_, err := os.Stat(p)
				require.ErrorIsf(t, err, os.ErrNotExist, "%s should not exists", p)

				_, err = db.GetFile(ctx, p)
				require.ErrorIsf(t, err, sql.ErrNoRows, "%s should not exists (database)", p)
			}
			for path, isDir := range tc.wantExists {
				got, err := os.Stat(path)
				require.NoErrorf(t, err, "%s doesn't exists", path)

				require.Equalf(t, got.IsDir(), isDir, "%s", path)

				gotDB, err := db.GetFile(ctx, path)
				require.NoErrorf(t, err, "%s doesn't exists (database)", path)

				require.Equalf(t, gotDB.Isdir, isDir, "%s", path)
			}
		}
		err = os.Chdir(wd)
		require.NoError(t, err)
	}
}
func TestServerHubRenameEvent(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	tests := []struct {
		name    string
		event   *FileEvent
		wantErr bool
		// wantExists is map of path
		// bool represents isDir
		wantExists    map[string]bool
		wantNotExists []string
		errType       error
	}{
		{
			name: "test 1 - rename (subdir-1)",
			event: &FileEvent{
				Path:    path.Join("dir-1", "subdir-1"),
				NewPath: path.Join("dir-1", "subdir-1-renamed"),
				Op:      fsnotify.Rename.String(),
				IsDir:   true,
			},
			wantExists: map[string]bool{
				path.Join("dir-1", "subdir-1-renamed"):               true,
				path.Join("dir-1", "subdir-1-renamed", "file-4.txt"): false,
			},
			wantNotExists: []string{
				path.Join("dir-1", "subdir-1"),
				path.Join("dir-1", "subdir-1", "file-4.txt"),
			},
		},
		{
			name: "test 2 - move (dir-2 to subdir-3)",
			event: &FileEvent{
				Path:    path.Join("dir-2"),
				NewPath: path.Join("dir-3", "subdir-3", "dir-2"),
				Op:      fsnotify.Rename.String(),
				IsDir:   true,
			},
			wantExists: map[string]bool{
				path.Join("dir-3", "subdir-3", "dir-2"):             true,
				path.Join("dir-3", "subdir-3", "dir-2", "subdir-2"): true,
			},
			wantNotExists: []string{
				path.Join("dir-2"),
				path.Join("dir-2", "subdir-2"),
				path.Join("dir-2", "file-2.txt"),
			},
		},
		{
			name: "test 3 - self move (dir-3 to subdir-3)",
			event: &FileEvent{
				Path:    path.Join("dir-3"),
				NewPath: path.Join("dir-3", "subdir-3", "dir-3"),
				Op:      fsnotify.Rename.String(),
				IsDir:   true,
			},
			wantErr: true,
			errType: ErrInvalidDest,
		},
		{
			name: "test 4 - empty new path ",
			event: &FileEvent{
				Path:    path.Join("dir-3"),
				NewPath: "",
				Op:      fsnotify.Rename.String(),
				IsDir:   true,
			},
			wantErr: true,
			errType: ErrEmptyPath,
		},
		{
			name: "test 5 - moving a file to an already existing path",
			event: &FileEvent{
				Path:    path.Join("dir-3", "subdir-3", "file-3.txt"),
				NewPath: path.Join("dir-1", "subdir-1", "file-3.txt"),
				Op:      fsnotify.Rename.String(),
			},
			wantErr: true,
			errType: os.ErrExist,
		},

		{
			name: "test 6 - renaming file",
			event: &FileEvent{
				Path:    path.Join("dir-3", "subdir-3", "file-3.txt"),
				NewPath: path.Join("dir-3", "subdir-3", "file-renamed.txt"),
				Op:      fsnotify.Rename.String(),
			},
			wantExists: map[string]bool{
				path.Join("dir-3", "subdir-3", "file-renamed.txt"): false,
			},
			wantNotExists: []string{
				path.Join("dir-3", "subdir-3", "file-3.txt"),
			},
		},
		{
			name: "test 7 - renaming (bad extension)",
			event: &FileEvent{
				Path:    path.Join("dir-3", "subdir-3", "file-3.txt"),
				NewPath: path.Join("dir-3", "subdir-3", "renamed.go"),
				Op:      fsnotify.Rename.String(),
			},
			wantErr: true,
			errType: ErrBadExt,
		},
		{
			name: "test 8 - renaming to a none directory parent",
			event: &FileEvent{
				Path:    path.Join("dir-1"),
				NewPath: path.Join("dir-3", "subdir-3", "file-3.txt", "dir-1"),
				Op:      fsnotify.Rename.String(),
				IsDir:   true,
			},
			wantErr: true,
			errType: ErrInvalidDest,
		},
		{
			name: "test 9 - malformed event (file type doesn't match)",
			event: &FileEvent{
				Path:    path.Join("dir-1"),
				NewPath: path.Join("dir-3", "dir-1"),
				Op:      fsnotify.Rename.String(),
				IsDir:   false,
			},
			wantErr: true,
			errType: ErrMalformedEvent,
		},
		{
			name: "test 10 - moving to invalid parent ",
			event: &FileEvent{
				Path:    path.Join("dir-1"),
				NewPath: path.Join("invalid-parent", "dir-1"),
				Op:      fsnotify.Rename.String(),
				IsDir:   true,
			},
			wantErr: true,
			errType: os.ErrNotExist,
		},
		{
			name: "test 11 - moving invalid file ",
			event: &FileEvent{
				Path:    path.Join("invalid.txt"),
				NewPath: path.Join("dir-1", "invalid.txt"),
				Op:      fsnotify.Rename.String(),
			},
			wantErr: true,
			errType: os.ErrNotExist,
		},
		{
			name: "test 12 - empty path ",
			event: &FileEvent{
				Path:    "",
				NewPath: path.Join("dir-1", "invalid.txt"),
				Op:      fsnotify.Rename.String(),
			},
			wantErr: true,
			errType: ErrEmptyPath,
		},
	}
	for _, tc := range tests {
		var (
			tmp    = t.TempDir()
			dbPath = path.Join(tmp, "test.db")
		)

		db, err := makeDB(dbPath, "sqlite")
		require.NoError(t, err)

		server := NewServerHub(db)
		ctx := context.Background()

		err = createTmps(tmp, db)
		require.NoError(t, err)

		err = os.Chdir(tmp)
		require.NoError(t, err)

		err = server.Process(ctx, tc.event)

		if tc.wantErr {
			var be EventError
			require.ErrorAs(t, err, &be)
			assert.True(t, errors.Is(be.Unwrap(), tc.errType))
		} else {
			require.NoErrorf(t, err, "%s", err)
			_, exists := paths[tc.event.Path]
			require.True(t, exists)
			for _, p := range tc.wantNotExists {
				_, err := os.Stat(p)
				require.ErrorIsf(t, err, os.ErrNotExist, "%s should not exists", p)

				_, err = db.GetFile(ctx, p)
				require.ErrorIsf(t, err, sql.ErrNoRows, "%s should not exists (database)", p)
			}
			for path, isDir := range tc.wantExists {
				got, err := os.Stat(path)
				require.NoErrorf(t, err, "%s doesn't exists", path)

				require.Equalf(t, got.IsDir(), isDir, "%s", path)

				gotDB, err := db.GetFile(ctx, path)
				require.NoErrorf(t, err, "%s doesn't exists (database)", path)

				require.Equalf(t, gotDB.Isdir, isDir, "%s", path)
			}
		}
		err = os.Chdir(wd)
		require.NoError(t, err)
	}
}

func TestServerHubRemoveEvent(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	tests := []struct {
		name    string
		event   *FileEvent
		wantErr bool
		// wantExists is map of path
		// bool represents isDir
		wantNotExists []string
		errType       error
	}{
		{
			name: "test 1 - remove directory",
			event: &FileEvent{
				Path:  "dir-1",
				Op:    fsnotify.Remove.String(),
				IsDir: true,
			},
			wantNotExists: []string{
				"dir-1",
				path.Join("dir-1", "file-1.txt"),
				path.Join("dir-1", "subdir-1", "file-3.txt"),
				path.Join("dir-1", "subdir-1", "file-4.txt"),
				path.Join("dir-1", "subdir-1"),
			},
		},
		{
			name: "test 2 - remove file",
			event: &FileEvent{
				Path:  path.Join("dir-1", "file-1.txt"),
				Op:    fsnotify.Remove.String(),
				IsDir: false,
			},
			wantNotExists: []string{path.Join("dir-1", "file-1.txt")},
		},
		{
			name: "test 3 - file already removed or doesn't exists",
			event: &FileEvent{
				Path:  path.Join("dir-1", "not_exists.txt"),
				Op:    fsnotify.Remove.String(),
				IsDir: false,
			},
			wantErr: true,
			errType: os.ErrNotExist,
		},
		{
			name: "test 4 - malformed event (doesn't match file type)",
			event: &FileEvent{
				Path:  path.Join("dir-1", "file-1.txt"),
				Op:    fsnotify.Remove.String(),
				IsDir: true,
			},
			wantErr: true,
			errType: ErrMalformedEvent,
		},
	}

	for _, tc := range tests {
		var (
			tmp    = t.TempDir()
			dbPath = path.Join(tmp, "test.db")
		)

		db, err := makeDB(dbPath, "sqlite")
		require.NoError(t, err)

		server := NewServerHub(db)
		ctx := context.Background()

		err = createTmps(tmp, db)
		require.NoError(t, err)

		err = os.Chdir(tmp)
		require.NoError(t, err)

		err = server.Process(ctx, tc.event)
		if tc.wantErr {
			var be EventError
			require.ErrorAs(t, err, &be)
			assert.True(t, errors.Is(be.Unwrap(), tc.errType))
		} else {
			require.NoErrorf(t, err, "%s", err)

			_, exists := paths[tc.event.Path]
			require.True(t, exists)

			for _, p := range tc.wantNotExists {
				_, err := os.Stat(p)
				require.ErrorIsf(t, err, os.ErrNotExist, "%s should not exists", p)

				_, err = db.GetFile(ctx, p)
				require.ErrorIsf(t, err, sql.ErrNoRows, "%s should not exists (database)", p)
			}
		}
		err = os.Chdir(wd)
		require.NoError(t, err)
	}
}

func TestServerHubWriteEvent(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	tests := []struct {
		name     string
		event    *FileEvent
		wantErr  bool
		wantData []byte
		wantHash string
		errType  error
	}{
		{
			name: "test 1 - write",
			event: &FileEvent{
				Path: path.Join("dir-1", "file-1.txt"),
				Op:   fsnotify.Write.String(),
				Data: []byte("new data"),
				Hash: "hash",
			},
			wantData: []byte("new data"),
			wantHash: "hash",
		},
		{
			name: "test 2 - write to unknown file",
			event: &FileEvent{
				Path: path.Join("dir-1", "invalid.txt"),
				Op:   fsnotify.Write.String(),
			},
			wantErr: true,
			errType: os.ErrNotExist,
		},
		{
			name: "test 3 - unsupported event",
			event: &FileEvent{
				Path: path.Join("dir-1", "invalid.txt"),
				Op:   "unsupported",
			},
			wantErr: true,
			errType: ErrUnsupportedEvent,
		},
	}
	for _, tc := range tests {
		var (
			tmp    = t.TempDir()
			dbPath = path.Join(tmp, "test.db")
		)

		db, err := makeDB(dbPath, "sqlite")
		require.NoError(t, err)

		server := NewServerHub(db)
		ctx := context.Background()

		err = createTmps(tmp, db)
		require.NoError(t, err)

		err = os.Chdir(tmp)
		require.NoError(t, err)

		err = server.Process(ctx, tc.event)
		if tc.wantErr {
			var be EventError
			require.ErrorAs(t, err, &be)
			assert.True(t, errors.Is(be.Unwrap(), tc.errType))
		} else {
			require.NoErrorf(t, err, "%s", err)

			_, exists := paths[tc.event.Path]
			require.True(t, exists)

			got, err := os.ReadFile(tc.event.Path)
			require.NoError(t, err)

			require.Equal(t, tc.wantData, got)

			gotDB, err := server.DB.GetFile(ctx, tc.event.Path)
			require.NoError(t, err)

			require.Equal(t, tc.wantHash, gotDB.Hash)
			require.False(t, gotDB.Isdir)
		}

		err = os.Chdir(wd)
		require.NoError(t, err)
	}
}

func TestServerHubUpdateEvent(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	tests := []struct {
		name      string
		event     *FileEvent
		wantEvent *FileEvent
		wantErr   bool
		errType   error
	}{
		{
			name: "test 1 - update (file)",
			event: &FileEvent{
				Path: path.Join("dir-1", "file-1.txt"),
				Op:   Update,
			},
			wantEvent: &FileEvent{
				Path: path.Join("dir-1", "file-1.txt"),
				Op:   fsnotify.Write.String(),
				Data: []byte(path.Join("dir-1", "file-1.txt")),
			},
		},
		{
			name: "test 2 - update (invalid path)",
			event: &FileEvent{
				Path: path.Join("dir-1", "invalid.txt"),
				Op:   Update,
			},
			wantErr: true,
			errType: os.ErrNotExist,
		},
		{
			name: "test 3 - update (updating a directory)",
			event: &FileEvent{
				Path: "dir-1",
				Op:   Update,
			},
			wantErr: true,
			errType: ErrInvalidPath,
		},
	}
	for _, tc := range tests {
		var (
			tmp    = t.TempDir()
			dbPath = path.Join(tmp, "test.db")
		)

		db, err := makeDB(dbPath, "sqlite")
		require.NoError(t, err)

		server := NewServerHub(db)
		ctx := context.Background()

		err = createTmps(tmp, db)
		require.NoError(t, err)

		err = os.Chdir(tmp)
		require.NoError(t, err)

		err = server.Process(ctx, tc.event)
		if tc.wantErr {
			var be EventError
			require.ErrorAs(t, err, &be)
			assert.True(t, errors.Is(be.Unwrap(), tc.errType))
		} else {
			require.NoErrorf(t, err, "%s", err)

			_, exists := paths[tc.event.Path]
			require.True(t, exists)

			require.False(t, tc.event.IsDir)

			require.Equal(t, tc.wantEvent.Data, tc.event.Data)
			require.Equal(t, tc.wantEvent.Op, tc.event.Op)
			require.Equal(t, tc.wantEvent.Path, tc.event.Path)
		}

		err = os.Chdir(wd)
		require.NoError(t, err)
	}
}
