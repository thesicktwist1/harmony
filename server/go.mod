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
	golang.org/x/exp v0.0.0-20251009144603-d2f985daa21b // indirect
	golang.org/x/sys v0.37.0 // indirect
)

replace github.com/fsnotify/fsnotify => github.com/thesicktwist1/fsnotify v0.0.0-20250930032603-633c36681ea1

replace github.com/thesicktwist1/harmony/shared => ../shared
