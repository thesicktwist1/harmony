package shared

import (
	"database/sql"

	"github.com/pressly/goose/v3"
)

func ConnectToDB(driverName, dataSourceName string) (*sql.DB, error) {
	db, err := sql.Open(driverName, dataSourceName)
	if err != nil {
		return nil, err
	}
	if err := goose.SetDialect(driverName); err != nil {
		return nil, err
	}
	return db, nil
}
