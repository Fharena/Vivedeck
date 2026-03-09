package agent

import (
	"testing"
	"time"

	"github.com/Fharena/VibeDeck/internal/protocol"
)

func TestControlEnvelopeTimeout(t *testing.T) {
	cases := []struct {
		name string
		typ  protocol.MessageType
		want time.Duration
	}{
		{name: "prompt", typ: protocol.TypePromptSubmit, want: 5 * time.Minute},
		{name: "run", typ: protocol.TypeRunProfile, want: 5 * time.Minute},
		{name: "patch", typ: protocol.TypePatchApply, want: 30 * time.Second},
		{name: "ack", typ: protocol.TypeCmdAck, want: 5 * time.Second},
	}

	for _, tc := range cases {
		if got := controlEnvelopeTimeout(tc.typ); got != tc.want {
			t.Fatalf("%s: want %s, got %s", tc.name, tc.want, got)
		}
	}
}
