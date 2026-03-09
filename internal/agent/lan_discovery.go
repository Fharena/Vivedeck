package agent

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

const (
	DefaultLANDiscoveryAddr = ":42777"
	lanDiscoveryProbeType   = "vibedeck_discover"
	lanDiscoveryResultType  = "vibedeck_discover_result"
	lanDiscoveryVersion     = 1
)

type LANDiscoveryConfig struct {
	Addr            string
	AgentListenAddr string
	Server          *HTTPServer
}

type lanDiscoveryProbe struct {
	Type    string `json:"type"`
	Version int    `json:"version"`
}

type LANDiscoveryResponse struct {
	Type        string `json:"type"`
	Version     int    `json:"version"`
	DisplayName string `json:"displayName,omitempty"`
	RespondedAt int64  `json:"respondedAt"`
	BootstrapResponse
}

type lanDiscoveryResponder struct {
	conn            net.PacketConn
	server          *HTTPServer
	agentListenAddr string
}

func StartLANDiscoveryResponder(cfg LANDiscoveryConfig) (io.Closer, error) {
	addr := strings.TrimSpace(cfg.Addr)
	if addr == "" {
		return nil, nil
	}
	if cfg.Server == nil {
		return nil, errors.New("lan discovery server is nil")
	}

	conn, err := net.ListenPacket("udp4", addr)
	if err != nil {
		return nil, err
	}

	responder := &lanDiscoveryResponder{
		conn:            conn,
		server:          cfg.Server,
		agentListenAddr: strings.TrimSpace(cfg.AgentListenAddr),
	}
	go responder.serve()
	return responder, nil
}

func (r *lanDiscoveryResponder) Close() error {
	return r.conn.Close()
}

func (r *lanDiscoveryResponder) serve() {
	buffer := make([]byte, 8192)
	for {
		n, addr, err := r.conn.ReadFrom(buffer)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			log.Printf("lan discovery read failed: %v", err)
			continue
		}

		if !acceptLANDiscoveryProbe(buffer[:n]) {
			continue
		}

		payload, err := json.Marshal(r.buildResponse())
		if err != nil {
			log.Printf("lan discovery response marshal failed: %v", err)
			continue
		}
		if _, err := r.conn.WriteTo(payload, addr); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Printf("lan discovery response write failed: %v", err)
		}
	}
}

func (r *lanDiscoveryResponder) buildResponse() LANDiscoveryResponse {
	displayName, _ := os.Hostname()
	agentBaseURL := resolveDiscoveryAgentBaseURL(
		r.agentListenAddr,
		r.server.config.PublicAgentBaseURL,
	)
	signalingBaseURL := resolveDiscoverySignalingBaseURL(
		configuredBootstrapSignalingBaseURL(r.server.p2pManager),
		r.server.config.PublicSignalingBaseURL,
		hostFromBaseURL(agentBaseURL),
	)

	return LANDiscoveryResponse{
		Type:        lanDiscoveryResultType,
		Version:     lanDiscoveryVersion,
		DisplayName: strings.TrimSpace(displayName),
		RespondedAt: time.Now().UTC().UnixMilli(),
		BootstrapResponse: buildBootstrapResponse(
			r.server.adapter,
			r.server.orchestrator,
			r.server.p2pManager,
			r.server.config,
			agentBaseURL,
			signalingBaseURL,
		),
	}
}

func acceptLANDiscoveryProbe(raw []byte) bool {
	var probe lanDiscoveryProbe
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(probe.Type), lanDiscoveryProbeType) &&
		(probe.Version == 0 || probe.Version == lanDiscoveryVersion)
}
