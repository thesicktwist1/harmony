-- +goose Up
CREATE TABLE files(
    path TEXT PRIMARY KEY,
    hash TEXT NOT NULL,
    updatedAt TEXT NOT NULL,
    createdAt TEXT NOT NULL,
    isDir BOOLEAN NOT NULL
);


-- +goose Down
DROP TABLE files;