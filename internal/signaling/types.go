package signaling

import "time"

type Pairing struct {
    Code          string    `json:"code"`
    SessionID     string    `json:"sessionId"`
    PCDeviceKey   string    `json:"pcDeviceKey"`
    MobileDeviceKey string  `json:"mobileDeviceKey,omitempty"`
    CreatedAt     time.Time `json:"createdAt"`
    ExpiresAt     time.Time `json:"expiresAt"`
    Claimed       bool      `json:"claimed"`
}

type Role string

const (
    RolePC     Role = "pc"
    RoleMobile Role = "mobile"
)
