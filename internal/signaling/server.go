package signaling

import (
    "encoding/json"
    "errors"
    "fmt"
    "log"
    "net/http"
    "strings"
    "sync"
    "time"

    "github.com/Fharena/VibeDeck/internal/protocol"
    "github.com/gorilla/websocket"
)

const maxPendingSignals = 128

type Server struct {
    store    *Store
    upgrader websocket.Upgrader

    mu    sync.Mutex
    rooms map[string]*room
}

type peerConn struct {
    conn    *websocket.Conn
    writeMu sync.Mutex
}

func (p *peerConn) WriteJSONWithTimeout(v any, timeout time.Duration) error {
    p.writeMu.Lock()
    defer p.writeMu.Unlock()

    _ = p.conn.SetWriteDeadline(time.Now().Add(timeout))
    return p.conn.WriteJSON(v)
}

func (p *peerConn) Close() error {
    p.writeMu.Lock()
    defer p.writeMu.Unlock()
    return p.conn.Close()
}

type room struct {
    pcConn     *peerConn
    mobileConn *peerConn

    pendingForPC     []protocol.Envelope
    pendingForMobile []protocol.Envelope
}

func NewServer(store *Store) *Server {
    return &Server{
        store: store,
        upgrader: websocket.Upgrader{
            ReadBufferSize:  1024,
            WriteBufferSize: 1024,
            CheckOrigin: func(r *http.Request) bool {
                return true
            },
        },
        rooms: make(map[string]*room),
    }
}

func (s *Server) Handler() http.Handler {
    mux := http.NewServeMux()
    mux.HandleFunc("/healthz", s.handleHealth)
    mux.HandleFunc("/v1/pairings", s.handlePairings)
    mux.HandleFunc("/v1/pairings/", s.handlePairingClaim)
    mux.HandleFunc("/v1/sessions/", s.handleSessionWS)
    return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
    writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handlePairings(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
        return
    }

    pairing, err := s.store.CreatePairing()
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
        return
    }

    writeJSON(w, http.StatusCreated, map[string]any{
        "code":        pairing.Code,
        "sessionId":   pairing.SessionID,
        "pcDeviceKey": pairing.PCDeviceKey,
        "expiresAt":   pairing.ExpiresAt.UnixMilli(),
    })
}

func (s *Server) handlePairingClaim(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
        return
    }

    // /v1/pairings/{code}/claim
    segments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
    if len(segments) != 4 || segments[0] != "v1" || segments[1] != "pairings" || segments[3] != "claim" {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "invalid claim path"})
        return
    }

    code := segments[2]
    pairing, err := s.store.ClaimPairing(code)
    if err != nil {
        switch {
        case errors.Is(err, ErrPairingNotFound):
            writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
        case errors.Is(err, ErrPairingExpired), errors.Is(err, ErrPairingClaimed):
            writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
        default:
            writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
        }
        return
    }

    writeJSON(w, http.StatusOK, map[string]any{
        "sessionId":       pairing.SessionID,
        "mobileDeviceKey": pairing.MobileDeviceKey,
        "expiresAt":       pairing.ExpiresAt.UnixMilli(),
    })
}

func (s *Server) handleSessionWS(w http.ResponseWriter, r *http.Request) {
    if !strings.HasSuffix(r.URL.Path, "/ws") {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
        return
    }

    // /v1/sessions/{sessionID}/ws
    segments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
    if len(segments) != 4 || segments[0] != "v1" || segments[1] != "sessions" || segments[3] != "ws" {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "invalid session path"})
        return
    }

    sessionID := segments[2]
    key := strings.TrimSpace(r.URL.Query().Get("deviceKey"))
    if key == "" {
        writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "deviceKey is required"})
        return
    }

    role, ok := s.store.ValidateSessionKey(sessionID, key)
    if !ok {
        writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid session or key"})
        return
    }

    requestedRole := Role(strings.TrimSpace(r.URL.Query().Get("role")))
    if requestedRole != "" && requestedRole != role {
        writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "role mismatch"})
        return
    }

    wsConn, err := s.upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Printf("signaling ws upgrade failed: %v", err)
        return
    }

    conn := &peerConn{conn: wsConn}
    pending, readyTargets := s.attachConn(sessionID, role, conn)

    // Flush queued signaling packets for the new peer first.
    for _, env := range pending {
        _ = conn.WriteJSONWithTimeout(env, 2*time.Second)
    }

    // Notify both peers that signaling room is fully ready.
    for _, target := range readyTargets {
        ready, err := buildSignalReadyEnvelope(sessionID, role)
        if err != nil {
            continue
        }
        _ = target.WriteJSONWithTimeout(ready, 2*time.Second)
    }

    go s.readLoop(sessionID, role, conn)
}

func (s *Server) attachConn(sessionID string, role Role, conn *peerConn) ([]protocol.Envelope, []*peerConn) {
    s.mu.Lock()
    defer s.mu.Unlock()

    rm, ok := s.rooms[sessionID]
    if !ok {
        rm = &room{}
        s.rooms[sessionID] = rm
    }

    var old *peerConn
    var pending []protocol.Envelope

    switch role {
    case RolePC:
        old = rm.pcConn
        rm.pcConn = conn
        if len(rm.pendingForPC) > 0 {
            pending = append(pending, rm.pendingForPC...)
            rm.pendingForPC = nil
        }

    case RoleMobile:
        old = rm.mobileConn
        rm.mobileConn = conn
        if len(rm.pendingForMobile) > 0 {
            pending = append(pending, rm.pendingForMobile...)
            rm.pendingForMobile = nil
        }
    }

    if old != nil {
        _ = old.Close()
    }

    if rm.pcConn != nil && rm.mobileConn != nil {
        return pending, []*peerConn{rm.pcConn, rm.mobileConn}
    }

    return pending, nil
}

func (s *Server) detachConn(sessionID string, role Role, conn *peerConn) {
    s.mu.Lock()
    defer s.mu.Unlock()

    rm, ok := s.rooms[sessionID]
    if !ok {
        return
    }

    if role == RolePC && rm.pcConn == conn {
        rm.pcConn = nil
    }
    if role == RoleMobile && rm.mobileConn == conn {
        rm.mobileConn = nil
    }

    if rm.pcConn == nil && rm.mobileConn == nil {
        delete(s.rooms, sessionID)
    }
}

func (s *Server) readLoop(sessionID string, role Role, conn *peerConn) {
    defer func() {
        s.detachConn(sessionID, role, conn)
        _ = conn.Close()
    }()

    _ = conn.conn.SetReadDeadline(time.Time{})

    for {
        var env protocol.Envelope
        if err := conn.conn.ReadJSON(&env); err != nil {
            return
        }

        if err := env.Validate(); err != nil {
            s.sendServerAck(sessionID, conn, env.RID, false, "invalid envelope")
            continue
        }

        if err := validateSignalEnvelope(sessionID, role, env); err != nil {
            s.sendServerAck(sessionID, conn, env.RID, false, err.Error())
            continue
        }

        if err := s.forwardOrQueue(sessionID, role, env); err != nil {
            s.sendServerAck(sessionID, conn, env.RID, false, err.Error())
            continue
        }

        s.sendServerAck(sessionID, conn, env.RID, true, "accepted")
    }
}

func (s *Server) forwardOrQueue(sessionID string, role Role, env protocol.Envelope) error {
    s.mu.Lock()
    rm, ok := s.rooms[sessionID]
    if !ok {
        s.mu.Unlock()
        return fmt.Errorf("room %s not found", sessionID)
    }

    var target *peerConn
    if role == RolePC {
        target = rm.mobileConn
        if target == nil {
            rm.pendingForMobile = appendPending(rm.pendingForMobile, env)
            s.mu.Unlock()
            return nil
        }
    } else {
        target = rm.pcConn
        if target == nil {
            rm.pendingForPC = appendPending(rm.pendingForPC, env)
            s.mu.Unlock()
            return nil
        }
    }
    s.mu.Unlock()

    return target.WriteJSONWithTimeout(env, 2*time.Second)
}

func appendPending(queue []protocol.Envelope, env protocol.Envelope) []protocol.Envelope {
    if len(queue) >= maxPendingSignals {
        queue = queue[1:]
    }

    return append(queue, env)
}

func (s *Server) sendServerAck(sessionID string, conn *peerConn, requestRID string, accepted bool, message string) {
    ack, err := protocol.NewEnvelope(
        sessionID,
        fmt.Sprintf("srv_ack_%d", time.Now().UTC().UnixNano()),
        time.Now().UTC().UnixMilli(),
        protocol.TypeCmdAck,
        protocol.CmdAckPayload{
            RequestRID: requestRID,
            Accepted:   accepted,
            Message:    message,
        },
    )
    if err != nil {
        return
    }

    _ = conn.WriteJSONWithTimeout(ack, 2*time.Second)
}

func buildSignalReadyEnvelope(sessionID string, role Role) (protocol.Envelope, error) {
    return protocol.NewEnvelope(
        sessionID,
        fmt.Sprintf("signal_ready_%d", time.Now().UTC().UnixNano()),
        time.Now().UTC().UnixMilli(),
        protocol.TypeSignalReady,
        protocol.SignalReadyPayload{
            Role:          string(role),
            PeerConnected: true,
            Timestamp:     time.Now().UTC().UnixMilli(),
        },
    )
}

func writeJSON(w http.ResponseWriter, status int, data any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(data)
}
