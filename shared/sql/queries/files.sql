-- name: GetFile :one
SELECT * FROM files
WHERE path = ?
LIMIT 1;

-- name: CreateFile :exec
INSERT INTO files (path , hash , updatedAt , createdAt, isDir)
VALUES (
    ?,
    ?,
    ?,
    ?,
    ?
)ON CONFLICT DO NOTHING;

-- name: UpdateFile :exec 
UPDATE files 
SET hash = ?,
updatedAt = ?
WHERE path = ?;


-- name: DeleteFile :exec
DELETE FROM files 
WHERE path = ?;