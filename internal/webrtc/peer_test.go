package webrtc

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/Fharena/Vivedeck/internal/protocol"
)

func TestPeerOfferAnswerAndDataChannel(t *testing.T) {
	pc, err := NewPeer(DefaultConfig(SidePC))
	if err != nil {
		t.Fatalf("new pc peer: %v", err)
	}
	defer func() { _ = pc.Close() }()

	mobile, err := NewPeer(DefaultConfig(SideMobile))
	if err != nil {
		t.Fatalf("new mobile peer: %v", err)
	}
	defer func() { _ = mobile.Close() }()

	offer, err := pc.CreateOffer()
	if err != nil {
		t.Fatalf("create offer: %v", err)
	}

	answer, err := mobile.ApplyOfferAndCreateAnswer(offer)
	if err != nil {
		t.Fatalf("apply offer and create answer: %v", err)
	}

	if err := pc.ApplyAnswer(answer); err != nil {
		t.Fatalf("apply answer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	go forwardCandidates(ctx, t, pc.LocalCandidates(), mobile)
	go forwardCandidates(ctx, t, mobile.LocalCandidates(), pc)

	if err := pc.WaitForState(StateConnected, 10*time.Second); err != nil {
		t.Fatalf("pc wait connected: %v", err)
	}
	if err := mobile.WaitForState(StateConnected, 10*time.Second); err != nil {
		t.Fatalf("mobile wait connected: %v", err)
	}

	if err := pc.WaitDataChannelOpen(5 * time.Second); err != nil {
		t.Fatalf("pc data channel open: %v", err)
	}
	if err := mobile.WaitDataChannelOpen(5 * time.Second); err != nil {
		t.Fatalf("mobile data channel open: %v", err)
	}

	payload := []byte("ping-from-pc")
	if err := pc.Send(payload); err != nil {
		t.Fatalf("pc send: %v", err)
	}

	select {
	case received := <-mobile.Messages():
		if !bytes.Equal(received, payload) {
			t.Fatalf("message mismatch: got=%q want=%q", string(received), string(payload))
		}
	case <-time.After(4 * time.Second):
		t.Fatalf("timeout waiting mobile message")
	}
}

func TestPeerRoleValidation(t *testing.T) {
	mobile, err := NewPeer(DefaultConfig(SideMobile))
	if err != nil {
		t.Fatalf("new mobile peer: %v", err)
	}
	defer func() { _ = mobile.Close() }()

	if _, err := mobile.CreateOffer(); err == nil {
		t.Fatalf("mobile CreateOffer should fail")
	}

	pc, err := NewPeer(DefaultConfig(SidePC))
	if err != nil {
		t.Fatalf("new pc peer: %v", err)
	}
	defer func() { _ = pc.Close() }()

	if _, err := pc.ApplyOfferAndCreateAnswer("dummy"); err == nil {
		t.Fatalf("pc ApplyOfferAndCreateAnswer should fail")
	}
}

func TestAddRemoteCandidateValidation(t *testing.T) {
	pc, err := NewPeer(DefaultConfig(SidePC))
	if err != nil {
		t.Fatalf("new pc peer: %v", err)
	}
	defer func() { _ = pc.Close() }()

	err = pc.AddRemoteICECandidate(protocol.SignalICEPayload{Candidate: ""})
	if err == nil {
		t.Fatalf("empty candidate should fail")
	}
}

func forwardCandidates(ctx context.Context, t *testing.T, candidates <-chan protocol.SignalICEPayload, target *Peer) {
	t.Helper()

	for {
		select {
		case candidate, ok := <-candidates:
			if !ok {
				return
			}

			if err := target.AddRemoteICECandidate(candidate); err != nil {
				// Ignore candidates arriving after close race.
				if ctx.Err() != nil {
					return
				}
				t.Logf("candidate forward warning: %v", err)
			}

		case <-ctx.Done():
			return
		}
	}
}
