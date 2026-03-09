package agent

import (
	"github.com/Fharena/VibeDeck/internal/protocol"
	"github.com/Fharena/VibeDeck/internal/runtime"
)

func registerAckableResponses(
	tracker *runtime.AckTracker,
	responses []protocol.Envelope,
	transport runtime.AckTransport,
	retryEnabled bool,
) {
	if tracker == nil {
		return
	}

	for _, response := range responses {
		if response.Type == protocol.TypeCmdAck {
			continue
		}
		tracker.RegisterEnvelope(response, transport, retryEnabled)
	}
}
