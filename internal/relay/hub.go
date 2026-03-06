package relay

import "sync"

type Hub struct {
    mu        sync.Mutex
    sessions  map[string]*Session
    queueSize int
}

func NewHub(queueSize int) *Hub {
    return &Hub{
        sessions:  make(map[string]*Session),
        queueSize: queueSize,
    }
}

func (h *Hub) GetOrCreate(sessionID string) *Session {
    h.mu.Lock()
    defer h.mu.Unlock()

    if s, ok := h.sessions[sessionID]; ok {
        return s
    }

    s := NewSession(sessionID, h.queueSize)
    h.sessions[sessionID] = s
    return s
}

func (h *Hub) RemoveIfEmpty(sessionID string) {
    h.mu.Lock()
    defer h.mu.Unlock()

    s, ok := h.sessions[sessionID]
    if !ok {
        return
    }

    if s.PeerCount() == 0 {
        delete(h.sessions, sessionID)
    }
}
