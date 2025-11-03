package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/fsnotify/fsnotify"
	"github.com/joho/godotenv"
	"github.com/thesicktwist1/harmony/shared"
	_ "github.com/tursodatabase/libsql-client-go/libsql"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal(".env unreadable: ", err)
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL environment variable is not set")
	}
	db, err := shared.OpenDB(dbURL, "libsql")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	ctx, cancel := context.WithCancel(context.Background())
	c := NewClient(watcher, db)

	signalChan := make(chan os.Signal, 1)

	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	if err := c.Run(ctx); err != nil {
		log.Fatal(err)
	}

	go func() {
		sig := <-signalChan
		cancel()
		log.Printf("Received signal : %v. Shutting down...", sig)
	}()
	<-ctx.Done()
	log.Print("Client successfully closed")
}
