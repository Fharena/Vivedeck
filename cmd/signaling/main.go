package main

import (
    "log"
    "net/http"
    "os"
    "time"

    "github.com/Fharena/Vivedeck/internal/signaling"
)

func main() {
    addr := envOr("SIGNALING_ADDR", ":8081")
    ttl := envDurationOr("PAIRING_TTL", 2*time.Minute)

    store := signaling.NewStore(ttl)
    server := signaling.NewServer(store)

    log.Printf("signaling server listening on %s", addr)
    if err := http.ListenAndServe(addr, server.Handler()); err != nil {
        log.Fatal(err)
    }
}

func envOr(key, fallback string) string {
    v := os.Getenv(key)
    if v == "" {
        return fallback
    }
    return v
}

func envDurationOr(key string, fallback time.Duration) time.Duration {
    v := os.Getenv(key)
    if v == "" {
        return fallback
    }

    d, err := time.ParseDuration(v)
    if err != nil {
        return fallback
    }
    return d
}
