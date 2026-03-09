package agent

import (
	"testing"

	"github.com/Fharena/VibeDeck/internal/protocol"
)

func TestSessionStoreBuildsReadModelFromThreadDetail(t *testing.T) {
	threadStore := NewThreadStore()
	threadStore.EnsureThread("thread-1", "sid-1", "Fix auth middleware")
	threadStore.AssignJob("thread-1", "job-1")

	_, _ = threadStore.AppendEvent("thread-1", ThreadEvent{
		JobID: "job-1",
		Kind:  "prompt_submitted",
		Role:  "user",
		Body:  "Fix auth middleware",
		Data: map[string]any{
			"activeFilePath": "middleware/auth.go",
			"selection":      "requireUser",
		},
	})
	_, _ = threadStore.AppendEvent("thread-1", ThreadEvent{
		JobID: "job-1",
		Kind:  "patch_ready",
		Role:  "assistant",
		Body:  "auth patch",
		Data: map[string]any{
			"summary": "auth patch",
			"files": []protocol.FilePatch{
				{Path: "middleware/auth.go", Status: "modified"},
			},
		},
	})
	_, _ = threadStore.AppendEvent("thread-1", ThreadEvent{
		JobID: "job-1",
		Kind:  "run_finished",
		Role:  "system",
		Body:  "tests failed",
		Data: map[string]any{
			"profileId": "test_all",
			"status":    "failed",
			"summary":   "tests failed",
			"changedFiles": []string{
				"middleware/auth.go",
			},
			"topErrors": []protocol.ParsedError{
				{Path: "middleware/auth_test.go", Line: 27, Message: "expected 200"},
			},
		},
	})

	store := NewSessionStore(threadStore, AdapterRuntimeInfo{
		Name:          "mock-cursor",
		Mode:          "cursor_bridge",
		WorkspaceRoot: `C:\repo\workspace`,
	})

	detail, ok := store.Get("thread-1")
	if !ok {
		t.Fatalf("expected session detail to exist")
	}
	if detail.Session.ID != "thread-1" {
		t.Fatalf("expected session id thread-1, got %+v", detail.Session)
	}
	if detail.Session.ControlSessionID != "sid-1" {
		t.Fatalf("expected control session id sid-1, got %+v", detail.Session)
	}
	if detail.Session.Provider != "cursor" {
		t.Fatalf("expected provider cursor, got %+v", detail.Session)
	}
	if detail.OperationState.Phase != "waiting_input" {
		t.Fatalf("expected phase waiting_input, got %+v", detail.OperationState)
	}
	if detail.OperationState.CurrentJobID != "job-1" {
		t.Fatalf("expected current job id job-1, got %+v", detail.OperationState)
	}
	if detail.OperationState.PatchSummary != "auth patch" {
		t.Fatalf("expected patch summary auth patch, got %+v", detail.OperationState)
	}
	if detail.OperationState.RunProfileID != "test_all" {
		t.Fatalf("expected run profile id test_all, got %+v", detail.OperationState)
	}
	if detail.OperationState.LastError != "tests failed" {
		t.Fatalf("expected last error tests failed, got %+v", detail.OperationState)
	}
	if len(detail.OperationState.CurrentJobFiles) != 1 || detail.OperationState.CurrentJobFiles[0] != "middleware/auth.go" {
		t.Fatalf("expected current job files to include middleware/auth.go, got %+v", detail.OperationState.CurrentJobFiles)
	}
	if detail.LiveState.Composer.DraftText != "Fix auth middleware" {
		t.Fatalf("expected composer draft from prompt, got %+v", detail.LiveState)
	}
	if detail.LiveState.Focus.ActiveFilePath != "middleware/auth.go" {
		t.Fatalf("expected active file path middleware/auth.go, got %+v", detail.LiveState.Focus)
	}
	if detail.LiveState.Focus.PatchPath != "middleware/auth.go" {
		t.Fatalf("expected patch path middleware/auth.go, got %+v", detail.LiveState.Focus)
	}
	if detail.LiveState.Focus.RunErrorPath != "middleware/auth_test.go" || detail.LiveState.Focus.RunErrorLine != 27 {
		t.Fatalf("expected run error focus from top errors, got %+v", detail.LiveState.Focus)
	}
}

func TestSessionStoreMergesLiveOverrides(t *testing.T) {
	threadStore := NewThreadStore()
	threadStore.EnsureThread("thread-override", "sid-2", "Override test")

	store := NewSessionStore(threadStore, AdapterRuntimeInfo{})
	store.UpsertParticipant("thread-override", SessionParticipant{
		ParticipantID: "mobile-1",
		ClientType:    "mobile",
		DisplayName:   "iPhone",
		Active:        true,
		LastSeenAt:    100,
	})
	store.UpdateComposer("thread-override", SessionComposerState{
		DraftText: "새 draft",
		IsTyping:  true,
		UpdatedAt: 101,
	})
	store.UpdateFocus("thread-override", SessionFocusState{
		ActiveFilePath: "mobile/app.dart",
		Selection:      "build",
		UpdatedAt:      102,
	})

	detail, ok := store.Get("thread-override")
	if !ok {
		t.Fatalf("expected session detail to exist")
	}
	if len(detail.LiveState.Participants) != 1 || detail.LiveState.Participants[0].ParticipantID != "mobile-1" {
		t.Fatalf("expected live participant override, got %+v", detail.LiveState.Participants)
	}
	if detail.Session.ControlSessionID != "sid-2" {
		t.Fatalf("expected control session id sid-2, got %+v", detail.Session)
	}
	if detail.LiveState.Composer.DraftText != "새 draft" || !detail.LiveState.Composer.IsTyping {
		t.Fatalf("expected composer override, got %+v", detail.LiveState.Composer)
	}
	if detail.LiveState.Focus.ActiveFilePath != "mobile/app.dart" || detail.LiveState.Focus.Selection != "build" {
		t.Fatalf("expected focus override, got %+v", detail.LiveState.Focus)
	}
}
