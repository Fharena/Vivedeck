package signaling

import (
    "crypto/rand"
    "encoding/base32"
    "errors"
    "fmt"
    "strings"
    "sync"
    "time"
)

var (
    ErrPairingNotFound = errors.New("pairing not found")
    ErrPairingExpired  = errors.New("pairing expired")
    ErrPairingClaimed  = errors.New("pairing already claimed")
    ErrInvalidRole     = errors.New("invalid role")
)

type Store struct {
    mu         sync.RWMutex
    ttl        time.Duration
    byCode     map[string]*Pairing
    bySession  map[string]*Pairing
}

func NewStore(ttl time.Duration) *Store {
    return &Store{
        ttl:       ttl,
        byCode:    make(map[string]*Pairing),
        bySession: make(map[string]*Pairing),
    }
}

func (s *Store) CreatePairing() (Pairing, error) {
    s.mu.Lock()
    defer s.mu.Unlock()

    if err := s.evictExpiredLocked(time.Now()); err != nil {
        return Pairing{}, err
    }

    code, err := randomToken(4)
    if err != nil {
        return Pairing{}, fmt.Errorf("generate pairing code: %w", err)
    }

    sessionID, err := randomToken(8)
    if err != nil {
        return Pairing{}, fmt.Errorf("generate session id: %w", err)
    }

    pcKey, err := randomToken(10)
    if err != nil {
        return Pairing{}, fmt.Errorf("generate device key: %w", err)
    }

    now := time.Now().UTC()
    p := &Pairing{
        Code:        strings.ToUpper(code),
        SessionID:   sessionID,
        PCDeviceKey: pcKey,
        CreatedAt:   now,
        ExpiresAt:   now.Add(s.ttl),
        Claimed:     false,
    }

    s.byCode[p.Code] = p
    s.bySession[p.SessionID] = p
    return *p, nil
}

func (s *Store) ClaimPairing(code string) (Pairing, error) {
    s.mu.Lock()
    defer s.mu.Unlock()

    normalized := strings.ToUpper(strings.TrimSpace(code))
    p, ok := s.byCode[normalized]
    if !ok {
        return Pairing{}, ErrPairingNotFound
    }

    if time.Now().After(p.ExpiresAt) {
        delete(s.byCode, normalized)
        delete(s.bySession, p.SessionID)
        return Pairing{}, ErrPairingExpired
    }

    if p.Claimed {
        return Pairing{}, ErrPairingClaimed
    }

    mobileKey, err := randomToken(10)
    if err != nil {
        return Pairing{}, fmt.Errorf("generate mobile key: %w", err)
    }

    p.MobileDeviceKey = mobileKey
    p.Claimed = true
    return *p, nil
}

func (s *Store) ValidateSessionKey(sessionID, deviceKey string) (Role, bool) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    p, ok := s.bySession[sessionID]
    if !ok {
        return "", false
    }

    if time.Now().After(p.ExpiresAt) {
        return "", false
    }

    switch deviceKey {
    case p.PCDeviceKey:
        return RolePC, true
    case p.MobileDeviceKey:
        return RoleMobile, true
    default:
        return "", false
    }
}

func (s *Store) evictExpiredLocked(now time.Time) error {
    for code, p := range s.byCode {
        if now.After(p.ExpiresAt) {
            delete(s.byCode, code)
            delete(s.bySession, p.SessionID)
        }
    }
    return nil
}

func randomToken(numBytes int) (string, error) {
    buf := make([]byte, numBytes)
    if _, err := rand.Read(buf); err != nil {
        return "", err
    }

    out := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)
    return strings.ToLower(out), nil
}
