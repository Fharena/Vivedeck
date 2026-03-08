package agent

import (
	"testing"
)

func TestPersistentThreadStoreRestoresThreadsAndJobMapping(t *testing.T) {
	tempDir := t.TempDir()
	store, err := NewPersistentThreadStore(tempDir + "/thread-store.json")
	if err != nil {
		t.Fatalf("create persistent thread store: %v", err)
	}

	summary := store.EnsureThread("thread_restore", "sid_restore", "Fix auth middleware")
	if summary.ID != "thread_restore" {
		t.Fatalf("unexpected thread id: %+v", summary)
	}
	store.AssignJob(summary.ID, "job_restore")
	if _, err := store.AppendEvent(summary.ID, ThreadEvent{
		JobID: "job_restore",
		Kind:  "prompt_submitted",
		Role:  "user",
		Body:  "Fix auth middleware",
		Data: map[string]any{
			"template": "fix_bug",
		},
	}); err != nil {
		t.Fatalf("append prompt event: %v", err)
	}
	if _, err := store.AppendEvent(summary.ID, ThreadEvent{
		JobID: "job_restore",
		Kind:  "run_finished",
		Role:  "system",
		Body:  "tests passed",
		Data: map[string]any{
			"status":  "passed",
			"summary": "tests passed",
		},
	}); err != nil {
		t.Fatalf("append run event: %v", err)
	}

	restored, err := NewPersistentThreadStore(tempDir + "/thread-store.json")
	if err != nil {
		t.Fatalf("restore thread store: %v", err)
	}

	threads := restored.List()
	if len(threads) != 1 {
		t.Fatalf("expected 1 restored thread, got %+v", threads)
	}
	if threads[0].ID != summary.ID || threads[0].State != "passed" {
		t.Fatalf("unexpected restored summary: %+v", threads[0])
	}
	if threadID, ok := restored.ThreadIDForJob("job_restore"); !ok || threadID != summary.ID {
		t.Fatalf("expected job_restore to map to %s, got %q ok=%v", summary.ID, threadID, ok)
	}

	detail, ok := restored.Get(summary.ID)
	if !ok {
		t.Fatalf("restored detail should exist")
	}
	if len(detail.Events) != 2 {
		t.Fatalf("expected 2 restored events, got %+v", detail.Events)
	}
	if detail.Events[1].Kind != "run_finished" {
		t.Fatalf("expected run_finished event, got %+v", detail.Events[1])
	}
}
