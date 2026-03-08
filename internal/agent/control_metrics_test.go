package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Fharena/Vivedeck/internal/protocol"
)

func TestControlMetricsSnapshot(t *testing.T) {
	metrics := NewControlMetrics()
	metrics.Observe(protocol.TypePromptSubmit, ControlPathHTTP, 120*time.Millisecond, nil)
	metrics.Observe(protocol.TypePromptSubmit, ControlPathHTTP, 80*time.Millisecond, context.DeadlineExceeded)
	metrics.Observe(protocol.TypePatchApply, ControlPathP2P, 40*time.Millisecond, errors.New("boom"))

	snapshot := metrics.Snapshot()
	if snapshot.Totals.Requests != 3 {
		t.Fatalf("expected total requests 3, got %d", snapshot.Totals.Requests)
	}
	if snapshot.Totals.Successes != 1 {
		t.Fatalf("expected total successes 1, got %d", snapshot.Totals.Successes)
	}
	if snapshot.Totals.Timeouts != 1 {
		t.Fatalf("expected total timeouts 1, got %d", snapshot.Totals.Timeouts)
	}
	if snapshot.Totals.Failures != 1 {
		t.Fatalf("expected total failures 1, got %d", snapshot.Totals.Failures)
	}
	if snapshot.Totals.LastLatencyMs != 40 {
		t.Fatalf("expected last latency 40ms, got %d", snapshot.Totals.LastLatencyMs)
	}
	if snapshot.Totals.AvgLatencyMs != 80 {
		t.Fatalf("expected avg latency 80ms, got %d", snapshot.Totals.AvgLatencyMs)
	}
	if snapshot.Totals.MaxLatencyMs != 120 {
		t.Fatalf("expected max latency 120ms, got %d", snapshot.Totals.MaxLatencyMs)
	}

	httpStats := snapshot.ByPath[ControlPathHTTP]
	if httpStats.Requests != 2 || httpStats.Successes != 1 || httpStats.Timeouts != 1 {
		t.Fatalf("unexpected http stats: %+v", httpStats)
	}
	if snapshot.ByType[string(protocol.TypePromptSubmit)].Requests != 2 {
		t.Fatalf("expected prompt submit stats to show 2 requests, got %+v", snapshot.ByType)
	}
	if snapshot.ByTypePath[string(protocol.TypePatchApply)][ControlPathP2P].Failures != 1 {
		t.Fatalf("expected patch_apply p2p failure count 1, got %+v", snapshot.ByTypePath)
	}
}
