package relay

import (
    "testing"

    "github.com/Fharena/Vivedeck/internal/protocol"
)

func TestSessionEmitsTermSummaryAfterDrops(t *testing.T) {
    session := NewSession("s1", 1)
    _ = session.Register("pc")
    outbound := session.Register("mobile")

    term1, _ := protocol.NewEnvelope("s1", "r1", 1, protocol.TypeTerm, map[string]string{"line": "one"})
    term2, _ := protocol.NewEnvelope("s1", "r2", 2, protocol.TypeTerm, map[string]string{"line": "two"})
    term3, _ := protocol.NewEnvelope("s1", "r3", 3, protocol.TypeTerm, map[string]string{"line": "three"})

    if err := session.Route("pc", term1); err != nil {
        t.Fatalf("route term1: %v", err)
    }
    if err := session.Route("pc", term2); err != nil {
        t.Fatalf("route term2: %v", err)
    }

    first := <-outbound
    if first.Type != protocol.TypeTerm {
        t.Fatalf("expected first payload to be TERM")
    }

    if err := session.Route("pc", term3); err != nil {
        t.Fatalf("route term3: %v", err)
    }

    second := <-outbound
    if second.Type != protocol.TypeTermSummary {
        t.Fatalf("expected TERM_SUMMARY after drop, got %s", second.Type)
    }
}

func TestSessionControlPathTimeoutWhenQueueFull(t *testing.T) {
    session := NewSession("s1", 1)
    _ = session.Register("pc")
    _ = session.Register("mobile")

    prompt1, _ := protocol.NewEnvelope("s1", "r1", 1, protocol.TypePromptSubmit, map[string]string{"prompt": "a"})
    prompt2, _ := protocol.NewEnvelope("s1", "r2", 2, protocol.TypePromptSubmit, map[string]string{"prompt": "b"})

    if err := session.Route("pc", prompt1); err != nil {
        t.Fatalf("route prompt1: %v", err)
    }

    if err := session.Route("pc", prompt2); err == nil {
        t.Fatalf("second control message should timeout when queue is full")
    }
}
