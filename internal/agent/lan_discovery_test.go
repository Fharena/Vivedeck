package agent

import (
	"encoding/json"
	"net"
	"testing"
	"time"
)

func TestResolveDiscoveryBaseURLsRewriteLoopbackHosts(t *testing.T) {
	agentBaseURL := resolveDiscoveryAgentBaseURL("192.168.0.24:8080", "http://127.0.0.1:8080")
	if agentBaseURL == "http://127.0.0.1:8080" || agentBaseURL == "http://localhost:8080" {
		t.Fatalf("expected discovery agent base url to avoid loopback, got %q", agentBaseURL)
	}

	signalingBaseURL := resolveDiscoverySignalingBaseURL(
		"http://127.0.0.1:8081",
		"",
		hostFromBaseURL(agentBaseURL),
	)
	if signalingBaseURL == "http://127.0.0.1:8081" || signalingBaseURL == "http://localhost:8081" {
		t.Fatalf("expected discovery signaling base url to avoid loopback, got %q", signalingBaseURL)
	}
}

func TestLANDiscoveryResponderReturnsBootstrapPayload(t *testing.T) {
	server, _, _ := newTestHTTPServer()
	server.config = normalizeHTTPServerConfig(HTTPServerConfig{
		PublicAgentBaseURL:     "http://192.168.0.24:8080",
		PublicSignalingBaseURL: "http://192.168.0.24:8081",
	})
	server.orchestrator.ThreadStore().EnsureThread("thread-discovery-1", "sid-discovery-1", "discovery thread")

	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	defer conn.Close()

	responder := &lanDiscoveryResponder{
		conn:            conn,
		server:          server,
		agentListenAddr: ":8080",
	}
	go responder.serve()

	client, err := net.DialUDP("udp4", nil, conn.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatalf("dial udp: %v", err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(2 * time.Second))

	probe, err := json.Marshal(lanDiscoveryProbe{
		Type:    lanDiscoveryProbeType,
		Version: lanDiscoveryVersion,
	})
	if err != nil {
		t.Fatalf("marshal probe: %v", err)
	}
	if _, err := client.Write(probe); err != nil {
		t.Fatalf("write probe: %v", err)
	}

	buffer := make([]byte, 4096)
	n, err := client.Read(buffer)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	var response LANDiscoveryResponse
	if err := json.Unmarshal(buffer[:n], &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Type != lanDiscoveryResultType {
		t.Fatalf("expected response type %q, got %q", lanDiscoveryResultType, response.Type)
	}
	if response.AgentBaseURL != "http://192.168.0.24:8080" {
		t.Fatalf("expected agent base url override, got %q", response.AgentBaseURL)
	}
	if response.SignalingBaseURL != "http://192.168.0.24:8081" {
		t.Fatalf("expected signaling base url override, got %q", response.SignalingBaseURL)
	}
	if response.CurrentThreadID != "thread-discovery-1" {
		t.Fatalf("expected current thread id thread-discovery-1, got %q", response.CurrentThreadID)
	}
	if len(response.RecentThreads) != 1 {
		t.Fatalf("expected one recent thread, got %+v", response.RecentThreads)
	}
}
