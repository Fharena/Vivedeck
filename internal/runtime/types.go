package runtime

import "time"

type ConnectionState string

const (
    StatePairing       ConnectionState = "PAIRING"
    StateSignaling     ConnectionState = "SIGNALING"
    StateP2PConnecting ConnectionState = "P2P_CONNECTING"
    StateP2PConnected  ConnectionState = "P2P_CONNECTED"
    StateRelayConnected ConnectionState = "RELAY_CONNECTED"
    StateReconnecting  ConnectionState = "RECONNECTING"
    StateClosed        ConnectionState = "CLOSED"
)

type Transition struct {
    State ConnectionState `json:"state"`
    At    int64           `json:"at"`
    Note  string          `json:"note,omitempty"`
}

type ManagerConfig struct {
    P2PTimeout         time.Duration
    RelayFallbackDelay time.Duration
}

func DefaultManagerConfig() ManagerConfig {
    return ManagerConfig{
        P2PTimeout:         3 * time.Second,
        RelayFallbackDelay: 100 * time.Millisecond,
    }
}
