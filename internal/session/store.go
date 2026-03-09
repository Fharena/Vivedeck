package session

import (
    "sync"

    "github.com/Fharena/VibeDeck/internal/protocol"
)

type Session struct {
    ID        string
    Transport protocol.TransportMode
    State     protocol.SessionState
}

type Store struct {
    mu       sync.RWMutex
    sessions map[string]Session
}

func NewStore() *Store {
    return &Store{sessions: make(map[string]Session)}
}

func (s *Store) Upsert(session Session) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.sessions[session.ID] = session
}

func (s *Store) Get(id string) (Session, bool) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    session, ok := s.sessions[id]
    return session, ok
}

func (s *Store) SetState(id string, state protocol.SessionState) bool {
    s.mu.Lock()
    defer s.mu.Unlock()

    session, ok := s.sessions[id]
    if !ok {
        return false
    }

    session.State = state
    s.sessions[id] = session
    return true
}
