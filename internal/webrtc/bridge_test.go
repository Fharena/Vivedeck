package webrtc

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/Fharena/Vivedeck/internal/protocol"
)

func TestSignalBridgeEndToEndNegotiation(t *testing.T) {
	pcPeer, err := NewPeer(DefaultConfig(SidePC))
	if err != nil {
		t.Fatalf("new pc peer: %v", err)
	}
	defer func() { _ = pcPeer.Close() }()

	mobilePeer, err := NewPeer(DefaultConfig(SideMobile))
	if err != nil {
		t.Fatalf("new mobile peer: %v", err)
	}
	defer func() { _ = mobilePeer.Close() }()

	pcBridge, err := NewSignalBridge("sid-1", SidePC, pcPeer)
	if err != nil {
		t.Fatalf("new pc bridge: %v", err)
	}

	mobileBridge, err := NewSignalBridge("sid-1", SideMobile, mobilePeer)
	if err != nil {
		t.Fatalf("new mobile bridge: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go pcBridge.Run(ctx)
	go mobileBridge.Run(ctx)
	go relayOutbound(ctx, pcBridge.Outbound(), mobileBridge)
	go relayOutbound(ctx, mobileBridge.Outbound(), pcBridge)

	rdyPC, _ := protocol.NewEnvelope("sid-1", "ready-pc", 1, protocol.TypeSignalReady, protocol.SignalReadyPayload{
		Role:          "pc",
		PeerConnected: true,
		Timestamp:     time.Now().UTC().UnixMilli(),
	})
	rdyMobile, _ := protocol.NewEnvelope("sid-1", "ready-mobile", 2, protocol.TypeSignalReady, protocol.SignalReadyPayload{
		Role:          "mobile",
		PeerConnected: true,
		Timestamp:     time.Now().UTC().UnixMilli(),
	})

	if err := pcBridge.InboundEnvelope(rdyPC); err != nil {
		t.Fatalf("pc ready envelope: %v", err)
	}
	if err := mobileBridge.InboundEnvelope(rdyMobile); err != nil {
		t.Fatalf("mobile ready envelope: %v", err)
	}

	if err := pcPeer.WaitForState(StateConnected, 10*time.Second); err != nil {
		t.Fatalf("pc connected: %v", err)
	}
	if err := mobilePeer.WaitForState(StateConnected, 10*time.Second); err != nil {
		t.Fatalf("mobile connected: %v", err)
	}

	if err := pcPeer.WaitDataChannelOpen(5 * time.Second); err != nil {
		t.Fatalf("pc datachannel open: %v", err)
	}
	if err := mobilePeer.WaitDataChannelOpen(5 * time.Second); err != nil {
		t.Fatalf("mobile datachannel open: %v", err)
	}

	payload := []byte("bridge-message")
	if err := pcPeer.Send(payload); err != nil {
		t.Fatalf("pc send: %v", err)
	}

	select {
	case got := <-mobilePeer.Messages():
		if !bytes.Equal(got, payload) {
			t.Fatalf("payload mismatch: got=%q want=%q", string(got), string(payload))
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout waiting for mobile message")
	}

	assertNoBridgeError(t, pcBridge, 150*time.Millisecond)
	assertNoBridgeError(t, mobileBridge, 150*time.Millisecond)
}

func TestSignalBridgeValidation(t *testing.T) {
	pcPeer, err := NewPeer(DefaultConfig(SidePC))
	if err != nil {
		t.Fatalf("new pc peer: %v", err)
	}
	defer func() { _ = pcPeer.Close() }()

	bridge, err := NewSignalBridge("sid-expected", SidePC, pcPeer)
	if err != nil {
		t.Fatalf("new bridge: %v", err)
	}

	rdy, _ := protocol.NewEnvelope("sid-other", "rid", 1, protocol.TypeSignalReady, protocol.SignalReadyPayload{
		Role:          "pc",
		PeerConnected: true,
		Timestamp:     time.Now().UTC().UnixMilli(),
	})
	if err := bridge.ProcessEnvelope(rdy); err == nil {
		t.Fatalf("sid mismatch should fail")
	}
}

func TestSignalBridgeStartOfferRoleGuard(t *testing.T) {
	mobilePeer, err := NewPeer(DefaultConfig(SideMobile))
	if err != nil {
		t.Fatalf("new mobile peer: %v", err)
	}
	defer func() { _ = mobilePeer.Close() }()

	bridge, err := NewSignalBridge("sid-1", SideMobile, mobilePeer)
	if err != nil {
		t.Fatalf("new mobile bridge: %v", err)
	}

	if err := bridge.StartOffer(); err == nil {
		t.Fatalf("mobile StartOffer should fail")
	}
}

func relayOutbound(ctx context.Context, src <-chan protocol.Envelope, target *SignalBridge) {
	for {
		select {
		case env := <-src:
			_ = target.InboundEnvelope(env)
		case <-ctx.Done():
			return
		}
	}
}

func assertNoBridgeError(t *testing.T, bridge *SignalBridge, wait time.Duration) {
	t.Helper()

	select {
	case err := <-bridge.Errors():
		t.Fatalf("unexpected bridge error: %v", err)
	case <-time.After(wait):
	}
}
