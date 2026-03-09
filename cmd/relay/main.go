package main

import (
    "log"
    "net/http"
    "os"
    "strconv"

    "github.com/Fharena/VibeDeck/internal/relay"
)

func main() {
    addr := envOr("RELAY_ADDR", ":8082")
    queueSize := envIntOr("RELAY_QUEUE_SIZE", 128)

    server := relay.NewServer(queueSize)

    log.Printf("relay server listening on %s (queue=%d)", addr, queueSize)
    if err := http.ListenAndServe(addr, server.Handler()); err != nil {
        log.Fatal(err)
    }
}

func envOr(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return fallback
}

func envIntOr(key string, fallback int) int {
    v := os.Getenv(key)
    if v == "" {
        return fallback
    }

    n, err := strconv.Atoi(v)
    if err != nil || n <= 0 {
        return fallback
    }

    return n
}
