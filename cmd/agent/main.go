package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Fharena/Vivedeck/internal/agent"
	"github.com/Fharena/Vivedeck/internal/runtime"
)

func main() {
	addr := envOr("AGENT_ADDR", ":8080")
	profilePath := envOr("RUN_PROFILE_FILE", "configs/run-profiles.json")
	signalingBaseURL := envOr("SIGNALING_BASE_URL", "http://127.0.0.1:8081")

	profiles, err := agent.LoadRunProfiles(profilePath)
	if err != nil {
		log.Fatalf("load run profiles: %v", err)
	}

	adapter := agent.NewMockAdapter()
	orchestrator := agent.NewOrchestrator(adapter, profiles)

	stateManager := runtime.NewStateManager(runtime.DefaultManagerConfig())
	ackTracker := runtime.NewAckTracker(2 * time.Second)
	p2pManager := agent.NewP2PSessionManager(stateManager, ackTracker, orchestrator, signalingBaseURL)

	server := agent.NewHTTPServer(orchestrator, stateManager, ackTracker, p2pManager)

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
