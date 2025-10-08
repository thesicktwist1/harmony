module github.com/thesicktwist1/harmony/client
go 1.24.6

require (
	github.com/coder/websocket v1.8.14
	github.com/fsnotify/fsnotify v1.9.0
    github.com/thesicktwist1/harmony/shared v0.0.0
)

require (
	github.com/mfridman/interpolate v0.0.2 // indirect
	github.com/pressly/goose/v3 v3.26.0 // indirect
	github.com/sethvargo/go-retry v0.3.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/sys v0.36.0 // indirect
)

replace github.com/thesicktwist1/harmony/shared => ../shared
replace github.com/fsnotify/fsnotify => github.com/thesicktwist1/fsnotify v0.0.0-20250930032603-633c36681ea1