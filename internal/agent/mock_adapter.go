package agent

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/Fharena/VibeDeck/internal/protocol"
)

type MockAdapter struct {
	taskSeq atomic.Int64
	runSeq  atomic.Int64
}

func NewMockAdapter() *MockAdapter {
	return &MockAdapter{}
}

func (a *MockAdapter) Name() string {
	return "mock-cursor"
}

func (a *MockAdapter) Capabilities() AdapterCapabilities {
	return AdapterCapabilities{
		SupportsPartialApply:     true,
		SupportsStructuredPatch:  true,
		SupportsContextSelection: true,
		SupportsArtifacts:        false,
		SupportsOpenLocation:     true,
	}
}

func (a *MockAdapter) GetContext(_ context.Context, input ContextRequest) (WorkspaceContext, error) {
	return WorkspaceContext{
		ActiveFilePath:      "src/auth/middleware.ts",
		Selection:           "if (!token) return 401",
		LatestTerminalError: "expected 401 got 500",
		ChangedFiles:        []string{"src/auth/middleware.ts", "tests/auth/middleware.test.ts"},
		LastRunProfile:      "test_all",
		LastRunStatus:       "failed",
	}, nil
}

func (a *MockAdapter) SubmitTask(_ context.Context, _ SubmitTaskInput) (TaskHandle, error) {
	id := a.taskSeq.Add(1)
	return TaskHandle{TaskID: fmt.Sprintf("task_%d", id)}, nil
}

func (a *MockAdapter) GetPatch(_ context.Context, _ string) (*protocol.PatchReadyPayload, error) {
	return &protocol.PatchReadyPayload{
		Summary: "Fix auth middleware test path and null guard",
		Files: []protocol.FilePatch{
			{
				Path:   "src/auth/middleware.ts",
				Status: "modified",
				Hunks: []protocol.Hunk{
					{
						HunkID: "h1",
						Header: "@@ -12,7 +12,9 @@",
						Diff:   "- if (!token) throw new Error()\n+ if (!token) return res.status(401).send()",
						Risk:   "low",
					},
				},
			},
		},
	}, nil
}

func (a *MockAdapter) ApplyPatch(_ context.Context, input ApplyPatchInput) (ApplyPatchResult, error) {
	if input.Mode == "selected" && len(input.Selected) == 0 {
		return ApplyPatchResult{Status: "failed", Message: "selected mode requires hunk selection"}, nil
	}

	if input.Mode == "selected" {
		return ApplyPatchResult{Status: "partial", Message: "selected hunks applied"}, nil
	}

	return ApplyPatchResult{Status: "success", Message: "patch applied"}, nil
}

func (a *MockAdapter) RunProfile(_ context.Context, input RunProfileInput) (RunHandle, error) {
	id := a.runSeq.Add(1)
	return RunHandle{RunID: fmt.Sprintf("run_%s_%d", input.ProfileID, id)}, nil
}

func (a *MockAdapter) GetRunResult(_ context.Context, runID string) (*RunResult, error) {
	return &RunResult{
		RunID:     runID,
		ProfileID: "test_all",
		Status:    "failed",
		Summary:   "1 failing test in auth middleware",
		TopErrors: []protocol.ParsedError{
			{
				Message: "expected 401 got 500",
				Path:    "tests/auth/middleware.test.ts",
				Line:    44,
				Column:  13,
			},
		},
		Excerpt: "AssertionError: expected 401 got 500",
		Output:  "FAIL tests/auth/middleware.test.ts\nAssertionError: expected 401 got 500",
	}, nil
}

func (a *MockAdapter) OpenLocation(_ context.Context, _ OpenLocationInput) error {
	return nil
}
