package agent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/Fharena/Vivedeck/internal/protocol"
)

const (
	ControlPathUnknown = "unknown"
	ControlPathHTTP    = "http"
	ControlPathP2P     = "p2p"
)

type ControlMetricStats struct {
	Requests      int   `json:"requests"`
	Successes     int   `json:"successes"`
	Failures      int   `json:"failures"`
	Timeouts      int   `json:"timeouts"`
	LastLatencyMs int64 `json:"lastLatencyMs"`
	AvgLatencyMs  int64 `json:"avgLatencyMs"`
	MaxLatencyMs  int64 `json:"maxLatencyMs"`
}

type ControlMetricsSnapshot struct {
	Totals     ControlMetricStats                       `json:"totals"`
	ByPath     map[string]ControlMetricStats            `json:"byPath"`
	ByType     map[string]ControlMetricStats            `json:"byType"`
	ByTypePath map[string]map[string]ControlMetricStats `json:"byTypePath"`
}

type controlMetricAccumulator struct {
	requests     int
	successes    int
	failures     int
	timeouts     int
	totalLatency time.Duration
	lastLatency  time.Duration
	maxLatency   time.Duration
}

type ControlMetrics struct {
	mu         sync.Mutex
	totals     controlMetricAccumulator
	byPath     map[string]*controlMetricAccumulator
	byType     map[string]*controlMetricAccumulator
	byTypePath map[string]map[string]*controlMetricAccumulator
}

func NewControlMetrics() *ControlMetrics {
	return &ControlMetrics{
		byPath: map[string]*controlMetricAccumulator{
			ControlPathUnknown: {},
			ControlPathHTTP:    {},
			ControlPathP2P:     {},
		},
		byType:     make(map[string]*controlMetricAccumulator),
		byTypePath: make(map[string]map[string]*controlMetricAccumulator),
	}
}

func EmptyControlMetricsSnapshot() ControlMetricsSnapshot {
	return ControlMetricsSnapshot{
		ByPath: map[string]ControlMetricStats{
			ControlPathUnknown: {},
			ControlPathHTTP:    {},
			ControlPathP2P:     {},
		},
		ByType:     make(map[string]ControlMetricStats),
		ByTypePath: make(map[string]map[string]ControlMetricStats),
	}
}

func (m *ControlMetrics) Observe(messageType protocol.MessageType, path string, duration time.Duration, err error) {
	if m == nil {
		return
	}

	path = normalizeControlPath(path)
	messageTypeLabel := normalizeControlMessageType(messageType)
	if duration < 0 {
		duration = 0
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.observeAccumulator(&m.totals, duration, err)
	m.observeAccumulator(m.pathAccumulator(path), duration, err)
	m.observeAccumulator(m.typeAccumulator(messageTypeLabel), duration, err)
	m.observeAccumulator(m.typePathAccumulator(messageTypeLabel, path), duration, err)
}

func (m *ControlMetrics) Snapshot() ControlMetricsSnapshot {
	if m == nil {
		return EmptyControlMetricsSnapshot()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	snapshot := EmptyControlMetricsSnapshot()
	snapshot.Totals = snapshotAccumulator(m.totals)

	for path, accumulator := range m.byPath {
		snapshot.ByPath[path] = snapshotAccumulator(*accumulator)
	}
	for messageType, accumulator := range m.byType {
		snapshot.ByType[messageType] = snapshotAccumulator(*accumulator)
	}
	for messageType, byPath := range m.byTypePath {
		next := make(map[string]ControlMetricStats, len(byPath))
		for path, accumulator := range byPath {
			next[path] = snapshotAccumulator(*accumulator)
		}
		snapshot.ByTypePath[messageType] = next
	}

	return snapshot
}

func (m *ControlMetrics) observeAccumulator(accumulator *controlMetricAccumulator, duration time.Duration, err error) {
	accumulator.requests++
	accumulator.totalLatency += duration
	accumulator.lastLatency = duration
	if duration > accumulator.maxLatency {
		accumulator.maxLatency = duration
	}

	switch {
	case err == nil:
		accumulator.successes++
	case errors.Is(err, context.DeadlineExceeded):
		accumulator.timeouts++
	default:
		accumulator.failures++
	}
}

func (m *ControlMetrics) pathAccumulator(path string) *controlMetricAccumulator {
	accumulator, ok := m.byPath[path]
	if ok {
		return accumulator
	}
	accumulator = &controlMetricAccumulator{}
	m.byPath[path] = accumulator
	return accumulator
}

func (m *ControlMetrics) typeAccumulator(messageType string) *controlMetricAccumulator {
	accumulator, ok := m.byType[messageType]
	if ok {
		return accumulator
	}
	accumulator = &controlMetricAccumulator{}
	m.byType[messageType] = accumulator
	return accumulator
}

func (m *ControlMetrics) typePathAccumulator(messageType, path string) *controlMetricAccumulator {
	byPath, ok := m.byTypePath[messageType]
	if !ok {
		byPath = make(map[string]*controlMetricAccumulator)
		m.byTypePath[messageType] = byPath
	}

	accumulator, ok := byPath[path]
	if ok {
		return accumulator
	}
	accumulator = &controlMetricAccumulator{}
	byPath[path] = accumulator
	return accumulator
}

func snapshotAccumulator(accumulator controlMetricAccumulator) ControlMetricStats {
	avgLatencyMs := int64(0)
	if accumulator.requests > 0 {
		avgLatencyMs = (accumulator.totalLatency / time.Duration(accumulator.requests)).Milliseconds()
	}

	return ControlMetricStats{
		Requests:      accumulator.requests,
		Successes:     accumulator.successes,
		Failures:      accumulator.failures,
		Timeouts:      accumulator.timeouts,
		LastLatencyMs: durationMillis(accumulator.lastLatency),
		AvgLatencyMs:  avgLatencyMs,
		MaxLatencyMs:  durationMillis(accumulator.maxLatency),
	}
}

func normalizeControlPath(path string) string {
	switch strings.ToLower(strings.TrimSpace(path)) {
	case ControlPathHTTP:
		return ControlPathHTTP
	case ControlPathP2P:
		return ControlPathP2P
	default:
		return ControlPathUnknown
	}
}

func normalizeControlMessageType(messageType protocol.MessageType) string {
	value := strings.TrimSpace(string(messageType))
	if value == "" {
		return "UNKNOWN"
	}
	return value
}

func durationMillis(value time.Duration) int64 {
	if value <= 0 {
		return 0
	}
	return value.Milliseconds()
}
