package runtime

import (
    "sync"
    "time"
)

type StateManager struct {
    cfg ManagerConfig

    now func() time.Time

    mu      sync.RWMutex
    state   ConnectionState
    history []Transition

    p2pTimer *time.Timer
}

func NewStateManager(cfg ManagerConfig) *StateManager {
    if cfg.P2PTimeout <= 0 {
        cfg.P2PTimeout = DefaultManagerConfig().P2PTimeout
    }
    if cfg.RelayFallbackDelay < 0 {
        cfg.RelayFallbackDelay = 0
    }

    m := &StateManager{
        cfg:     cfg,
        now:     time.Now,
        state:   StatePairing,
        history: make([]Transition, 0, 16),
    }
    m.appendLocked(StatePairing, "session created")
    return m
}

func (m *StateManager) State() ConnectionState {
    m.mu.RLock()
    defer m.mu.RUnlock()
    return m.state
}

func (m *StateManager) History() []Transition {
    m.mu.RLock()
    defer m.mu.RUnlock()

    copied := make([]Transition, len(m.history))
    copy(copied, m.history)
    return copied
}

func (m *StateManager) BeginSignaling() {
    m.setState(StateSignaling, "signaling started")
}

func (m *StateManager) BeginP2P() {
    m.mu.Lock()
    defer m.mu.Unlock()

    m.setStateLocked(StateP2PConnecting, "attempting p2p connection")
    m.stopP2PTimerLocked()

    m.p2pTimer = time.AfterFunc(m.cfg.P2PTimeout, func() {
        m.onP2PTimeout()
    })
}

func (m *StateManager) MarkP2PConnected() {
    m.mu.Lock()
    defer m.mu.Unlock()

    m.stopP2PTimerLocked()
    m.setStateLocked(StateP2PConnected, "p2p connected")
}

func (m *StateManager) MarkRelayConnected(reason string) {
    if reason == "" {
        reason = "relay connected"
    }

    m.mu.Lock()
    defer m.mu.Unlock()

    m.stopP2PTimerLocked()
    m.setStateLocked(StateRelayConnected, reason)
}

func (m *StateManager) BeginReconnect() {
    m.mu.Lock()
    defer m.mu.Unlock()

    m.stopP2PTimerLocked()
    m.setStateLocked(StateReconnecting, "reconnecting")
}

func (m *StateManager) Close() {
    m.mu.Lock()
    defer m.mu.Unlock()

    m.stopP2PTimerLocked()
    m.setStateLocked(StateClosed, "closed")
}

func (m *StateManager) onP2PTimeout() {
    m.mu.Lock()
    defer m.mu.Unlock()

    if m.state != StateP2PConnecting {
        return
    }

    if m.cfg.RelayFallbackDelay > 0 {
        m.setStateLocked(StateReconnecting, "p2p timeout, scheduling relay fallback")
        delay := m.cfg.RelayFallbackDelay
        time.AfterFunc(delay, func() {
            m.MarkRelayConnected("relay fallback after p2p timeout")
        })
        return
    }

    m.setStateLocked(StateRelayConnected, "relay fallback after p2p timeout")
}

func (m *StateManager) setState(next ConnectionState, note string) {
    m.mu.Lock()
    defer m.mu.Unlock()

    m.setStateLocked(next, note)
}

func (m *StateManager) setStateLocked(next ConnectionState, note string) {
    if m.state == next {
        return
    }

    m.state = next
    m.appendLocked(next, note)
}

func (m *StateManager) appendLocked(state ConnectionState, note string) {
    m.history = append(m.history, Transition{
        State: state,
        At:    m.now().UnixMilli(),
        Note:  note,
    })
}

func (m *StateManager) stopP2PTimerLocked() {
    if m.p2pTimer != nil {
        m.p2pTimer.Stop()
        m.p2pTimer = nil
    }
}
