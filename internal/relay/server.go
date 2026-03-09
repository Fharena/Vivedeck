package relay

import (
    "encoding/json"
    "log"
    "net/http"
    "strconv"
    "strings"
    "time"

    "github.com/Fharena/VibeDeck/internal/protocol"
    "github.com/gorilla/websocket"
)

type Server struct {
    hub      *Hub
    upgrader websocket.Upgrader
}

func NewServer(queueSize int) *Server {
    return &Server{
        hub: NewHub(queueSize),
        upgrader: websocket.Upgrader{
            ReadBufferSize:  1024,
            WriteBufferSize: 1024,
            CheckOrigin: func(r *http.Request) bool {
                return true
            },
        },
    }
}

func (s *Server) Handler() http.Handler {
    mux := http.NewServeMux()
    mux.HandleFunc("/healthz", s.handleHealth)
    mux.HandleFunc("/v1/relay/", s.handleRelayWS)
    return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
    writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleRelayWS(w http.ResponseWriter, r *http.Request) {
    if !strings.HasSuffix(r.URL.Path, "/ws") {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
        return
    }

    // /v1/relay/{sessionID}/ws
    segments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
    if len(segments) != 4 || segments[0] != "v1" || segments[1] != "relay" || segments[3] != "ws" {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "invalid path"})
        return
    }

    sessionID := segments[2]
    peer := strings.TrimSpace(r.URL.Query().Get("peer"))
    if peer == "" {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "peer query is required"})
        return
    }

    conn, err := s.upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Printf("relay ws upgrade failed: %v", err)
        return
    }

    relaySession := s.hub.GetOrCreate(sessionID)
    outbound := relaySession.Register(peer)

    go s.writeLoop(conn, outbound)
    s.readLoop(relaySession, sessionID, peer, conn)
}

func (s *Server) readLoop(session *Session, sessionID, peer string, conn *websocket.Conn) {
    defer func() {
        session.Unregister(peer)
        s.hub.RemoveIfEmpty(sessionID)
        _ = conn.Close()
    }()

    for {
        var env protocol.Envelope
        if err := conn.ReadJSON(&env); err != nil {
            return
        }

        if err := env.Validate(); err != nil {
            continue
        }

        if err := session.Route(peer, env); err != nil {
            ack, ackErr := protocol.NewCmdAck(env.SID, env.Seq, env.RID, false, err.Error())
            if ackErr == nil {
                _ = conn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
                _ = conn.WriteJSON(ack)
            }
        }
    }
}

func (s *Server) writeLoop(conn *websocket.Conn, outbound <-chan protocol.Envelope) {
    for env := range outbound {
        _ = conn.SetWriteDeadline(time.Now().Add(1 * time.Second))
        if err := conn.WriteJSON(env); err != nil {
            return
        }
    }
}

func writeJSON(w http.ResponseWriter, status int, data any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(data)
}

func queueSizeFromEnv(raw string, fallback int) int {
    if raw == "" {
        return fallback
    }

    n, err := strconv.Atoi(raw)
    if err != nil || n <= 0 {
        return fallback
    }

    return n
}
