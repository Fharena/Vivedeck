package runtime

import (
	"testing"
	"time"

	"github.com/Fharena/VibeDeck/internal/protocol"
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

func TestAckTrackerSnapshot(t *testing.T) {
	tracker := NewAckTracker(1 * time.Second)
	tracker.Register("sid1", "rid-a", "PATCH_READY")
	tracker.Register("sid1", "rid-b", "RUN_RESULT")

	snapshot := tracker.Snapshot()
	if len(snapshot) != 2 {
		t.Fatalf("snapshot size should be 2, got %d", len(snapshot))
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

func TestAckTrackerDueRetriesBackoffAndExhaustion(t *testing.T) {
	base := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	tracker := NewAckTrackerWithConfig(AckTrackerConfig{
		Timeout:           10 * time.Millisecond,
		MaxRetries:        2,
		BackoffMultiplier: 2,
	})
	tracker.now = func() time.Time { return base }

	env, err := protocol.NewEnvelope("sid1", "rid-retry", 1, protocol.TypePatchReady, map[string]any{
		"jobId":   "job_1",
		"summary": "mock patch",
		"files":   []any{},
	})
	if err != nil {
		t.Fatalf("build envelope: %v", err)
	}
	tracker.RegisterEnvelope(env, AckTransportP2P, true)

	base = base.Add(10 * time.Millisecond)
	first := tracker.DueRetries()
	if len(first.Retries) != 1 {
		t.Fatalf("expected first retry batch size 1, got %d", len(first.Retries))
	}
	if first.Retries[0].Pending.RetryCount != 1 {
		t.Fatalf("expected retry count 1, got %d", first.Retries[0].Pending.RetryCount)
	}
	if len(first.Exhausted) != 0 {
		t.Fatalf("did not expect exhausted entries on first retry")
	}

	snapshot := tracker.Snapshot()
	if len(snapshot) != 1 {
		t.Fatalf("expected one pending ack after first retry")
	}
	if got := snapshot[0].ExpiresAt.Sub(base); got != 20*time.Millisecond {
		t.Fatalf("expected second retry delay 20ms, got %s", got)
	}

	base = base.Add(20 * time.Millisecond)
	second := tracker.DueRetries()
	if len(second.Retries) != 1 {
		t.Fatalf("expected second retry batch size 1, got %d", len(second.Retries))
	}
	if second.Retries[0].Pending.RetryCount != 2 {
		t.Fatalf("expected retry count 2, got %d", second.Retries[0].Pending.RetryCount)
	}

	base = base.Add(40 * time.Millisecond)
	third := tracker.DueRetries()
	if len(third.Retries) != 0 {
		t.Fatalf("did not expect retry after max retries exhausted")
	}
	if len(third.Exhausted) != 1 {
		t.Fatalf("expected one exhausted ack, got %d", len(third.Exhausted))
	}
	if third.Exhausted[0].RID != "rid-retry" {
		t.Fatalf("unexpected exhausted rid: %s", third.Exhausted[0].RID)
	}
	if tracker.PendingCount() != 0 {
		t.Fatalf("expected pending count 0 after exhaustion")
	}
}

func TestAckTrackerExpiredSkipsRetryableBeforeExhaustion(t *testing.T) {
	base := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	tracker := NewAckTrackerWithConfig(AckTrackerConfig{
		Timeout:           10 * time.Millisecond,
		MaxRetries:        1,
		BackoffMultiplier: 2,
	})
	tracker.now = func() time.Time { return base }

	env, err := protocol.NewEnvelope("sid1", "rid-retryable", 1, protocol.TypeRunResult, map[string]any{
		"jobId":     "job_1",
		"profileId": "test_all",
		"status":    "failed",
		"summary":   "boom",
	})
	if err != nil {
		t.Fatalf("build envelope: %v", err)
	}
	tracker.RegisterEnvelope(env, AckTransportP2P, true)

	base = base.Add(10 * time.Millisecond)
	expired := tracker.Expired()
	if len(expired) != 0 {
		t.Fatalf("retryable ack should not expire before retries are exhausted")
	}
	if tracker.PendingCount() != 1 {
		t.Fatalf("pending retryable ack should remain tracked")
	}
}

func TestAckTrackerMetrics(t *testing.T) {
	base := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	tracker := NewAckTrackerWithConfig(AckTrackerConfig{
		Timeout:           10 * time.Millisecond,
		MaxRetries:        1,
		BackoffMultiplier: 2,
	})
	tracker.now = func() time.Time { return base }

	httpEnv, err := protocol.NewEnvelope("sid1", "rid-http", 1, protocol.TypePromptAck, map[string]any{
		"jobId": "job_1",
	})
	if err != nil {
		t.Fatalf("build http envelope: %v", err)
	}
	tracker.RegisterEnvelope(httpEnv, AckTransportHTTP, false)

	p2pEnv, err := protocol.NewEnvelope("sid1", "rid-p2p", 2, protocol.TypePatchReady, map[string]any{
		"jobId":   "job_1",
		"summary": "mock patch",
		"files":   []any{},
	})
	if err != nil {
		t.Fatalf("build p2p envelope: %v", err)
	}
	tracker.RegisterEnvelope(p2pEnv, AckTransportP2P, true)

	base = base.Add(4 * time.Millisecond)
	tracker.Ack("rid-http")

	base = base.Add(6 * time.Millisecond)
	retryBatch := tracker.DueRetries()
	if len(retryBatch.Retries) != 1 {
		t.Fatalf("expected one retry batch item, got %d", len(retryBatch.Retries))
	}

	tracker.Register("sid1", "rid-unknown", "PATCH_RESULT")

	base = base.Add(11 * time.Millisecond)
	expired := tracker.Expired()
	if len(expired) != 1 {
		t.Fatalf("expected one expired item, got %d", len(expired))
	}
	if expired[0].RID != "rid-unknown" {
		t.Fatalf("unexpected expired rid: %s", expired[0].RID)
	}

	base = base.Add(20 * time.Millisecond)
	exhausted := tracker.DueRetries()
	if len(exhausted.Exhausted) != 1 {
		t.Fatalf("expected one exhausted item, got %d", len(exhausted.Exhausted))
	}
	if exhausted.Exhausted[0].RID != "rid-p2p" {
		t.Fatalf("unexpected exhausted rid: %s", exhausted.Exhausted[0].RID)
	}

	metrics := tracker.Metrics()
	if metrics.PendingCount != 0 {
		t.Fatalf("expected pending count 0, got %d", metrics.PendingCount)
	}
	if metrics.MaxPendingCount != 2 {
		t.Fatalf("expected max pending count 2, got %d", metrics.MaxPendingCount)
	}
	if metrics.AckedCount != 1 {
		t.Fatalf("expected acked count 1, got %d", metrics.AckedCount)
	}
	if metrics.RetryDispatchCount != 1 {
		t.Fatalf("expected retry dispatch count 1, got %d", metrics.RetryDispatchCount)
	}
	if metrics.ExpiredCount != 1 {
		t.Fatalf("expected expired count 1, got %d", metrics.ExpiredCount)
	}
	if metrics.ExhaustedCount != 1 {
		t.Fatalf("expected exhausted count 1, got %d", metrics.ExhaustedCount)
	}
	if metrics.LastAckRTTMs != 4 {
		t.Fatalf("expected last ack RTT 4ms, got %d", metrics.LastAckRTTMs)
	}
	if metrics.AvgAckRTTMs != 4 {
		t.Fatalf("expected avg ack RTT 4ms, got %d", metrics.AvgAckRTTMs)
	}
	if metrics.MaxAckRTTMs != 4 {
		t.Fatalf("expected max ack RTT 4ms, got %d", metrics.MaxAckRTTMs)
	}
	if metrics.PendingByTransport[string(AckTransportHTTP)] != 0 {
		t.Fatalf("expected http pending 0")
	}
	if metrics.PendingByTransport[string(AckTransportP2P)] != 0 {
		t.Fatalf("expected p2p pending 0")
	}
	if metrics.PendingByTransport[string(AckTransportUnknown)] != 0 {
		t.Fatalf("expected unknown pending 0")
	}
}
