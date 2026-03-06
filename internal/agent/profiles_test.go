package agent

import "testing"

func TestLoadRunProfilesFallback(t *testing.T) {
    profiles, err := LoadRunProfiles("not-found.json")
    if err != nil {
        t.Fatalf("load run profiles fallback: %v", err)
    }

    if _, ok := profiles["test_all"]; !ok {
        t.Fatalf("default profile test_all should exist")
    }
}
