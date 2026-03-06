package webrtc

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/Fharena/Vivedeck/internal/protocol"
)

type SignalBridge struct {
	sessionID string
	side      Side
	peer      *Peer

	inbound  chan protocol.Envelope
	outbound chan protocol.Envelope
	errors   chan error

	seq          atomic.Int64
	offerStarted atomic.Bool
	closed       atomic.Bool
}

func NewSignalBridge(sessionID string, side Side, peer *Peer) (*SignalBridge, error) {
	if sessionID == "" {
		return nil, errors.New("sessionID is required")
	}
	if side == "" {
		return nil, errors.New("side is required")
	}
	if peer == nil {
		return nil, errors.New("peer is required")
	}

	return &SignalBridge{
		sessionID: sessionID,
		side:      side,
		peer:      peer,
		inbound:   make(chan protocol.Envelope, 256),
		outbound:  make(chan protocol.Envelope, 256),
		errors:    make(chan error, 128),
	}, nil
}

func (b *SignalBridge) InboundEnvelope(env protocol.Envelope) error {
	if b.closed.Load() {
		return errors.New("bridge is closed")
	}

	select {
	case b.inbound <- env:
		return nil
	default:
		return errors.New("inbound queue is full")
	}
}

func (b *SignalBridge) Outbound() <-chan protocol.Envelope {
	return b.outbound
}

func (b *SignalBridge) Errors() <-chan error {
	return b.errors
}

func (b *SignalBridge) Run(ctx context.Context) {
	for {
		select {
		case env := <-b.inbound:
			if err := b.ProcessEnvelope(env); err != nil {
				b.reportError(err)
			}

		case candidate := <-b.peer.LocalCandidates():
			if candidate.Candidate == "" {
				continue
			}

			if err := b.emit(protocol.TypeSignalICE, protocol.SignalICEPayload{
				Candidate:     candidate.Candidate,
				SDPMid:        candidate.SDPMid,
				SDPMLineIndex: candidate.SDPMLineIndex,
			}); err != nil {
				b.reportError(err)
			}

		case <-ctx.Done():
			b.closed.Store(true)
			return
		}
	}
}

func (b *SignalBridge) ProcessEnvelope(env protocol.Envelope) error {
	if env.SID != b.sessionID {
		return fmt.Errorf("sid mismatch: expected=%s got=%s", b.sessionID, env.SID)
	}

	switch env.Type {
	case protocol.TypeSignalReady:
		var payload protocol.SignalReadyPayload
		if err := env.DecodePayload(&payload); err != nil {
			return fmt.Errorf("decode signal ready payload: %w", err)
		}

		if payload.PeerConnected && b.side == SidePC {
			return b.StartOffer()
		}

		return nil

	case protocol.TypeSignalOffer:
		if b.side != SideMobile {
			return errors.New("only mobile bridge can process SIGNAL_OFFER")
		}

		var payload protocol.SignalOfferPayload
		if err := env.DecodePayload(&payload); err != nil {
			return fmt.Errorf("decode signal offer payload: %w", err)
		}

		answer, err := b.peer.ApplyOfferAndCreateAnswer(payload.SDP)
		if err != nil {
			return fmt.Errorf("apply offer/create answer: %w", err)
		}

		return b.emit(protocol.TypeSignalAnswer, protocol.SignalAnswerPayload{SDP: answer})

	case protocol.TypeSignalAnswer:
		if b.side != SidePC {
			return errors.New("only pc bridge can process SIGNAL_ANSWER")
		}

		var payload protocol.SignalAnswerPayload
		if err := env.DecodePayload(&payload); err != nil {
			return fmt.Errorf("decode signal answer payload: %w", err)
		}

		if err := b.peer.ApplyAnswer(payload.SDP); err != nil {
			return fmt.Errorf("apply answer: %w", err)
		}

		return nil

	case protocol.TypeSignalICE:
		var payload protocol.SignalICEPayload
		if err := env.DecodePayload(&payload); err != nil {
			return fmt.Errorf("decode signal ice payload: %w", err)
		}

		if err := b.peer.AddRemoteICECandidate(payload); err != nil {
			return fmt.Errorf("add remote ice candidate: %w", err)
		}

		return nil

	case protocol.TypeCmdAck:
		// signaling path ack is ignored in bridge runtime.
		return nil

	default:
		return fmt.Errorf("unsupported signaling envelope type: %s", env.Type)
	}
}

func (b *SignalBridge) StartOffer() error {
	if b.side != SidePC {
		return errors.New("only pc bridge can start offer")
	}
	if !b.offerStarted.CompareAndSwap(false, true) {
		return nil
	}

	offer, err := b.peer.CreateOffer()
	if err != nil {
		return fmt.Errorf("create offer: %w", err)
	}

	if err := b.emit(protocol.TypeSignalOffer, protocol.SignalOfferPayload{SDP: offer}); err != nil {
		return err
	}

	return nil
}

func (b *SignalBridge) emit(typ protocol.MessageType, payload any) error {
	env, err := protocol.NewEnvelope(
		b.sessionID,
		fmt.Sprintf("bridge_%s_%d", typ, time.Now().UTC().UnixNano()),
		b.seq.Add(1),
		typ,
		payload,
	)
	if err != nil {
		return fmt.Errorf("build envelope: %w", err)
	}

	select {
	case b.outbound <- env:
		return nil
	default:
		return errors.New("outbound queue is full")
	}
}

func (b *SignalBridge) reportError(err error) {
	if err == nil {
		return
	}

	select {
	case b.errors <- err:
	default:
	}
}
