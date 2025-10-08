package main

import (
	"log"
	"net/http"
	"os"

	"github.com/thesicktwist1/harmony/shared"
	"github.com/thesicktwist1/harmony/shared/database"
)

const (
	addr = "localhost:8080"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL environment variable is not set")
	}
	sql, err := shared.OpenDB("sqlite3", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	db := database.New(sql)
	server := NewServer(db)

	log.Fatal(http.ListenAndServe(addr, server.mux))
}
