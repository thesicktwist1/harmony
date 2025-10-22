module github.com/thesicktwist1/harmony/server

go 1.24.6

require (
	github.com/coder/websocket v1.8.14
	github.com/go-chi/chi/v5 v5.2.3
	github.com/joho/godotenv v1.5.1
	github.com/thesicktwist1/harmony/shared v0.0.0
	github.com/tursodatabase/libsql-client-go v0.0.0-20240902231107-85af5b9d094d
)

require (
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/mattn/go-sqlite3 v1.14.32 // indirect
	github.com/mfridman/interpolate v0.0.2 // indirect
	github.com/pressly/goose/v3 v3.26.0 // indirect
	github.com/sethvargo/go-retry v0.3.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/exp v0.0.0-20251017212417-90e834f514db // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/sys v0.37.0 // indirect
)

replace github.com/fsnotify/fsnotify => github.com/thesicktwist1/fsnotify v0.0.0-20250930032603-633c36681ea1

replace github.com/thesicktwist1/harmony/shared => ../shared
