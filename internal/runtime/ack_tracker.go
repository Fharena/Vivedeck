package runtime

import (
    "sync"
    "time"
)

type PendingAck struct {
    SID          string    `json:"sid"`
    RID          string    `json:"rid"`
    MessageType  string    `json:"messageType"`
    RegisteredAt time.Time `json:"registeredAt"`
    ExpiresAt    time.Time `json:"expiresAt"`
}

type AckTracker struct {
    timeout time.Duration
    now     func() time.Time

    mu      sync.Mutex
    pending map[string]PendingAck
}

func NewAckTracker(timeout time.Duration) *AckTracker {
    if timeout <= 0 {
        timeout = 2 * time.Second
    }

    return &AckTracker{
        timeout: timeout,
        now:     time.Now,
        pending: make(map[string]PendingAck),
    }
}

func (t *AckTracker) Register(sid, rid, messageType string) {
    t.mu.Lock()
    defer t.mu.Unlock()

    now := t.now().UTC()
    t.pending[rid] = PendingAck{
        SID:          sid,
        RID:          rid,
        MessageType:  messageType,
        RegisteredAt: now,
        ExpiresAt:    now.Add(t.timeout),
    }
}

func (t *AckTracker) Ack(rid string) bool {
    t.mu.Lock()
    defer t.mu.Unlock()

    if _, ok := t.pending[rid]; !ok {
        return false
    }

    delete(t.pending, rid)
    return true
}

func (t *AckTracker) PendingCount() int {
    t.mu.Lock()
    defer t.mu.Unlock()

    return len(t.pending)
}

func (t *AckTracker) Expired() []PendingAck {
    t.mu.Lock()
    defer t.mu.Unlock()

    now := t.now().UTC()
    expired := make([]PendingAck, 0)

    for rid, pending := range t.pending {
        if now.After(pending.ExpiresAt) {
            expired = append(expired, pending)
            delete(t.pending, rid)
        }
    }

    return expired
}
