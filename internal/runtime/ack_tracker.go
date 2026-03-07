package runtime

import (
	"sync"
	"time"

	"github.com/Fharena/Vivedeck/internal/protocol"
)

type AckTransport string

const (
	AckTransportUnknown AckTransport = "unknown"
	AckTransportHTTP    AckTransport = "http"
	AckTransportP2P     AckTransport = "p2p"
)

type PendingAck struct {
	SID          string       `json:"sid"`
	RID          string       `json:"rid"`
	MessageType  string       `json:"messageType"`
	Transport    AckTransport `json:"transport"`
	RegisteredAt time.Time    `json:"registeredAt"`
	LastSentAt   time.Time    `json:"lastSentAt"`
	ExpiresAt    time.Time    `json:"expiresAt"`
	RetryEnabled bool         `json:"retryEnabled"`
	RetryCount   int          `json:"retryCount"`
	MaxRetries   int          `json:"maxRetries"`
}

type AckTrackerConfig struct {
	Timeout           time.Duration
	MaxRetries        int
	BackoffMultiplier int
}

type AckRetry struct {
	Pending  PendingAck
	Envelope protocol.Envelope
}

type AckRetryBatch struct {
	Retries   []AckRetry
	Exhausted []PendingAck
}

type trackedAck struct {
	pending  PendingAck
	envelope protocol.Envelope
}

type AckTracker struct {
	timeout           time.Duration
	maxRetries        int
	backoffMultiplier int
	now               func() time.Time

	mu      sync.Mutex
	pending map[string]trackedAck
}

func DefaultAckTrackerConfig() AckTrackerConfig {
	return AckTrackerConfig{
		Timeout:           2 * time.Second,
		MaxRetries:        2,
		BackoffMultiplier: 2,
	}
}

func NewAckTracker(timeout time.Duration) *AckTracker {
	cfg := DefaultAckTrackerConfig()
	if timeout > 0 {
		cfg.Timeout = timeout
	}
	return NewAckTrackerWithConfig(cfg)
}

func NewAckTrackerWithConfig(cfg AckTrackerConfig) *AckTracker {
	defaults := DefaultAckTrackerConfig()
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaults.Timeout
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = defaults.MaxRetries
	}
	if cfg.BackoffMultiplier < 1 {
		cfg.BackoffMultiplier = defaults.BackoffMultiplier
	}

	return &AckTracker{
		timeout:           cfg.Timeout,
		maxRetries:        cfg.MaxRetries,
		backoffMultiplier: cfg.BackoffMultiplier,
		now:               time.Now,
		pending:           make(map[string]trackedAck),
	}
}

func (t *AckTracker) Register(sid, rid, messageType string) {
	t.registerLocked(trackedAck{
		pending: PendingAck{
			SID:          sid,
			RID:          rid,
			MessageType:  messageType,
			Transport:    AckTransportUnknown,
			RetryEnabled: false,
			MaxRetries:   0,
		},
	})
}

func (t *AckTracker) RegisterEnvelope(env protocol.Envelope, transport AckTransport, retryEnabled bool) {
	if transport == "" {
		transport = AckTransportUnknown
	}

	t.registerLocked(trackedAck{
		envelope: env,
		pending: PendingAck{
			SID:          env.SID,
			RID:          env.RID,
			MessageType:  string(env.Type),
			Transport:    transport,
			RetryEnabled: retryEnabled,
			MaxRetries:   maxRetriesForRetry(retryEnabled, t.maxRetries),
		},
	})
}

func (t *AckTracker) Ack(rid string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, ok := t.pending[rid]; !ok {
		return false
	}

	delete(t.pending, rid)
	return true
}

func (t *AckTracker) PendingCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	return len(t.pending)
}

func (t *AckTracker) Snapshot() []PendingAck {
	t.mu.Lock()
	defer t.mu.Unlock()

	out := make([]PendingAck, 0, len(t.pending))
	for _, tracked := range t.pending {
		out = append(out, tracked.pending)
	}

	return out
}

func (t *AckTracker) DueRetries() AckRetryBatch {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now().UTC()
	batch := AckRetryBatch{
		Retries:   make([]AckRetry, 0),
		Exhausted: make([]PendingAck, 0),
	}

	for rid, tracked := range t.pending {
		pending := tracked.pending
		if !pending.RetryEnabled || now.Before(pending.ExpiresAt) {
			continue
		}

		if pending.RetryCount >= pending.MaxRetries {
			batch.Exhausted = append(batch.Exhausted, pending)
			delete(t.pending, rid)
			continue
		}

		pending.RetryCount++
		pending.LastSentAt = now
		pending.ExpiresAt = now.Add(t.nextDelay(pending.RetryCount))
		tracked.pending = pending
		t.pending[rid] = tracked

		batch.Retries = append(batch.Retries, AckRetry{
			Pending:  pending,
			Envelope: tracked.envelope,
		})
	}

	return batch
}

func (t *AckTracker) Expired() []PendingAck {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now().UTC()
	expired := make([]PendingAck, 0)

	for rid, tracked := range t.pending {
		pending := tracked.pending
		if now.Before(pending.ExpiresAt) {
			continue
		}
		if pending.RetryEnabled && pending.RetryCount < pending.MaxRetries {
			continue
		}

		expired = append(expired, pending)
		delete(t.pending, rid)
	}

	return expired
}

func (t *AckTracker) ForgetBySessionTransport(sid string, transport AckTransport) {
	if sid == "" {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	for rid, tracked := range t.pending {
		if tracked.pending.SID == sid && tracked.pending.Transport == transport {
			delete(t.pending, rid)
		}
	}
}

func (t *AckTracker) registerLocked(entry trackedAck) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now().UTC()
	entry.pending.RegisteredAt = now
	entry.pending.LastSentAt = now
	entry.pending.ExpiresAt = now.Add(t.timeout)
	entry.pending.MaxRetries = maxRetriesForRetry(entry.pending.RetryEnabled, t.maxRetries)
	entry.pending.Transport = normalizeTransport(entry.pending.Transport)

	t.pending[entry.pending.RID] = entry
}

func (t *AckTracker) nextDelay(retryCount int) time.Duration {
	delay := t.timeout
	for i := 0; i < retryCount; i++ {
		if t.backoffMultiplier <= 1 {
			continue
		}
		delay *= time.Duration(t.backoffMultiplier)
	}
	return delay
}

func maxRetriesForRetry(retryEnabled bool, maxRetries int) int {
	if !retryEnabled || maxRetries < 0 {
		return 0
	}
	return maxRetries
}

func normalizeTransport(transport AckTransport) AckTransport {
	if transport == "" {
		return AckTransportUnknown
	}
	return transport
}
