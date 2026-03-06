package agent

import (
    "bytes"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/Fharena/Vivedeck/internal/protocol"
)

func TestHTTPServerHandleEnvelope(t *testing.T) {
    orch := NewOrchestrator(NewMockAdapter(), DefaultRunProfiles())
    server := NewHTTPServer(orch)

    env, _ := protocol.NewEnvelope("sid-1", "rid-1", 1, protocol.TypePromptSubmit, protocol.PromptSubmitPayload{
        Prompt: "Fix auth",
    })

    body := []byte(`{"sid":"` + env.SID + `","rid":"` + env.RID + `","seq":1,"ts":` + "1700000000000" + `,"type":"PROMPT_SUBMIT","payload":{"prompt":"Fix auth","contextOptions":{}}}`)

    req := httptest.NewRequest(http.MethodPost, "/v1/agent/envelope", bytes.NewReader(body))
    rec := httptest.NewRecorder()

    server.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d", rec.Code)
    }
}
