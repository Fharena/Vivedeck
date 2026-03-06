package signaling

import (
    "testing"

    "github.com/Fharena/Vivedeck/internal/protocol"
)

func TestValidateSignalEnvelopeRoleDirection(t *testing.T) {
    offer, err := protocol.NewEnvelope("sid1", "rid1", 1, protocol.TypeSignalOffer, protocol.SignalOfferPayload{SDP: "v=0"})
    if err != nil {
        t.Fatalf("build offer envelope: %v", err)
    }

    if err := validateSignalEnvelope("sid1", RolePC, offer); err != nil {
        t.Fatalf("pc should be allowed to send offer: %v", err)
    }

    if err := validateSignalEnvelope("sid1", RoleMobile, offer); err == nil {
        t.Fatalf("mobile should not be allowed to send offer")
    }
}

func TestValidateSignalEnvelopePayload(t *testing.T) {
    badIce, err := protocol.NewEnvelope("sid1", "rid2", 2, protocol.TypeSignalICE, map[string]string{"candidate": ""})
    if err != nil {
        t.Fatalf("build ice envelope: %v", err)
    }

    if err := validateSignalEnvelope("sid1", RolePC, badIce); err == nil {
        t.Fatalf("empty candidate should fail validation")
    }
}
