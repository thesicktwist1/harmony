package shared

import (
	"database/sql"

	"github.com/pressly/goose/v3"
	_ "github.com/tursodatabase/go-libsql"
)

const (
	schema = "sql/schema"
)

type optsFunc func(*sql.DB) error

func WithReset() optsFunc {
	return func(d *sql.DB) error {
		if err := goose.DownTo(d, schema, 0); err != nil {
			return err
		}
		return nil
	}
}

func OpenWithGoose(dbURL string, driverName string, opts ...optsFunc) (*sql.DB, error) {
	db, err := sql.Open(driverName, dbURL)
	if err != nil {
		return nil, err
	}
	if err := goose.SetDialect("sqlite3"); err != nil {
		return nil, err
	}
	for _, opt := range opts {
		if err := opt(db); err != nil {
			return nil, err
		}
	}
	if err := goose.Up(db, schema); err != nil {
		return nil, err
	}
	return db, nil
}
