package agent

import (
	"time"

	"github.com/Fharena/Vivedeck/internal/protocol"
)

func controlEnvelopeTimeout(messageType protocol.MessageType) time.Duration {
	switch messageType {
	case protocol.TypePromptSubmit, protocol.TypeRunProfile:
		return 5 * time.Minute
	case protocol.TypePatchApply:
		return 30 * time.Second
	default:
		return 5 * time.Second
	}
}
