package runtime

import (
    "testing"
    "time"
)

func TestStateManagerP2PTimeoutFallback(t *testing.T) {
    manager := NewStateManager(ManagerConfig{
        P2PTimeout:         20 * time.Millisecond,
        RelayFallbackDelay: 0,
    })

    manager.BeginSignaling()
    manager.BeginP2P()

    time.Sleep(40 * time.Millisecond)

    if manager.State() != StateRelayConnected {
        t.Fatalf("expected relay fallback state, got %s", manager.State())
    }
}

func TestStateManagerP2PSuccess(t *testing.T) {
    manager := NewStateManager(ManagerConfig{
        P2PTimeout:         100 * time.Millisecond,
        RelayFallbackDelay: 0,
    })

    manager.BeginSignaling()
    manager.BeginP2P()
    manager.MarkP2PConnected()

    time.Sleep(130 * time.Millisecond)

    if manager.State() != StateP2PConnected {
        t.Fatalf("state should remain p2p connected, got %s", manager.State())
    }
}

func TestStateManagerHistory(t *testing.T) {
    manager := NewStateManager(DefaultManagerConfig())

    manager.BeginSignaling()
    manager.BeginP2P()
    manager.BeginReconnect()
    manager.MarkRelayConnected("manual fallback")
    manager.Close()

    history := manager.History()
    if len(history) < 5 {
        t.Fatalf("expected state history entries, got %d", len(history))
    }

    if history[len(history)-1].State != StateClosed {
        t.Fatalf("last state should be closed")
    }
}
