package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/Fharena/Vivedeck/internal/protocol"
)

func TestCursorBridgeAdapterRoundTrip(t *testing.T) {
	adapter, err := NewCursorBridgeAdapter(context.Background(), CursorBridgeProcessConfig{
		Command:        os.Args[0],
		Args:           []string{"-test.run=TestCursorBridgeHelperProcess", "--"},
		Env:            []string{"GO_WANT_CURSOR_BRIDGE_HELPER=1"},
		StartupTimeout: 2 * time.Second,
		CallTimeout:    2 * time.Second,
	})
	if err != nil {
		t.Fatalf("start bridge adapter: %v", err)
	}
	defer func() {
		_ = adapter.Close()
	}()

	if adapter.Name() != "helper-cursor-bridge" {
		t.Fatalf("unexpected adapter name: %s", adapter.Name())
	}

	caps := adapter.Capabilities()
	if !caps.SupportsStructuredPatch || !caps.SupportsOpenLocation {
		t.Fatalf("unexpected capabilities: %+v", caps)
	}

	ctx, err := adapter.GetContext(context.Background(), ContextRequest{Options: protocol.ContextOptions{
		IncludeActiveFile:       true,
		IncludeSelection:        true,
		IncludeLatestError:      true,
		IncludeWorkspaceSummary: true,
	}})
	if err != nil {
		t.Fatalf("get context: %v", err)
	}
	if ctx.ActiveFilePath != "src/auth/middleware.ts" {
		t.Fatalf("unexpected active file: %s", ctx.ActiveFilePath)
	}

	task, err := adapter.SubmitTask(context.Background(), SubmitTaskInput{
		Prompt:  "Fix auth",
		Context: WorkspaceContext{},
	})
	if err != nil {
		t.Fatalf("submit task: %v", err)
	}
	if task.TaskID != "task_helper_1" {
		t.Fatalf("unexpected task id: %s", task.TaskID)
	}

	patch, err := adapter.GetPatch(context.Background(), task.TaskID)
	if err != nil {
		t.Fatalf("get patch: %v", err)
	}
	if patch == nil || len(patch.Files) != 1 {
		t.Fatalf("unexpected patch payload: %+v", patch)
	}

	applyResult, err := adapter.ApplyPatch(context.Background(), ApplyPatchInput{
		TaskID: task.TaskID,
		Mode:   "all",
	})
	if err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if applyResult.Status != "success" {
		t.Fatalf("unexpected apply result: %+v", applyResult)
	}

	runHandle, err := adapter.RunProfile(context.Background(), RunProfileInput{
		TaskID:    task.TaskID,
		JobID:     "job_1",
		ProfileID: "test_all",
		Command:   "go test ./...",
	})
	if err != nil {
		t.Fatalf("run profile: %v", err)
	}
	if runHandle.RunID != "run_helper_1" {
		t.Fatalf("unexpected run id: %s", runHandle.RunID)
	}

	runResult, err := adapter.GetRunResult(context.Background(), runHandle.RunID)
	if err != nil {
		t.Fatalf("get run result: %v", err)
	}
	if runResult == nil || runResult.Status != "failed" {
		t.Fatalf("unexpected run result: %+v", runResult)
	}

	if err := adapter.OpenLocation(context.Background(), OpenLocationInput{
		Path:   "src/auth/middleware.ts",
		Line:   12,
		Column: 3,
	}); err != nil {
		t.Fatalf("open location: %v", err)
	}
}

func TestCursorBridgeHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_CURSOR_BRIDGE_HELPER") != "1" {
		return
	}

	reader := bufio.NewReader(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)
	defer writer.Flush()

	for {
		line, err := reader.ReadBytes('\n')
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) > 0 {
			var request cursorBridgeRequest
			if unmarshalErr := json.Unmarshal(trimmed, &request); unmarshalErr != nil {
				writeHelperResponse(writer, cursorBridgeResponse{
					ID:    request.ID,
					Error: &cursorBridgeError{Message: unmarshalErr.Error()},
				})
				os.Exit(1)
			}

			response := helperResponse(request)
			writeHelperResponse(writer, response)
		}

		if err != nil {
			if err == io.EOF {
				os.Exit(0)
			}
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
	}
}

func helperResponse(request cursorBridgeRequest) cursorBridgeResponse {
	switch request.Method {
	case "name":
		return helperResult(request.ID, "helper-cursor-bridge")
	case "capabilities":
		return helperResult(request.ID, AdapterCapabilities{
			SupportsPartialApply:     true,
			SupportsStructuredPatch:  true,
			SupportsContextSelection: true,
			SupportsArtifacts:        false,
			SupportsOpenLocation:     true,
		})
	case "getContext":
		return helperResult(request.ID, WorkspaceContext{
			ActiveFilePath:      "src/auth/middleware.ts",
			Selection:           "if (!token) return 401",
			LatestTerminalError: "expected 401 got 500",
			ChangedFiles:        []string{"src/auth/middleware.ts", "tests/auth/middleware.test.ts"},
			LastRunProfile:      "test_all",
			LastRunStatus:       "failed",
		})
	case "submitTask":
		return helperResult(request.ID, TaskHandle{TaskID: "task_helper_1"})
	case "getPatch":
		return helperResult(request.ID, &protocol.PatchReadyPayload{
			JobID:   "",
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
		})
	case "applyPatch":
		return helperResult(request.ID, ApplyPatchResult{Status: "success", Message: "patch applied"})
	case "runProfile":
		return helperResult(request.ID, RunHandle{RunID: "run_helper_1"})
	case "getRunResult":
		return helperResult(request.ID, &RunResult{
			RunID:     "run_helper_1",
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
		})
	case "openLocation":
		return helperResult(request.ID, nil)
	default:
		return cursorBridgeResponse{
			ID:    request.ID,
			Error: &cursorBridgeError{Message: "unsupported helper method"},
		}
	}
}

func helperResult(id string, value any) cursorBridgeResponse {
	payload, err := json.Marshal(value)
	if err != nil {
		return cursorBridgeResponse{
			ID:    id,
			Error: &cursorBridgeError{Message: err.Error()},
		}
	}

	return cursorBridgeResponse{
		ID:     id,
		Result: payload,
	}
}

func writeHelperResponse(writer *bufio.Writer, response cursorBridgeResponse) {
	payload, err := json.Marshal(response)
	if err != nil {
		panic(err)
	}
	_, _ = writer.Write(append(payload, '\n'))
	_ = writer.Flush()
}
