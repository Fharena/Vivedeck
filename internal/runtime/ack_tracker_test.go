package runtime

import (
    "testing"
    "time"
)

func TestAckTrackerRegisterAck(t *testing.T) {
    tracker := NewAckTracker(50 * time.Millisecond)
    tracker.Register("sid1", "rid1", "PROMPT_SUBMIT")

    if tracker.PendingCount() != 1 {
        t.Fatalf("pending count should be 1")
    }

    ok := tracker.Ack("rid1")
    if !ok {
        t.Fatalf("ack should succeed")
    }

    if tracker.PendingCount() != 0 {
        t.Fatalf("pending count should be 0")
    }
}

func TestAckTrackerExpired(t *testing.T) {
    tracker := NewAckTracker(20 * time.Millisecond)
    tracker.Register("sid1", "rid-timeout", "PATCH_APPLY")

    time.Sleep(30 * time.Millisecond)
    expired := tracker.Expired()

    if len(expired) != 1 {
        t.Fatalf("expected one expired ack, got %d", len(expired))
    }

    if expired[0].RID != "rid-timeout" {
        t.Fatalf("unexpected expired rid: %s", expired[0].RID)
    }
}
