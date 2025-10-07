module server

go 1.24.6

require (
	github.com/fsnotify/fsnotify v1.9.0
	github.com/thesicktwist1/harmony-shared v0.0.0-20251007142844-a3ad62a00204
	golang.org/x/net v0.45.0
)

require (
	github.com/go-chi/chi/v5 v5.2.3
	github.com/mfridman/interpolate v0.0.2 // indirect
	github.com/pressly/goose/v3 v3.26.0 // indirect
	github.com/sethvargo/go-retry v0.3.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/sys v0.36.0 // indirect
)

replace github.com/fsnotify/fsnotify => github.com/thesicktwist1/fsnotify v0.0.0-20250930032603-633c36681ea1
