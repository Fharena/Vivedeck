package agent

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Fharena/Vivedeck/internal/protocol"
	"github.com/Fharena/Vivedeck/internal/runtime"
)

func newTestHTTPServer() (*HTTPServer, *runtime.AckTracker) {
	orch := NewOrchestrator(NewMockAdapter(), DefaultRunProfiles())
	stateManager := runtime.NewStateManager(runtime.DefaultManagerConfig())
	ackTracker := runtime.NewAckTracker(2 * time.Second)
	p2pManager := NewP2PSessionManager(stateManager, "http://127.0.0.1:8081")

	server := NewHTTPServer(
		orch,
		stateManager,
		ackTracker,
		p2pManager,
	)

	return server, ackTracker
}

func TestHTTPServerHandleEnvelope(t *testing.T) {
	server, _ := newTestHTTPServer()

	body := []byte(`{"sid":"sid-1","rid":"rid-1","seq":1,"ts":1700000000000,"type":"PROMPT_SUBMIT","payload":{"prompt":"Fix auth","contextOptions":{}}}`)

	req := httptest.NewRequest(http.MethodPost, "/v1/agent/envelope", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHTTPServerRuntimeState(t *testing.T) {
	server, _ := newTestHTTPServer()

	req := httptest.NewRequest(http.MethodPost, "/v1/agent/runtime/state", bytes.NewBufferString(`{"action":"begin_signaling"}`))
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHTTPServerInboundCmdAckClearsPending(t *testing.T) {
	server, tracker := newTestHTTPServer()

	submitReq := httptest.NewRequest(
		http.MethodPost,
		"/v1/agent/envelope",
		bytes.NewBufferString(`{"sid":"sid-1","rid":"rid-submit","seq":1,"ts":1700000000000,"type":"PROMPT_SUBMIT","payload":{"prompt":"Fix auth","contextOptions":{}}}`),
	)
	submitRec := httptest.NewRecorder()

	server.Handler().ServeHTTP(submitRec, submitReq)
	if submitRec.Code != http.StatusOK {
		t.Fatalf("submit response should be 200")
	}

	var submitBody struct {
		Responses []protocol.Envelope `json:"responses"`
	}
	if err := json.Unmarshal(submitRec.Body.Bytes(), &submitBody); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}

	if len(submitBody.Responses) < 2 {
		t.Fatalf("expected at least 2 responses")
	}

	targetRID := ""
	for _, response := range submitBody.Responses {
		if response.Type != protocol.TypeCmdAck {
			targetRID = response.RID
			break
		}
	}

	if targetRID == "" {
		t.Fatalf("tracked response rid not found")
	}

	if tracker.PendingCount() == 0 {
		t.Fatalf("pending acks should exist after submit")
	}

	ackEnvelope, err := protocol.NewEnvelope("sid-1", "rid-ack", 2, protocol.TypeCmdAck, protocol.CmdAckPayload{
		RequestRID: targetRID,
		Accepted:   true,
		Message:    "received",
	})
	if err != nil {
		t.Fatalf("build ack envelope: %v", err)
	}

	ackBytes, _ := json.Marshal(ackEnvelope)
	ackReq := httptest.NewRequest(http.MethodPost, "/v1/agent/envelope", bytes.NewReader(ackBytes))
	ackRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(ackRec, ackReq)

	if ackRec.Code != http.StatusOK {
		t.Fatalf("ack response should be 200, got %d", ackRec.Code)
	}

	if tracker.PendingCount() == 0 {
		return
	}

	snapshot := tracker.Snapshot()
	for _, pending := range snapshot {
		if pending.RID == targetRID {
			t.Fatalf("acked rid should be removed from pending")
		}
	}
}

func TestHTTPServerP2PStatusEndpoint(t *testing.T) {
	server, _ := newTestHTTPServer()

	req := httptest.NewRequest(http.MethodGet, "/v1/agent/p2p/status", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var status P2PSessionStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode p2p status: %v", err)
	}

	if status.Active {
		t.Fatalf("expected p2p status inactive")
	}
}
