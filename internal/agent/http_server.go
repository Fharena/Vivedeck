package agent

import (
    "context"
    "encoding/json"
    "net/http"
    "time"

    "github.com/Fharena/Vivedeck/internal/protocol"
)

type HTTPServer struct {
    orchestrator *Orchestrator
}

func NewHTTPServer(orchestrator *Orchestrator) *HTTPServer {
    return &HTTPServer{orchestrator: orchestrator}
}

func (s *HTTPServer) Handler() http.Handler {
    mux := http.NewServeMux()
    mux.HandleFunc("/healthz", s.handleHealth)
    mux.HandleFunc("/v1/agent/envelope", s.handleEnvelope)
    return mux
}

func (s *HTTPServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
    writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *HTTPServer) handleEnvelope(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
        return
    }

    var env protocol.Envelope
    if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid envelope json"})
        return
    }

    ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
    defer cancel()

    responses, err := s.orchestrator.HandleEnvelope(ctx, env)
    if err != nil {
        writeJSON(w, http.StatusBadRequest, map[string]any{
            "error":     err.Error(),
            "responses": responses,
        })
        return
    }

    writeJSON(w, http.StatusOK, map[string]any{"responses": responses})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(data)
}
