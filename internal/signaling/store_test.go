package signaling

import (
    "testing"
    "time"
)

func TestStoreCreateAndClaim(t *testing.T) {
    store := NewStore(1 * time.Minute)

    p, err := store.CreatePairing()
    if err != nil {
        t.Fatalf("create pairing: %v", err)
    }

    if p.Code == "" || p.SessionID == "" || p.PCDeviceKey == "" {
        t.Fatalf("pairing fields must be populated")
    }

    claimed, err := store.ClaimPairing(p.Code)
    if err != nil {
        t.Fatalf("claim pairing: %v", err)
    }

    if !claimed.Claimed || claimed.MobileDeviceKey == "" {
        t.Fatalf("mobile key must be generated on claim")
    }

    if _, err := store.ClaimPairing(p.Code); err == nil {
        t.Fatalf("second claim must fail")
    }
}

func TestStoreValidateSessionKey(t *testing.T) {
    store := NewStore(1 * time.Minute)
    p, err := store.CreatePairing()
    if err != nil {
        t.Fatalf("create pairing: %v", err)
    }

    p, err = store.ClaimPairing(p.Code)
    if err != nil {
        t.Fatalf("claim pairing: %v", err)
    }

    role, ok := store.ValidateSessionKey(p.SessionID, p.PCDeviceKey)
    if !ok || role != RolePC {
        t.Fatalf("pc key should validate as pc role")
    }

    role, ok = store.ValidateSessionKey(p.SessionID, p.MobileDeviceKey)
    if !ok || role != RoleMobile {
        t.Fatalf("mobile key should validate as mobile role")
    }

    if _, ok := store.ValidateSessionKey(p.SessionID, "wrong"); ok {
        t.Fatalf("wrong key should be rejected")
    }
}
