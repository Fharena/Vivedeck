package agent

import (
	"context"
	"errors"

	"github.com/Fharena/Vivedeck/internal/protocol"
	"github.com/Fharena/Vivedeck/internal/runtime"
)

type ControlHandleResult struct {
	Responses    []protocol.Envelope
	AckHandled   bool
	AckRequestID string
}

type ControlRouter struct {
	orchestrator *Orchestrator
	ackTracker   *runtime.AckTracker
}

func NewControlRouter(orchestrator *Orchestrator, ackTracker *runtime.AckTracker) *ControlRouter {
	return &ControlRouter{
		orchestrator: orchestrator,
		ackTracker:   ackTracker,
	}
}

func (r *ControlRouter) HandleEnvelope(ctx context.Context, env protocol.Envelope) (ControlHandleResult, error) {
	if env.Type == protocol.TypeCmdAck {
		if r.ackTracker == nil {
			return ControlHandleResult{}, errors.New("ack tracker is not configured")
		}

		var payload protocol.CmdAckPayload
		if err := env.DecodePayload(&payload); err != nil {
			return ControlHandleResult{}, err
		}

		return ControlHandleResult{
			AckHandled:   r.ackTracker.Ack(payload.RequestRID),
			AckRequestID: payload.RequestRID,
		}, nil
	}

	if r.orchestrator == nil {
		return ControlHandleResult{}, errors.New("orchestrator is not configured")
	}

	responses, err := r.orchestrator.HandleEnvelope(ctx, env)
	if r.ackTracker != nil {
		for _, response := range responses {
			if response.Type == protocol.TypeCmdAck {
				continue
			}
			r.ackTracker.Register(response.SID, response.RID, string(response.Type))
		}
	}

	return ControlHandleResult{Responses: responses}, err
}
