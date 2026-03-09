package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Fharena/VibeDeck/internal/protocol"
	"github.com/Fharena/VibeDeck/internal/runtime"
)

func newTestHTTPServer() (*HTTPServer, *runtime.AckTracker, *ControlMetrics) {
	adapter := NewMockAdapter()
	threadStore := NewThreadStore()
	orch := NewOrchestrator(adapter, DefaultRunProfiles(), threadStore)
	stateManager := runtime.NewStateManager(runtime.DefaultManagerConfig())
	ackTracker := runtime.NewAckTracker(2 * time.Second)
	controlMetrics := NewControlMetrics()
	p2pManager := NewP2PSessionManager(stateManager, ackTracker, orch, "http://127.0.0.1:8081")
	p2pManager.SetControlMetrics(controlMetrics)

	server := NewHTTPServer(
		adapter,
		orch,
		stateManager,
		ackTracker,
		controlMetrics,
		p2pManager,
		HTTPServerConfig{},
	)

	return server, ackTracker, controlMetrics
}

func TestHTTPServerHandleEnvelope(t *testing.T) {
	server, _, _ := newTestHTTPServer()

	body := []byte(`{"sid":"sid-1","rid":"rid-1","seq":1,"ts":1700000000000,"type":"PROMPT_SUBMIT","payload":{"prompt":"Fix auth","contextOptions":{}}}`)

	req := httptest.NewRequest(http.MethodPost, "/v1/agent/envelope", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHTTPServerRuntimeState(t *testing.T) {
	server, _, _ := newTestHTTPServer()

	req := httptest.NewRequest(http.MethodPost, "/v1/agent/runtime/state", bytes.NewBufferString(`{"action":"begin_signaling"}`))
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHTTPServerRuntimeMetrics(t *testing.T) {
	server, tracker, controlMetrics := newTestHTTPServer()

	env, err := protocol.NewEnvelope("sid-1", "rid-metrics", 1, protocol.TypePromptAck, map[string]any{
		"jobId": "job_1",
	})
	if err != nil {
		t.Fatalf("build metrics envelope: %v", err)
	}
	tracker.RegisterEnvelope(env, runtime.AckTransportHTTP, false)
	tracker.Ack(env.RID)

	controlMetrics.Observe(protocol.TypePromptSubmit, ControlPathHTTP, 25*time.Millisecond, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/agent/runtime/metrics", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body struct {
		State     runtime.ConnectionState `json:"state"`
		P2PActive bool                    `json:"p2pActive"`
		Ack       runtime.AckMetrics      `json:"ack"`
		Control   ControlMetricsSnapshot  `json:"control"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode runtime metrics: %v", err)
	}

	if body.Ack.AckedCount != 1 {
		t.Fatalf("expected acked count 1, got %d", body.Ack.AckedCount)
	}
	if body.Ack.PendingCount != 0 {
		t.Fatalf("expected pending count 0, got %d", body.Ack.PendingCount)
	}
	if body.Ack.PendingByTransport[string(runtime.AckTransportHTTP)] != 0 {
		t.Fatalf("expected http pending 0")
	}
	if body.Control.Totals.Requests != 1 {
		t.Fatalf("expected control request count 1, got %d", body.Control.Totals.Requests)
	}
	if body.Control.ByPath[ControlPathHTTP].Successes != 1 {
		t.Fatalf("expected http control success count 1, got %+v", body.Control.ByPath[ControlPathHTTP])
	}
	if body.Control.ByType[string(protocol.TypePromptSubmit)].Requests != 1 {
		t.Fatalf("expected prompt_submit control count 1, got %+v", body.Control.ByType)
	}
}

func TestHTTPServerRuntimeAdapter(t *testing.T) {
	server, _, _ := newTestHTTPServer()

	req := httptest.NewRequest(http.MethodGet, "/v1/agent/runtime/adapter", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body AdapterRuntimeInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode runtime adapter: %v", err)
	}
	if body.Name != "mock-cursor" {
		t.Fatalf("expected adapter name mock-cursor, got %s", body.Name)
	}
	if !body.Ready {
		t.Fatalf("expected adapter ready")
	}
	if !body.Capabilities.SupportsStructuredPatch {
		t.Fatalf("expected structured patch capability")
	}
}

func TestHTTPServerRunProfilesAndThreadsEndpoints(t *testing.T) {
	server, _, _ := newTestHTTPServer()

	profileReq := httptest.NewRequest(http.MethodGet, "/v1/agent/run-profiles", nil)
	profileRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(profileRec, profileReq)

	if profileRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", profileRec.Code)
	}

	var profileBody struct {
		Profiles []RunProfileDescriptor `json:"profiles"`
	}
	if err := json.Unmarshal(profileRec.Body.Bytes(), &profileBody); err != nil {
		t.Fatalf("decode run profiles: %v", err)
	}
	if len(profileBody.Profiles) == 0 {
		t.Fatalf("expected non-empty run profile list")
	}

	submitReq := httptest.NewRequest(
		http.MethodPost,
		"/v1/agent/envelope",
		bytes.NewBufferString(`{"sid":"sid-threads","rid":"rid-submit","seq":1,"ts":1700000000000,"type":"PROMPT_SUBMIT","payload":{"prompt":"Create hello world file","contextOptions":{}}}`),
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

	var promptAck protocol.PromptAckPayload
	if err := submitBody.Responses[1].DecodePayload(&promptAck); err != nil {
		t.Fatalf("decode prompt ack: %v", err)
	}

	threadsReq := httptest.NewRequest(http.MethodGet, "/v1/agent/threads", nil)
	threadsRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(threadsRec, threadsReq)
	if threadsRec.Code != http.StatusOK {
		t.Fatalf("threads response should be 200")
	}

	var threadsBody struct {
		Threads []ThreadSummary `json:"threads"`
	}
	if err := json.Unmarshal(threadsRec.Body.Bytes(), &threadsBody); err != nil {
		t.Fatalf("decode threads: %v", err)
	}
	if len(threadsBody.Threads) != 1 {
		t.Fatalf("expected 1 thread, got %+v", threadsBody.Threads)
	}
	if threadsBody.Threads[0].ID != promptAck.ThreadID {
		t.Fatalf("expected thread id %s, got %+v", promptAck.ThreadID, threadsBody.Threads[0])
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/v1/agent/threads/"+promptAck.ThreadID, nil)
	detailRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("thread detail response should be 200")
	}

	var detail ThreadDetail
	if err := json.Unmarshal(detailRec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode thread detail: %v", err)
	}
	if len(detail.Events) != 3 {
		t.Fatalf("expected 3 events, got %+v", detail.Events)
	}
	if detail.Thread.CurrentJobID == "" {
		t.Fatalf("expected current job id to be set")
	}
	if detail.Events[2].Kind != "patch_ready" {
		t.Fatalf("expected third event to be patch_ready, got %+v", detail.Events[2])
	}
	filesRaw, ok := detail.Events[2].Data["files"].([]any)
	if !ok || len(filesRaw) == 0 {
		t.Fatalf("expected patch_ready event to include files, got %+v", detail.Events[2].Data)
	}
}

func TestHTTPServerInboundCmdAckClearsPending(t *testing.T) {
	server, tracker, _ := newTestHTTPServer()

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

func TestHTTPServerPrometheusMetricsEndpoint(t *testing.T) {
	server, tracker, controlMetrics := newTestHTTPServer()
	server.stateManager.BeginSignaling()

	env, err := protocol.NewEnvelope("sid-1", "rid-prometheus", 1, protocol.TypePromptAck, map[string]any{
		"jobId": "job_1",
	})
	if err != nil {
		t.Fatalf("build prometheus envelope: %v", err)
	}
	tracker.RegisterEnvelope(env, runtime.AckTransportHTTP, false)
	tracker.Ack(env.RID)

	controlMetrics.Observe(protocol.TypePromptSubmit, ControlPathHTTP, 120*time.Millisecond, nil)
	controlMetrics.Observe(protocol.TypePatchApply, ControlPathP2P, 30*time.Millisecond, context.DeadlineExceeded)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("expected prometheus content type, got %q", rec.Header().Get("Content-Type"))
	}

	body := rec.Body.String()
	for _, pattern := range []string{
		`vibedeck_runtime_state{state="SIGNALING"} 1`,
		`vibedeck_ack_acked_total 1`,
		`vibedeck_control_requests_total{type="PROMPT_SUBMIT",path="http",result="success"} 1`,
		`vibedeck_control_requests_total{type="PATCH_APPLY",path="p2p",result="timeout"} 1`,
	} {
		if !strings.Contains(body, pattern) {
			t.Fatalf("expected prometheus output to contain %q\n%s", pattern, body)
		}
	}
}

func TestHTTPServerP2PStatusEndpoint(t *testing.T) {
	server, _, _ := newTestHTTPServer()

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

func TestHTTPServerBootstrapEndpoint(t *testing.T) {
	server, _, _ := newTestHTTPServer()
	server.orchestrator.ThreadStore().EnsureThread("thread-bootstrap-1", "sid-bootstrap-1", "bootstrap thread")

	req := httptest.NewRequest(http.MethodGet, "/v1/agent/bootstrap", nil)
	req.Host = "192.168.0.24:8080"
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body BootstrapResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode bootstrap response: %v", err)
	}
	if body.AgentBaseURL != "http://192.168.0.24:8080" {
		t.Fatalf("expected agent base url to follow request host, got %q", body.AgentBaseURL)
	}
	if body.SignalingBaseURL != "http://192.168.0.24:8081" {
		t.Fatalf("expected signaling base url to follow request host, got %q", body.SignalingBaseURL)
	}
	if body.WorkspaceRoot != "" {
		t.Fatalf("expected empty workspace root for mock adapter, got %q", body.WorkspaceRoot)
	}
	if body.Adapter.Name != "mock-cursor" || body.Adapter.Provider != "cursor" || !body.Adapter.Ready {
		t.Fatalf("unexpected bootstrap adapter info: %+v", body.Adapter)
	}
	if body.CurrentThreadID != "thread-bootstrap-1" {
		t.Fatalf("expected current thread id thread-bootstrap-1, got %q", body.CurrentThreadID)
	}
	if len(body.RecentThreads) != 1 {
		t.Fatalf("expected 1 recent thread, got %+v", body.RecentThreads)
	}
	if !body.RecentThreads[0].Current {
		t.Fatalf("expected first recent thread to be current")
	}
}
