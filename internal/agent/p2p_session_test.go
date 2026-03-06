package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Fharena/Vivedeck/internal/protocol"
	"github.com/Fharena/Vivedeck/internal/runtime"
	"github.com/Fharena/Vivedeck/internal/signaling"
	"github.com/Fharena/Vivedeck/internal/webrtc"
	"github.com/gorilla/websocket"
)

func newSignalingTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	store := signaling.NewStore(3 * time.Minute)
	srv := signaling.NewServer(store)
	return httptest.NewServer(srv.Handler())
}

func newP2PTestManager(signalingBaseURL string) (*P2PSessionManager, *runtime.AckTracker) {
	stateManager := runtime.NewStateManager(runtime.DefaultManagerConfig())
	ackTracker := runtime.NewAckTracker(2 * time.Second)
	orchestrator := NewOrchestrator(NewMockAdapter(), DefaultRunProfiles())
	manager := NewP2PSessionManager(stateManager, ackTracker, orchestrator, signalingBaseURL)
	return manager, ackTracker
}

func waitForManagerState(t *testing.T, manager *P2PSessionManager, target runtime.ConnectionState, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		current := manager.Status()
		if current.State == target {
			return
		}
		time.Sleep(120 * time.Millisecond)
	}

	finalStatus := manager.Status()
	t.Fatalf("expected state %s, got %s (lastError=%s)", target, finalStatus.State, finalStatus.LastError)
}

func TestP2PSessionManagerStartAndStop(t *testing.T) {
	ts := newSignalingTestServer(t)
	defer ts.Close()

	manager, _ := newP2PTestManager(ts.URL)

	status, err := manager.Start(context.Background(), StartP2PRequest{})
	if err != nil {
		t.Fatalf("start p2p session: %v", err)
	}

	if !status.Active {
		t.Fatalf("status should be active")
	}
	if status.SessionID == "" || status.PairingCode == "" {
		t.Fatalf("sessionId/pairingCode should be set")
	}
	if status.State != runtime.StateSignaling {
		t.Fatalf("state should be signaling after start, got %s", status.State)
	}

	stopStatus, err := manager.Stop()
	if err != nil {
		t.Fatalf("stop p2p session: %v", err)
	}

	if stopStatus.Active {
		t.Fatalf("status should be inactive after stop")
	}
	if stopStatus.State != runtime.StateClosed {
		t.Fatalf("state should be closed after stop, got %s", stopStatus.State)
	}
}

func TestP2PSessionManagerNegotiatesWithMobile(t *testing.T) {
	ts := newSignalingTestServer(t)
	defer ts.Close()

	manager, _ := newP2PTestManager(ts.URL)

	status, err := manager.Start(context.Background(), StartP2PRequest{})
	if err != nil {
		t.Fatalf("start p2p session: %v", err)
	}
	defer func() { _, _ = manager.Stop() }()

	mobileSessionID, mobileKey, err := claimPairing(ts.URL, status.PairingCode)
	if err != nil {
		t.Fatalf("claim pairing: %v", err)
	}
	if mobileSessionID != status.SessionID {
		t.Fatalf("session id mismatch: claimed=%s status=%s", mobileSessionID, status.SessionID)
	}

	mobileConn, err := dialMobileWS(ts.URL, status.SessionID, mobileKey)
	if err != nil {
		t.Fatalf("dial mobile ws: %v", err)
	}
	defer mobileConn.Close()

	mobilePeer, err := webrtc.NewPeer(webrtc.DefaultConfig(webrtc.SideMobile))
	if err != nil {
		t.Fatalf("new mobile peer: %v", err)
	}
	defer func() { _ = mobilePeer.Close() }()

	mobileBridge, err := webrtc.NewSignalBridge(status.SessionID, webrtc.SideMobile, mobilePeer)
	if err != nil {
		t.Fatalf("new mobile bridge: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go mobileBridge.Run(ctx)
	go mobileReadLoop(ctx, t, mobileConn, mobileBridge)
	go mobileWriteLoop(ctx, t, mobileConn, mobileBridge)

	waitForManagerState(t, manager, runtime.StateP2PConnected, 12*time.Second)
}

func TestP2PSessionManagerRoutesControlEnvelopeOverDataChannel(t *testing.T) {
	ts := newSignalingTestServer(t)
	defer ts.Close()

	manager, ackTracker := newP2PTestManager(ts.URL)

	status, err := manager.Start(context.Background(), StartP2PRequest{})
	if err != nil {
		t.Fatalf("start p2p session: %v", err)
	}
	defer func() { _, _ = manager.Stop() }()

	mobileSessionID, mobileKey, err := claimPairing(ts.URL, status.PairingCode)
	if err != nil {
		t.Fatalf("claim pairing: %v", err)
	}
	if mobileSessionID != status.SessionID {
		t.Fatalf("session id mismatch: claimed=%s status=%s", mobileSessionID, status.SessionID)
	}

	mobileConn, err := dialMobileWS(ts.URL, status.SessionID, mobileKey)
	if err != nil {
		t.Fatalf("dial mobile ws: %v", err)
	}
	defer mobileConn.Close()

	mobilePeer, err := webrtc.NewPeer(webrtc.DefaultConfig(webrtc.SideMobile))
	if err != nil {
		t.Fatalf("new mobile peer: %v", err)
	}
	defer func() { _ = mobilePeer.Close() }()

	mobileBridge, err := webrtc.NewSignalBridge(status.SessionID, webrtc.SideMobile, mobilePeer)
	if err != nil {
		t.Fatalf("new mobile bridge: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go mobileBridge.Run(ctx)
	go mobileReadLoop(ctx, t, mobileConn, mobileBridge)
	go mobileWriteLoop(ctx, t, mobileConn, mobileBridge)

	waitForManagerState(t, manager, runtime.StateP2PConnected, 12*time.Second)
	if err := mobilePeer.WaitDataChannelOpen(5 * time.Second); err != nil {
		t.Fatalf("mobile data channel open: %v", err)
	}

	submitEnv, err := protocol.NewEnvelope(status.SessionID, "rid-submit", 1, protocol.TypePromptSubmit, protocol.PromptSubmitPayload{
		Prompt:         "Fix auth middleware",
		ContextOptions: protocol.ContextOptions{},
	})
	if err != nil {
		t.Fatalf("build submit envelope: %v", err)
	}

	submitBytes, err := json.Marshal(submitEnv)
	if err != nil {
		t.Fatalf("marshal submit envelope: %v", err)
	}

	if err := mobilePeer.Send(submitBytes); err != nil {
		t.Fatalf("send submit over data channel: %v", err)
	}

	typeSet := map[protocol.MessageType]bool{}
	received := make([]protocol.Envelope, 0, 3)
	deadline := time.After(8 * time.Second)
	for len(typeSet) < 3 {
		select {
		case raw := <-mobilePeer.Messages():
			var response protocol.Envelope
			if err := json.Unmarshal(raw, &response); err != nil {
				t.Fatalf("decode response envelope: %v", err)
			}
			received = append(received, response)
			typeSet[response.Type] = true

		case <-deadline:
			t.Fatalf("timeout waiting p2p control responses")
		}
	}

	if !typeSet[protocol.TypeCmdAck] {
		t.Fatalf("expected CMD_ACK response")
	}
	if !typeSet[protocol.TypePromptAck] {
		t.Fatalf("expected PROMPT_ACK response")
	}
	if !typeSet[protocol.TypePatchReady] {
		t.Fatalf("expected PATCH_READY response")
	}

	targetRID := ""
	for _, response := range received {
		if response.Type != protocol.TypeCmdAck {
			targetRID = response.RID
			break
		}
	}
	if targetRID == "" {
		t.Fatalf("target rid for CMD_ACK was not found")
	}

	snapshot := ackTracker.Snapshot()
	foundPending := false
	for _, pending := range snapshot {
		if pending.RID == targetRID {
			foundPending = true
			break
		}
	}
	if !foundPending {
		t.Fatalf("expected pending ack for rid %s", targetRID)
	}

	ackEnv, err := protocol.NewEnvelope(status.SessionID, "rid-mobile-ack", 2, protocol.TypeCmdAck, protocol.CmdAckPayload{
		RequestRID: targetRID,
		Accepted:   true,
		Message:    "received",
	})
	if err != nil {
		t.Fatalf("build mobile cmd ack envelope: %v", err)
	}
	ackBytes, err := json.Marshal(ackEnv)
	if err != nil {
		t.Fatalf("marshal mobile cmd ack envelope: %v", err)
	}
	if err := mobilePeer.Send(ackBytes); err != nil {
		t.Fatalf("send mobile cmd ack: %v", err)
	}

	ackDeadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(ackDeadline) {
		stillPending := false
		for _, pending := range ackTracker.Snapshot() {
			if pending.RID == targetRID {
				stillPending = true
				break
			}
		}
		if !stillPending {
			return
		}
		time.Sleep(80 * time.Millisecond)
	}

	t.Fatalf("acked rid should be removed from pending: %s", targetRID)
}

func claimPairing(baseURL, code string) (sessionID string, mobileKey string, err error) {
	url := strings.TrimRight(baseURL, "/") + "/v1/pairings/" + code + "/claim"
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return "", "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var out struct {
		SessionID       string `json:"sessionId"`
		MobileDeviceKey string `json:"mobileDeviceKey"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", err
	}

	return out.SessionID, out.MobileDeviceKey, nil
}

func dialMobileWS(baseURL, sessionID, mobileKey string) (*websocket.Conn, error) {
	wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/v1/sessions/" + sessionID + "/ws?deviceKey=" + mobileKey + "&role=mobile"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func mobileReadLoop(ctx context.Context, t *testing.T, conn *websocket.Conn, bridge *webrtc.SignalBridge) {
	t.Helper()

	for {
		if ctx.Err() != nil {
			return
		}

		var env protocol.Envelope
		if err := conn.ReadJSON(&env); err != nil {
			if ctx.Err() != nil {
				return
			}
			t.Logf("mobile read loop ended: %v", err)
			return
		}

		if err := env.Validate(); err != nil {
			continue
		}

		if err := bridge.InboundEnvelope(env); err != nil {
			t.Logf("mobile bridge inbound warning: %v", err)
		}
	}
}

func mobileWriteLoop(ctx context.Context, t *testing.T, conn *websocket.Conn, bridge *webrtc.SignalBridge) {
	t.Helper()

	for {
		select {
		case env := <-bridge.Outbound():
			if err := conn.WriteJSON(env); err != nil {
				if ctx.Err() != nil {
					return
				}
				t.Logf("mobile write loop ended: %v", err)
				return
			}

		case <-ctx.Done():
			return
		}
	}
}
