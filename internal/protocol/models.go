package protocol

type Hunk struct {
    HunkID string `json:"hunkId"`
    Header string `json:"header"`
    Diff   string `json:"diff"`
    Risk   string `json:"risk,omitempty"`
}

type FilePatch struct {
    Path   string `json:"path"`
    Status string `json:"status"`
    Hunks  []Hunk `json:"hunks"`
}

type PatchReadyPayload struct {
    JobID   string     `json:"jobId"`
    Summary string     `json:"summary"`
    Files   []FilePatch `json:"files"`
}

type TransportMode string

const (
    TransportP2P   TransportMode = "p2p"
    TransportRelay TransportMode = "relay"
    TransportLAN   TransportMode = "lan"
)

type SessionState string

const (
    SessionIdle         SessionState = "idle"
    SessionConnected    SessionState = "connected"
    SessionReconnecting SessionState = "reconnecting"
    SessionClosed       SessionState = "closed"
)
