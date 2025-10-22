module github.com/thesicktwist1/harmony/client

go 1.24.6

require (
	github.com/coder/websocket v1.8.14
	github.com/fsnotify/fsnotify v1.9.0
	github.com/joho/godotenv v1.5.1
	github.com/mattn/go-sqlite3 v1.14.32
	github.com/pressly/goose/v3 v3.26.0
	github.com/stretchr/testify v1.11.1
	github.com/thesicktwist1/harmony/shared v0.0.0
	github.com/tursodatabase/libsql-client-go v0.0.0-20240902231107-85af5b9d094d
	modernc.org/sqlite v1.39.1
)

require (
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mfridman/interpolate v0.0.2 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/sethvargo/go-retry v0.3.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/exp v0.0.0-20251017212417-90e834f514db // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/sys v0.37.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	modernc.org/libc v1.66.10 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

replace github.com/thesicktwist1/harmony/shared => ../shared

replace github.com/fsnotify/fsnotify => github.com/thesicktwist1/fsnotify v0.0.0-20250930032603-633c36681ea1
