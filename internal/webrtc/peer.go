package webrtc

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Fharena/Vivedeck/internal/protocol"
	pion "github.com/pion/webrtc/v4"
)

type Peer struct {
	cfg Config

	pc *pion.PeerConnection

	mu sync.RWMutex
	dc *pion.DataChannel

	closed  atomic.Bool
	dcReady atomic.Bool

	localCandidates chan protocol.SignalICEPayload
	messages        chan []byte
	states          chan PeerState
}

func NewPeer(cfg Config) (*Peer, error) {
	if cfg.Side == "" {
		return nil, errors.New("side is required")
	}
	if cfg.DataChannelLabel == "" {
		cfg.DataChannelLabel = DefaultConfig(cfg.Side).DataChannelLabel
	}
	if cfg.OfferTimeout <= 0 {
		cfg.OfferTimeout = DefaultConfig(cfg.Side).OfferTimeout
	}

	pc, err := pion.NewPeerConnection(pion.Configuration{})
	if err != nil {
		return nil, fmt.Errorf("create peer connection: %w", err)
	}

	p := &Peer{
		cfg:             cfg,
		pc:              pc,
		localCandidates: make(chan protocol.SignalICEPayload, 128),
		messages:        make(chan []byte, 128),
		states:          make(chan PeerState, 32),
	}

	p.bindBaseHandlers()

	// Offerer(PC) creates control channel proactively.
	if cfg.Side == SidePC {
		dc, err := p.pc.CreateDataChannel(cfg.DataChannelLabel, nil)
		if err != nil {
			_ = p.pc.Close()
			return nil, fmt.Errorf("create data channel: %w", err)
		}
		p.bindDataChannel(dc)
	}

	return p, nil
}

func (p *Peer) bindBaseHandlers() {
	p.pc.OnICECandidate(func(candidate *pion.ICECandidate) {
		if candidate == nil || p.closed.Load() {
			return
		}

		payload := toSignalICEPayload(candidate)
		select {
		case p.localCandidates <- payload:
		default:
			// best effort queue to keep control path responsive
		}
	})

	p.pc.OnConnectionStateChange(func(state pion.PeerConnectionState) {
		if p.closed.Load() {
			return
		}

		mapped := mapPeerState(state)
		select {
		case p.states <- mapped:
		default:
		}
	})

	p.pc.OnDataChannel(func(dc *pion.DataChannel) {
		p.bindDataChannel(dc)
	})
}

func (p *Peer) bindDataChannel(dc *pion.DataChannel) {
	p.mu.Lock()
	p.dc = dc
	p.mu.Unlock()

	dc.OnOpen(func() {
		if p.closed.Load() {
			return
		}
		p.dcReady.Store(true)
	})

	dc.OnClose(func() {
		p.dcReady.Store(false)
	})

	dc.OnMessage(func(msg pion.DataChannelMessage) {
		if p.closed.Load() {
			return
		}

		data := make([]byte, len(msg.Data))
		copy(data, msg.Data)

		select {
		case p.messages <- data:
		default:
		}
	})
}

func (p *Peer) CreateOffer() (string, error) {
	if p.cfg.Side != SidePC {
		return "", errors.New("only pc peer can create offer")
	}

	offer, err := p.pc.CreateOffer(nil)
	if err != nil {
		return "", fmt.Errorf("create offer: %w", err)
	}

	if err := p.pc.SetLocalDescription(offer); err != nil {
		return "", fmt.Errorf("set local offer: %w", err)
	}

	if err := p.waitGatheringComplete(p.cfg.OfferTimeout); err != nil {
		return "", err
	}

	local := p.pc.LocalDescription()
	if local == nil {
		return "", errors.New("local description is nil")
	}

	return local.SDP, nil
}

func (p *Peer) ApplyOfferAndCreateAnswer(offerSDP string) (string, error) {
	if p.cfg.Side != SideMobile {
		return "", errors.New("only mobile peer can apply offer")
	}

	if offerSDP == "" {
		return "", errors.New("offer sdp is required")
	}

	if err := p.pc.SetRemoteDescription(pion.SessionDescription{Type: pion.SDPTypeOffer, SDP: offerSDP}); err != nil {
		return "", fmt.Errorf("set remote offer: %w", err)
	}

	answer, err := p.pc.CreateAnswer(nil)
	if err != nil {
		return "", fmt.Errorf("create answer: %w", err)
	}

	if err := p.pc.SetLocalDescription(answer); err != nil {
		return "", fmt.Errorf("set local answer: %w", err)
	}

	if err := p.waitGatheringComplete(p.cfg.OfferTimeout); err != nil {
		return "", err
	}

	local := p.pc.LocalDescription()
	if local == nil {
		return "", errors.New("local answer is nil")
	}

	return local.SDP, nil
}

func (p *Peer) ApplyAnswer(answerSDP string) error {
	if p.cfg.Side != SidePC {
		return errors.New("only pc peer can apply answer")
	}

	if answerSDP == "" {
		return errors.New("answer sdp is required")
	}

	if err := p.pc.SetRemoteDescription(pion.SessionDescription{Type: pion.SDPTypeAnswer, SDP: answerSDP}); err != nil {
		return fmt.Errorf("set remote answer: %w", err)
	}

	return nil
}

func (p *Peer) AddRemoteICECandidate(payload protocol.SignalICEPayload) error {
	if payload.Candidate == "" {
		return errors.New("candidate is required")
	}

	var mid *string
	if payload.SDPMid != "" {
		m := payload.SDPMid
		mid = &m
	}

	var idx *uint16
	if payload.SDPMLineIndex >= 0 {
		line := uint16(payload.SDPMLineIndex)
		idx = &line
	}

	if err := p.pc.AddICECandidate(pion.ICECandidateInit{
		Candidate:     payload.Candidate,
		SDPMid:        mid,
		SDPMLineIndex: idx,
	}); err != nil {
		return fmt.Errorf("add remote candidate: %w", err)
	}

	return nil
}

func (p *Peer) Send(data []byte) error {
	if p.closed.Load() {
		return errors.New("peer is closed")
	}

	if !p.dcReady.Load() {
		return errors.New("data channel is not open")
	}

	p.mu.RLock()
	dc := p.dc
	p.mu.RUnlock()
	if dc == nil {
		return errors.New("data channel is nil")
	}

	if err := dc.Send(data); err != nil {
		return fmt.Errorf("send data channel message: %w", err)
	}

	return nil
}

func (p *Peer) LocalCandidates() <-chan protocol.SignalICEPayload {
	return p.localCandidates
}

func (p *Peer) Messages() <-chan []byte {
	return p.messages
}

func (p *Peer) States() <-chan PeerState {
	return p.states
}

func (p *Peer) WaitForState(target PeerState, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case state := <-p.states:
			if state == target {
				return nil
			}
			if state == StateFailed || state == StateClosed {
				return fmt.Errorf("peer entered terminal state: %s", state)
			}

		case <-timer.C:
			return fmt.Errorf("wait for state timeout: %s", target)
		}
	}
}

func (p *Peer) WaitDataChannelOpen(timeout time.Duration) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		if p.dcReady.Load() {
			return nil
		}

		select {
		case <-ticker.C:
		case <-timer.C:
			return errors.New("wait data channel open timeout")
		}
	}
}

func (p *Peer) Close() error {
	if p.closed.Swap(true) {
		return nil
	}

	p.dcReady.Store(false)

	if err := p.pc.Close(); err != nil {
		return fmt.Errorf("close peer connection: %w", err)
	}

	return nil
}

func (p *Peer) waitGatheringComplete(timeout time.Duration) error {
	gatherDone := pion.GatheringCompletePromise(p.pc)

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-gatherDone:
		return nil
	case <-timer.C:
		return errors.New("ice gathering timeout")
	}
}

func toSignalICEPayload(candidate *pion.ICECandidate) protocol.SignalICEPayload {
	c := candidate.ToJSON()

	line := 0
	if c.SDPMLineIndex != nil {
		line = int(*c.SDPMLineIndex)
	}

	mid := ""
	if c.SDPMid != nil {
		mid = *c.SDPMid
	}

	return protocol.SignalICEPayload{
		Candidate:     c.Candidate,
		SDPMid:        mid,
		SDPMLineIndex: line,
	}
}

func mapPeerState(state pion.PeerConnectionState) PeerState {
	switch state {
	case pion.PeerConnectionStateNew:
		return StateNew
	case pion.PeerConnectionStateConnecting:
		return StateConnecting
	case pion.PeerConnectionStateConnected:
		return StateConnected
	case pion.PeerConnectionStateDisconnected:
		return StateDisconnected
	case pion.PeerConnectionStateFailed:
		return StateFailed
	case pion.PeerConnectionStateClosed:
		return StateClosed
	default:
		return StateNew
	}
}
