package agent

import (
    "context"
    "encoding/json"
    "net/http"
    "strings"
    "time"

    "github.com/Fharena/Vivedeck/internal/protocol"
    "github.com/Fharena/Vivedeck/internal/runtime"
)

type HTTPServer struct {
    orchestrator *Orchestrator
    stateManager *runtime.StateManager
    ackTracker   *runtime.AckTracker
}

func NewHTTPServer(orchestrator *Orchestrator, stateManager *runtime.StateManager, ackTracker *runtime.AckTracker) *HTTPServer {
    return &HTTPServer{
        orchestrator: orchestrator,
        stateManager: stateManager,
        ackTracker:   ackTracker,
    }
}

func (s *HTTPServer) Handler() http.Handler {
    mux := http.NewServeMux()
    mux.HandleFunc("/healthz", s.handleHealth)
    mux.HandleFunc("/v1/agent/envelope", s.handleEnvelope)
    mux.HandleFunc("/v1/agent/runtime/state", s.handleRuntimeState)
    mux.HandleFunc("/v1/agent/runtime/acks/expired", s.handleExpiredAcks)
    mux.HandleFunc("/v1/agent/runtime/acks/pending", s.handlePendingAcks)
    return mux
}

func (s *HTTPServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
    writeJSON(w, http.StatusOK, map[string]any{
        "status":      "ok",
        "state":       s.stateManager.State(),
        "pendingAcks": s.ackTracker.PendingCount(),
    })
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

    if env.Type == protocol.TypeCmdAck {
        s.handleInboundAckEnvelope(w, env)
        return
    }

    ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
    defer cancel()

    responses, err := s.orchestrator.HandleEnvelope(ctx, env)
    s.registerOutboundAcks(env.SID, responses)

    if err != nil {
        writeJSON(w, http.StatusBadRequest, map[string]any{
            "error":     err.Error(),
            "responses": responses,
        })
        return
    }

    writeJSON(w, http.StatusOK, map[string]any{"responses": responses})
}

func (s *HTTPServer) handleInboundAckEnvelope(w http.ResponseWriter, env protocol.Envelope) {
    var payload protocol.CmdAckPayload
    if err := env.DecodePayload(&payload); err != nil {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid CMD_ACK payload"})
        return
    }

    handled := s.ackTracker.Ack(payload.RequestRID)
    writeJSON(w, http.StatusOK, map[string]any{
        "handled":     handled,
        "requestRid":  payload.RequestRID,
        "pendingAcks": s.ackTracker.PendingCount(),
    })
}

func (s *HTTPServer) handleRuntimeState(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        writeJSON(w, http.StatusOK, map[string]any{
            "state":   s.stateManager.State(),
            "history": s.stateManager.History(),
        })

    case http.MethodPost:
        var req struct {
            Action string `json:"action"`
            Note   string `json:"note,omitempty"`
        }

        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid runtime state request"})
            return
        }

        action := strings.ToLower(strings.TrimSpace(req.Action))
        switch action {
        case "begin_signaling":
            s.stateManager.BeginSignaling()
        case "begin_p2p":
            s.stateManager.BeginP2P()
        case "p2p_connected":
            s.stateManager.MarkP2PConnected()
        case "relay_connected":
            s.stateManager.MarkRelayConnected(req.Note)
        case "reconnecting":
            s.stateManager.BeginReconnect()
        case "close":
            s.stateManager.Close()
        default:
            writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown action"})
            return
        }

        writeJSON(w, http.StatusOK, map[string]any{
            "state":   s.stateManager.State(),
            "history": s.stateManager.History(),
        })

    default:
        writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
    }
}

func (s *HTTPServer) handleExpiredAcks(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
        return
    }

    expired := s.ackTracker.Expired()
    if len(expired) > 0 {
        s.stateManager.BeginReconnect()
    }

    writeJSON(w, http.StatusOK, map[string]any{
        "expired": expired,
        "state":   s.stateManager.State(),
    })
}

func (s *HTTPServer) handlePendingAcks(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
        return
    }

    writeJSON(w, http.StatusOK, map[string]any{
        "pending": s.ackTracker.Snapshot(),
        "count":   s.ackTracker.PendingCount(),
    })
}

func (s *HTTPServer) registerOutboundAcks(sid string, responses []protocol.Envelope) {
    for _, response := range responses {
        if response.Type == protocol.TypeCmdAck {
            continue
        }

        s.ackTracker.Register(sid, response.RID, string(response.Type))
    }
}

func writeJSON(w http.ResponseWriter, status int, data any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(data)
}
