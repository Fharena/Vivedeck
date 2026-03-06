package agent

import (
    "bytes"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/Fharena/Vivedeck/internal/runtime"
)

func TestHTTPServerHandleEnvelope(t *testing.T) {
    orch := NewOrchestrator(NewMockAdapter(), DefaultRunProfiles())
    server := NewHTTPServer(
        orch,
        runtime.NewStateManager(runtime.DefaultManagerConfig()),
        runtime.NewAckTracker(2*time.Second),
    )

    body := []byte(`{"sid":"sid-1","rid":"rid-1","seq":1,"ts":1700000000000,"type":"PROMPT_SUBMIT","payload":{"prompt":"Fix auth","contextOptions":{}}}`)

    req := httptest.NewRequest(http.MethodPost, "/v1/agent/envelope", bytes.NewReader(body))
    rec := httptest.NewRecorder()

    server.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d", rec.Code)
    }
}

func TestHTTPServerRuntimeState(t *testing.T) {
    orch := NewOrchestrator(NewMockAdapter(), DefaultRunProfiles())
    server := NewHTTPServer(
        orch,
        runtime.NewStateManager(runtime.DefaultManagerConfig()),
        runtime.NewAckTracker(2*time.Second),
    )

    req := httptest.NewRequest(http.MethodPost, "/v1/agent/runtime/state", bytes.NewBufferString(`{"action":"begin_signaling"}`))
    rec := httptest.NewRecorder()

    server.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d", rec.Code)
    }
}
