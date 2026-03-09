package agent

import (
	"context"
	"testing"

	"github.com/Fharena/VibeDeck/internal/protocol"
)

func TestOrchestratorPromptSubmitFlow(t *testing.T) {
	orch := NewOrchestrator(NewMockAdapter(), DefaultRunProfiles(), nil)

	env, err := protocol.NewEnvelope("sid-1", "rid-1", 1, protocol.TypePromptSubmit, protocol.PromptSubmitPayload{
		Prompt:   "Fix auth middleware",
		Template: "fix_bug",
		ContextOptions: protocol.ContextOptions{
			IncludeActiveFile: true,
		},
	})
	if err != nil {
		t.Fatalf("build envelope: %v", err)
	}

	responses, err := orch.HandleEnvelope(context.Background(), env)
	if err != nil {
		t.Fatalf("handle envelope: %v", err)
	}

	if len(responses) != 3 {
		t.Fatalf("expected 3 responses, got %d", len(responses))
	}

	if responses[0].Type != protocol.TypeCmdAck {
		t.Fatalf("response[0] should be CMD_ACK")
	}
	if responses[1].Type != protocol.TypePromptAck {
		t.Fatalf("response[1] should be PROMPT_ACK")
	}
	if responses[2].Type != protocol.TypePatchReady {
		t.Fatalf("response[2] should be PATCH_READY")
	}

	var promptAck protocol.PromptAckPayload
	if err := responses[1].DecodePayload(&promptAck); err != nil {
		t.Fatalf("decode prompt ack payload: %v", err)
	}
	if promptAck.JobID == "" {
		t.Fatalf("prompt ack job id should be set")
	}
	if promptAck.ThreadID == "" {
		t.Fatalf("prompt ack thread id should be set")
	}

	detail, ok := orch.ThreadStore().Get(promptAck.ThreadID)
	if !ok {
		t.Fatalf("thread detail should exist")
	}
	if len(detail.Events) != 3 {
		t.Fatalf("expected 3 thread events, got %+v", detail.Events)
	}
	if detail.Events[0].Kind != "prompt_submitted" {
		t.Fatalf("expected first event prompt_submitted, got %+v", detail.Events[0])
	}
	if detail.Events[2].Kind != "patch_ready" {
		t.Fatalf("expected patch_ready event, got %+v", detail.Events[2])
	}
}

func TestOrchestratorPatchApplyUnknownJob(t *testing.T) {
	orch := NewOrchestrator(NewMockAdapter(), DefaultRunProfiles(), nil)

	env, err := protocol.NewEnvelope("sid-1", "rid-apply", 2, protocol.TypePatchApply, protocol.PatchApplyPayload{
		JobID: "missing",
		Mode:  "all",
	})
	if err != nil {
		t.Fatalf("build envelope: %v", err)
	}

	responses, err := orch.HandleEnvelope(context.Background(), env)
	if err == nil {
		t.Fatalf("expected error for unknown job")
	}
	if len(responses) != 1 || responses[0].Type != protocol.TypeCmdAck {
		t.Fatalf("expected single CMD_ACK failure")
	}
}

func TestOrchestratorRunProfileFlow(t *testing.T) {
	orch := NewOrchestrator(NewMockAdapter(), DefaultRunProfiles(), nil)

	submit, _ := protocol.NewEnvelope("sid-1", "rid-submit", 1, protocol.TypePromptSubmit, protocol.PromptSubmitPayload{
		Prompt: "Fix test",
	})

	submitResponses, err := orch.HandleEnvelope(context.Background(), submit)
	if err != nil {
		t.Fatalf("submit flow failed: %v", err)
	}

	var promptAck protocol.PromptAckPayload
	if err := submitResponses[1].DecodePayload(&promptAck); err != nil {
		t.Fatalf("decode prompt ack: %v", err)
	}

	runEnv, _ := protocol.NewEnvelope("sid-1", "rid-run", 2, protocol.TypeRunProfile, protocol.RunProfilePayload{
		JobID:     promptAck.JobID,
		ProfileID: "test_all",
	})

	runResponses, err := orch.HandleEnvelope(context.Background(), runEnv)
	if err != nil {
		t.Fatalf("run flow failed: %v", err)
	}

	if len(runResponses) != 2 {
		t.Fatalf("expected 2 responses for run profile, got %d", len(runResponses))
	}

	if runResponses[1].Type != protocol.TypeRunResult {
		t.Fatalf("second response should be RUN_RESULT")
	}

	var runResult protocol.RunResultPayload
	if err := runResponses[1].DecodePayload(&runResult); err != nil {
		t.Fatalf("decode run result: %v", err)
	}
	if len(runResult.ChangedFiles) == 0 {
		t.Fatalf("expected run result changed files, got %+v", runResult)
	}

	detail, ok := orch.ThreadStore().Get(promptAck.ThreadID)
	if !ok {
		t.Fatalf("thread detail should exist")
	}
	if detail.Thread.State != "failed" {
		t.Fatalf("expected thread state failed after mock run result, got %+v", detail.Thread)
	}
	if detail.Events[len(detail.Events)-1].Kind != "run_finished" {
		t.Fatalf("expected last event run_finished, got %+v", detail.Events[len(detail.Events)-1])
	}
}
