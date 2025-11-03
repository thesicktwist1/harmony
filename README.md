# Harmony 

Harmony is a cloud storage system written in go. 
It recursively monitors a directory using **fsnotify** , synchronizes you're files
in real time using **websocket** and stores metadata in a sql **database**.

---

## Overview

Whenever a file is created, modified, or deleted the client sends the event 
to the server. The server then processes the payload, updates his own
directory to mirror the changes, stores the metadata and finally sends 
the changes to other connected clients.

**Using:**
- **Go** Simple, fast, and concurrent
- **fsnotify** Cross-platform filesystem notifier
- **WebSocket** Communication between client and server
- **Turso (libSql)** SQLite-compatible database

## 🧰 Requirements

- [Go 1.20+](https://go.dev/dl/)
- [Turso Database](https://turso.tech/)
- [libSQL Go client](https://github.com/tursodatabase/libsql-client-go)

