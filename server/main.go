package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

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
	db, err := shared.OpenWithGoose(dbURL, "libsql", shared.WithReset())
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signalChan := make(chan os.Signal, 1)

	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	defer close(signalChan)

	server := NewServer(ctx, db)

	if err := shared.MakeStorage(); err != nil {
		log.Fatal(err)
	}

	go func() {
		sig := <-signalChan
		log.Printf("Received signal : %v. Shutting down...", sig)
		cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Print("Server forced to shutdown: ", err)
		}
	}()

	log.Fatal(server.ListenAndServe())

}
