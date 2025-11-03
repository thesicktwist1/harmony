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
	path.Join(storage, "dir-1"):                           true,
	path.Join(storage, "dir-1", "subdir-1"):               true,
	path.Join(storage, "dir-1", "subdir-1", "file-4.txt"): false,
	path.Join(storage, "dir-1", "subdir-1", "file-3.txt"): false,
	path.Join(storage, "dir-1", "file-1.txt"):             false,
	path.Join(storage, "dir-2"):                           true,
	path.Join(storage, "dir-2", "subdir-2"):               true,
	path.Join(storage, "dir-2", "file-2.txt"):             false,
	path.Join(storage, "dir-3"):                           true,
	path.Join(storage, "dir-3", "subdir-3"):               true,
	path.Join(storage, "dir-3", "subdir-3", "file-3.txt"): false,
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

func initTMP(db *database.Queries) error {
	ctx := context.Background()
	for p, isDir := range paths {
		if err := os.MkdirAll(path.Dir(p), 0777); err != nil {
			return err
		}
		if isDir {
			if err := os.MkdirAll(p, 0777); err != nil {
				return err
			}
		} else {
			file, err := os.Create(p)
			if err != nil {
				return err
			}
			defer file.Close()
			if _, err := file.Write([]byte(p)); err != nil {
				return err
			}
		}
		if db != nil {
			if err := db.CreateFile(ctx, database.CreateFileParams{
				Path:  p,
				Hash:  p,
				Isdir: isDir,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func TestServerHubCreateEvent(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)

	defer os.Chdir(wd)

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
			name: "create directory",
			event: &FileEvent{
				Path:  path.Join(storage, "dir-1", "created-dir"),
				Op:    fsnotify.Create.String(),
				IsDir: true,
			},
			wantExists: map[string]bool{
				path.Join(storage, "dir-1", "created-dir"): true,
			},
		},
		{
			name: "create file",
			event: &FileEvent{
				Path:  path.Join(storage, "dir-1", "created-file.go"),
				Op:    fsnotify.Create.String(),
				IsDir: false,
			},
			wantExists: map[string]bool{
				path.Join(storage, "dir-1", "created-file.go"): false,
			},
		},
		{
			name: "invalid parent (not a directory)",
			event: &FileEvent{
				Path:  path.Join(storage, "dir-1", "file-1.txt", "created-file.go"),
				Op:    fsnotify.Create.String(),
				IsDir: false,
			},
			wantErr: true,
			errType: ErrInvalidDest,
		},
		{
			name: "invalid parent (doesn't exists)",
			event: &FileEvent{
				Path:  path.Join(storage, "not exist", "created-file.go"),
				Op:    fsnotify.Create.String(),
				IsDir: false,
			},
			wantErr: true,
			errType: os.ErrNotExist,
		},
		{
			name: "creating preexisting file",
			event: &FileEvent{
				Path:  path.Join(storage, "dir-1", "file-1.txt"),
				Op:    fsnotify.Create.String(),
				IsDir: false,
			},
		}, {
			name: "invalid (top directory doesn't match)",
			event: &FileEvent{
				Path:  path.Join("other", "dir-1", "file-1.txt"),
				Op:    fsnotify.Create.String(),
				IsDir: false,
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

		err = os.Chdir(tmp)
		require.NoError(t, err)

		err = initTMP(db)
		require.NoError(t, err)

		err = server.Process(ctx, tc.event)

		if tc.wantErr {
			var be EventError
			require.ErrorAs(t, err, &be)
			assert.True(t, errors.Is(be.Unwrap(), tc.errType))
		} else {

			require.NoErrorf(t, err, "%s", err)

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

	defer os.Chdir(wd)
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
			name: "rename (subdir-1)",
			event: &FileEvent{
				Path:    path.Join(storage, "dir-1", "subdir-1"),
				NewPath: path.Join(storage, "dir-1", "subdir-1-renamed"),
				Op:      fsnotify.Rename.String(),
				IsDir:   true,
			},
			wantExists: map[string]bool{
				path.Join(storage, "dir-1", "subdir-1-renamed"):               true,
				path.Join(storage, "dir-1", "subdir-1-renamed", "file-4.txt"): false,
			},
			wantNotExists: []string{
				path.Join(storage, "dir-1", "subdir-1"),
				path.Join(storage, "dir-1", "subdir-1", "file-4.txt"),
			},
		},
		{
			name: "move (dir-2 to subdir-3)",
			event: &FileEvent{
				Path:    path.Join(storage, "dir-2"),
				NewPath: path.Join(storage, "dir-3", "subdir-3", "dir-2"),
				Op:      fsnotify.Rename.String(),
				IsDir:   true,
			},
			wantExists: map[string]bool{
				path.Join(storage, "dir-3", "subdir-3", "dir-2"):             true,
				path.Join(storage, "dir-3", "subdir-3", "dir-2", "subdir-2"): true,
			},
			wantNotExists: []string{
				path.Join(storage, "dir-2"),
				path.Join(storage, "dir-2", "subdir-2"),
				path.Join(storage, "dir-2", "file-2.txt"),
			},
		},
		{
			name: "self move (dir-3 to subdir-3)",
			event: &FileEvent{
				Path:    path.Join(storage, "dir-3"),
				NewPath: path.Join(storage, "dir-3", "subdir-3", "dir-3"),
				Op:      fsnotify.Rename.String(),
				IsDir:   true,
			},
			wantErr: true,
			errType: ErrInvalidDest,
		},
		{
			name: "empty new path ",
			event: &FileEvent{
				Path:    path.Join(storage, "dir-3"),
				NewPath: "",
				Op:      fsnotify.Rename.String(),
				IsDir:   true,
			},
			wantErr: true,
			errType: ErrEmptyPath,
		},
		{
			name: "moving a file to an already existing path",
			event: &FileEvent{
				Path:    path.Join(storage, "dir-3", "subdir-3", "file-3.txt"),
				NewPath: path.Join(storage, "dir-1", "subdir-1", "file-3.txt"),
				Op:      fsnotify.Rename.String(),
			},
			wantErr: true,
			errType: os.ErrExist,
		},

		{
			name: "renaming file",
			event: &FileEvent{
				Path:    path.Join(storage, "dir-3", "subdir-3", "file-3.txt"),
				NewPath: path.Join(storage, "dir-3", "subdir-3", "file-renamed.txt"),
				Op:      fsnotify.Rename.String(),
			},
			wantExists: map[string]bool{
				path.Join(storage, "dir-3", "subdir-3", "file-renamed.txt"): false,
			},
			wantNotExists: []string{
				path.Join(storage, "dir-3", "subdir-3", "file-3.txt"),
			},
		},
		{
			name: "renaming to a none directory parent",
			event: &FileEvent{
				Path:    path.Join(storage, "dir-1"),
				NewPath: path.Join(storage, "dir-3", "subdir-3", "file-3.txt", "dir-1"),
				Op:      fsnotify.Rename.String(),
				IsDir:   true,
			},
			wantErr: true,
			errType: ErrInvalidDest,
		},
		{
			name: "malformed event (file type doesn't match)",
			event: &FileEvent{
				Path:    path.Join(storage, "dir-1"),
				NewPath: path.Join(storage, "dir-3", "dir-1"),
				Op:      fsnotify.Rename.String(),
				IsDir:   false,
			},
			wantErr: true,
			errType: ErrMalformedEvent,
		},
		{
			name: "moving to invalid parent ",
			event: &FileEvent{
				Path:    path.Join(storage, "dir-1"),
				NewPath: path.Join(storage, "invalid-parent", "dir-1"),
				Op:      fsnotify.Rename.String(),
				IsDir:   true,
			},
			wantErr: true,
			errType: os.ErrNotExist,
		},
		{
			name: "moving invalid file ",
			event: &FileEvent{
				Path:    path.Join(storage, "invalid.txt"),
				NewPath: path.Join(storage, "dir-1", "invalid.txt"),
				Op:      fsnotify.Rename.String(),
			},
			wantErr: true,
			errType: os.ErrNotExist,
		},
		{
			name: "empty path ",
			event: &FileEvent{
				Path:    "",
				NewPath: path.Join(storage, "dir-1", "invalid.txt"),
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

		err = os.Chdir(tmp)
		require.NoError(t, err)

		err = initTMP(db)
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

	defer os.Chdir(wd)
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
			name: "remove directory",
			event: &FileEvent{
				Path:  path.Join(storage, "dir-1"),
				Op:    fsnotify.Remove.String(),
				IsDir: true,
			},
			wantNotExists: []string{
				"dir-1",
				path.Join(storage, "dir-1", "file-1.txt"),
				path.Join(storage, "dir-1", "subdir-1", "file-3.txt"),
				path.Join(storage, "dir-1", "subdir-1", "file-4.txt"),
				path.Join(storage, "dir-1", "subdir-1"),
			},
		},
		{
			name: "remove file",
			event: &FileEvent{
				Path:  path.Join(storage, "dir-1", "file-1.txt"),
				Op:    fsnotify.Remove.String(),
				IsDir: false,
			},
			wantNotExists: []string{path.Join(storage, "dir-1", "file-1.txt")},
		},
		{
			name: "file already removed or doesn't exists",
			event: &FileEvent{
				Path:  path.Join(storage, "dir-1", "not_exists.txt"),
				Op:    fsnotify.Remove.String(),
				IsDir: false,
			},
			wantErr:       false,
			wantNotExists: []string{path.Join(storage, "dir-1", "not_exists.txt")},
		},
		{
			name: "malformed event (doesn't match file type)",
			event: &FileEvent{
				Path:  path.Join(storage, "dir-1", "file-1.txt"),
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

		err = os.Chdir(tmp)
		require.NoError(t, err)

		err = initTMP(db)
		require.NoError(t, err)

		err = server.Process(ctx, tc.event)
		if tc.wantErr {
			var be EventError
			require.ErrorAs(t, err, &be)
			assert.True(t, errors.Is(be.Unwrap(), tc.errType))
		} else {
			require.NoErrorf(t, err, "%s", err)

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

	defer os.Chdir(wd)
	tests := []struct {
		name     string
		event    *FileEvent
		wantErr  bool
		wantData []byte
		wantHash string
		errType  error
	}{
		{
			name: "write",
			event: &FileEvent{
				Path: path.Join(storage, "dir-1", "file-1.txt"),
				Op:   fsnotify.Write.String(),
				Data: []byte("new data"),
				Hash: "hash",
			},
			wantData: []byte("new data"),
			wantHash: "hash",
		},
		{
			name: "writing to a directory",
			event: &FileEvent{
				Path: path.Join(storage, "dir-1"),
				Op:   fsnotify.Write.String(),
				Data: []byte("new data"),
				Hash: "hash",
			},
			wantErr: true,
			errType: ErrMalformedEvent,
		},
		{
			name: "write to unknown file",
			event: &FileEvent{
				Path: path.Join(storage, "dir-1", "invalid.txt"),
				Op:   fsnotify.Write.String(),
			},
			wantErr: true,
			errType: os.ErrNotExist,
		},
		{
			name: "unsupported event",
			event: &FileEvent{
				Path: path.Join(storage, "dir-1", "invalid.txt"),
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

		err = os.Chdir(tmp)
		require.NoError(t, err)

		err = initTMP(db)
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
	defer os.Chdir(wd)
	tests := []struct {
		name      string
		event     *FileEvent
		wantEvent *FileEvent
		wantErr   bool
		errType   error
	}{
		{
			name: "update existing file",
			event: &FileEvent{
				Path: path.Join(storage, "dir-1", "file-1.txt"),
				Op:   Update,
			},
			wantEvent: &FileEvent{
				Path: path.Join(storage, "dir-1", "file-1.txt"),
				Op:   Update,
				Data: []byte(path.Join(storage, "dir-1", "file-1.txt")),
			},
		},
		{
			name: "update unknown file",
			event: &FileEvent{
				Path: path.Join(storage, "dir-1", "invalid.txt"),
				Op:   Update,
			},
			wantErr: true,
			errType: os.ErrNotExist,
		},
		{
			name: "updating a directory",
			event: &FileEvent{
				Path: path.Join(storage, "dir-1"),
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

		err = os.Chdir(tmp)
		require.NoError(t, err)

		err = initTMP(db)
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

func TestClientHubUpdate(t *testing.T) {
	client := NewClientHub()
	ctx := context.Background()
	wd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(wd)
	tests := []struct {
		name     string
		event    *FileEvent
		wantData []byte
		wantErr  bool
		errType  error
	}{
		{
			name: "update (existing file)",
			event: &FileEvent{
				Path: path.Join(storage, "dir-1", "file-1.txt"),
				Op:   Update,
				Data: []byte("123"),
			},
			wantData: []byte("123"),
		},
		{
			name: "update (empty path)",
			event: &FileEvent{
				Path: "",
				Op:   Update,
				Data: []byte("123"),
			},
			wantErr: true,
			errType: ErrEmptyPath,
		},
		{
			name: "file doesn't exists",
			event: &FileEvent{
				Path: path.Join(storage, "dir-1", "invalid.txt"),
				Op:   Update,
				Data: []byte("123"),
			},
			wantData: []byte("123"),
		},
		{
			name: "invalid update (trying to update a directory)",
			event: &FileEvent{
				Path: path.Join(storage, "dir-1"),
				Op:   Update,
			},
			wantErr: true,
			errType: ErrMalformedEvent,
		},
	}
	for _, tc := range tests {
		var (
			tmp = t.TempDir()
		)

		err = os.Chdir(tmp)
		require.NoError(t, err)

		err = initTMP(nil)
		require.NoError(t, err)

		err = client.Process(ctx, tc.event)
		if tc.wantErr {
			var be EventError
			require.ErrorAs(t, err, &be)
			assert.Truef(t, errors.Is(be.Unwrap(), tc.errType), "%v", tc.name)
		} else {
			require.NoErrorf(t, err, "%s - %s", err, tc.name)

			got, err := os.ReadFile(tc.event.Path)
			require.NoErrorf(t, err, "%v", tc.name)

			require.Equalf(t, tc.wantData, got, "%v", tc.name)
		}
		err = os.Chdir(wd)
		require.NoError(t, err)
	}
}

func TestBuildTree(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(wd)
	tests := []struct {
		name    string
		path    string
		want    *FSNode
		wantNil bool
	}{
		{
			name: "dir-2 tree",
			path: path.Join(storage, "dir-2"),
			want: &FSNode{
				Path:  path.Join(storage, "dir-2"),
				IsDir: true,
				Childs: map[string]*FSNode{
					"subdir-2": {
						Path:  path.Join(storage, "dir-2", "subdir-2"),
						IsDir: true,
					},
					"file-2.txt": {
						Path: path.Join(storage, "dir-2", "file-2.txt"),
						Hash: "35b6affcaf3e88291a3eddfcdf6634f4cfc5c31126d1648ab36c09aff1c1f1b1",
					},
				},
			},
		},
		{
			name:    "tree based on a file should be nil",
			path:    path.Join(storage, "dir-2", "file-2.txt"),
			want:    nil,
			wantNil: true,
		},
	}

	for _, tc := range tests {
		var (
			tmp          = t.TempDir()
			compareNodes = func(want, got *FSNode) {
				require.Equalf(t, want.Hash, got.Hash,
					"expected : %+v, got : %+v",
					want, got)
				require.Equalf(t, want.Path, got.Path,
					"expected : %+v, got : %+v",
					want, got)
				require.Equalf(t, want.IsDir, got.IsDir,
					"expected : %+v, got : %+v",
					want, got)
			}
		)

		err = os.Chdir(tmp)
		require.NoError(t, err)

		err = initTMP(nil)
		require.NoError(t, err)

		if tc.wantNil {
			require.Nil(t, tc.want)
		} else {
			require.NotNil(t, tc.want)

			got := BuildTree(tc.path)

			compareNodes(tc.want, got)

		}

		err = os.Chdir(wd)
		require.NoError(t, err)
	}
}
