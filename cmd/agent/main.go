package main

import (
    "log"
    "net/http"
    "os"

    "github.com/Fharena/Vivedeck/internal/agent"
)

func main() {
    addr := envOr("AGENT_ADDR", ":8080")
    profilePath := envOr("RUN_PROFILE_FILE", "configs/run-profiles.json")

    profiles, err := agent.LoadRunProfiles(profilePath)
    if err != nil {
        log.Fatalf("load run profiles: %v", err)
    }

    adapter := agent.NewMockAdapter()
    orchestrator := agent.NewOrchestrator(adapter, profiles)
    server := agent.NewHTTPServer(orchestrator)

    log.Printf("agent server listening on %s (adapter=%s)", addr, adapter.Name())
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
