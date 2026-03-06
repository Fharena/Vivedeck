package signaling

import (
    "encoding/json"
    "errors"
    "log"
    "net/http"
    "strings"
    "sync"
    "time"

    "github.com/Fharena/Vivedeck/internal/protocol"
    "github.com/gorilla/websocket"
)

type Server struct {
    store    *Store
    upgrader websocket.Upgrader

    mu    sync.Mutex
    rooms map[string]*room
}

type room struct {
    pcConn     *websocket.Conn
    mobileConn *websocket.Conn
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

    conn, err := s.upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Printf("signaling ws upgrade failed: %v", err)
        return
    }

    s.attachConn(sessionID, role, conn)
    go s.readLoop(sessionID, role, conn)
}

func (s *Server) attachConn(sessionID string, role Role, conn *websocket.Conn) {
    s.mu.Lock()
    defer s.mu.Unlock()

    rm, ok := s.rooms[sessionID]
    if !ok {
        rm = &room{}
        s.rooms[sessionID] = rm
    }

    if role == RolePC {
        if rm.pcConn != nil {
            _ = rm.pcConn.Close()
        }
        rm.pcConn = conn
        return
    }

    if rm.mobileConn != nil {
        _ = rm.mobileConn.Close()
    }
    rm.mobileConn = conn
}

func (s *Server) detachConn(sessionID string, role Role, conn *websocket.Conn) {
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

func (s *Server) readLoop(sessionID string, role Role, conn *websocket.Conn) {
    defer func() {
        s.detachConn(sessionID, role, conn)
        _ = conn.Close()
    }()

    _ = conn.SetReadDeadline(time.Time{})

    for {
        var env protocol.Envelope
        if err := conn.ReadJSON(&env); err != nil {
            return
        }

        if err := env.Validate(); err != nil {
            continue
        }

        s.forward(sessionID, role, env)
    }
}

func (s *Server) forward(sessionID string, role Role, env protocol.Envelope) {
    s.mu.Lock()
    defer s.mu.Unlock()

    rm, ok := s.rooms[sessionID]
    if !ok {
        return
    }

    var target *websocket.Conn
    if role == RolePC {
        target = rm.mobileConn
    } else {
        target = rm.pcConn
    }

    if target == nil {
        return
    }

    _ = target.SetWriteDeadline(time.Now().Add(2 * time.Second))
    _ = target.WriteJSON(env)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(data)
}
