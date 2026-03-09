package relay

import (
    "fmt"
    "sync"
    "time"

    "github.com/Fharena/VibeDeck/internal/protocol"
)

type Session struct {
    id        string
    queueSize int

    mu          sync.RWMutex
    peers       map[string]chan protocol.Envelope
    droppedTerm map[string]int
}

func NewSession(id string, queueSize int) *Session {
    return &Session{
        id:          id,
        queueSize:   queueSize,
        peers:       make(map[string]chan protocol.Envelope),
        droppedTerm: make(map[string]int),
    }
}

func (s *Session) Register(peer string) <-chan protocol.Envelope {
    s.mu.Lock()
    defer s.mu.Unlock()

    ch := make(chan protocol.Envelope, s.queueSize)
    if old, ok := s.peers[peer]; ok {
        close(old)
    }
    s.peers[peer] = ch
    return ch
}

func (s *Session) Unregister(peer string) {
    s.mu.Lock()
    defer s.mu.Unlock()

    if ch, ok := s.peers[peer]; ok {
        delete(s.peers, peer)
        delete(s.droppedTerm, peer)
        close(ch)
    }
}

func (s *Session) PeerCount() int {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return len(s.peers)
}

func (s *Session) Route(from string, env protocol.Envelope) error {
    s.mu.RLock()
    targets := make([]string, 0, len(s.peers))
    for peer := range s.peers {
        if peer == from {
            continue
        }
        targets = append(targets, peer)
    }
    s.mu.RUnlock()

    for _, target := range targets {
        if err := s.routeToPeer(target, env); err != nil {
            return err
        }
    }

    return nil
}

func (s *Session) routeToPeer(target string, env protocol.Envelope) error {
    s.mu.RLock()
    ch, ok := s.peers[target]
    s.mu.RUnlock()
    if !ok {
        return nil
    }

    if env.Type == protocol.TypeTerm {
        s.tryEmitTermSummary(ch, env, target)

        select {
        case ch <- env:
            return nil
        default:
            s.incrementDropCounter(target)
            return nil
        }
    }

    // Control-path packets should not be dropped easily.
    select {
    case ch <- env:
        return nil
    case <-time.After(800 * time.Millisecond):
        return fmt.Errorf("control packet timeout for target=%s", target)
    }
}

func (s *Session) tryEmitTermSummary(ch chan protocol.Envelope, env protocol.Envelope, target string) {
    dropped := s.currentDropCounter(target)
    if dropped == 0 {
        return
    }

    summary, err := protocol.NewEnvelope(
        env.SID,
        env.RID+"_term_summary",
        env.Seq,
        protocol.TypeTermSummary,
        protocol.TermSummaryPayload{SkippedLines: dropped},
    )
    if err != nil {
        return
    }

    select {
    case ch <- summary:
        s.resetDropCounter(target)
    default:
    }
}

func (s *Session) incrementDropCounter(target string) int {
    s.mu.Lock()
    defer s.mu.Unlock()

    s.droppedTerm[target]++
    return s.droppedTerm[target]
}

func (s *Session) currentDropCounter(target string) int {
    s.mu.RLock()
    defer s.mu.RUnlock()

    return s.droppedTerm[target]
}

func (s *Session) resetDropCounter(target string) {
    s.mu.Lock()
    defer s.mu.Unlock()

    s.droppedTerm[target] = 0
}
