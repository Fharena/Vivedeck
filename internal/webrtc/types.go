package webrtc

import "time"

type Side string

const (
	SidePC     Side = "pc"
	SideMobile Side = "mobile"
)

type Config struct {
	Side             Side
	DataChannelLabel string
	OfferTimeout     time.Duration
}

func DefaultConfig(side Side) Config {
	return Config{
		Side:             side,
		DataChannelLabel: "vibedeck-control",
		OfferTimeout:     6 * time.Second,
	}
}

type PeerState string

const (
	StateNew          PeerState = "new"
	StateConnecting   PeerState = "connecting"
	StateConnected    PeerState = "connected"
	StateDisconnected PeerState = "disconnected"
	StateFailed       PeerState = "failed"
	StateClosed       PeerState = "closed"
)
