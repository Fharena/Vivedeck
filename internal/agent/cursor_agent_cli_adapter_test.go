package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Fharena/Vivedeck/internal/protocol"
)

func TestCursorAgentCLIHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	path := filepath.Join(cwd, filepath.FromSlash(os.Getenv("VIBEDECK_TEST_TARGET_FILE")))
	mode := os.Getenv("VIBEDECK_TEST_CURSOR_AGENT_MODE")

	switch mode {
	case "append_line":
		content, err := os.ReadFile(path)
		if err != nil {
			panic(err)
		}
		updated := strings.TrimRight(string(content), "\n") + "\nagent-change\n"
		if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
			panic(err)
		}
	case "replace_two_lines":
		content, err := os.ReadFile(path)
		if err != nil {
			panic(err)
		}
		lines := strings.Split(strings.TrimRight(string(content), "\n"), "\n")
		lines[1] = "line-02 updated by agent"
		lines[17] = "line-18 updated by agent"
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			panic(err)
		}
	case "append_required_file":
		requiredRelativePath := os.Getenv("VIBEDECK_TEST_REQUIRED_FILE")
		requiredPath := filepath.Join(cwd, filepath.FromSlash(requiredRelativePath))
		requiredContent, err := os.ReadFile(requiredPath)
		if err != nil {
			panic("required snapshot file missing: " + requiredRelativePath)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			panic(err)
		}
		updated := strings.TrimRight(string(content), "\n") + "\n" + strings.TrimSpace(string(requiredContent)) + "\n"
		if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
			panic(err)
		}
	case "create_new_file":
		newRelativePath := os.Getenv("VIBEDECK_TEST_NEW_FILE")
		if strings.TrimSpace(newRelativePath) == "" {
			panic("missing VIBEDECK_TEST_NEW_FILE")
		}
		newPath := filepath.Join(cwd, filepath.FromSlash(newRelativePath))
		if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
			panic(err)
		}
		if err := os.WriteFile(newPath, []byte("print('hello world')\n"), 0o644); err != nil {
			panic(err)
		}
	default:
		panic("unknown helper mode: " + mode)
	}

	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
		"type":   "result",
		"result": "Applied requested changes in temp worktree",
	})
	os.Exit(0)
}

func TestCursorAgentCLIAdapterSubmitTaskUsesWorkspaceSnapshot(t *testing.T) {
	repo := newCursorAgentTestRepo(t, map[string]string{
		"src/app.txt": "alpha\nbeta\ngamma\n",
	})
	writeTestFile(t, filepath.Join(repo, "src", "app.txt"), "alpha\nbeta\ngamma\nuser-local\n")

	adapter := newTestCursorAgentCLIAdapter(t, repo, "append_line", "src/app.txt")
	handle, err := adapter.SubmitTask(context.Background(), SubmitTaskInput{
		Prompt:   "append one agent line",
		Template: "fix_bug",
		Context: WorkspaceContext{
			ChangedFiles:        []string{"src/app.txt"},
			LatestTerminalError: "expected 401 got 500",
		},
	})
	if err != nil {
		t.Fatalf("submit task: %v", err)
	}

	patch, err := adapter.GetPatch(context.Background(), handle.TaskID)
	if err != nil {
		t.Fatalf("get patch: %v", err)
	}
	if patch == nil || len(patch.Files) != 1 {
		t.Fatalf("unexpected patch payload: %+v", patch)
	}
	if len(patch.Files[0].Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(patch.Files[0].Hunks))
	}
	if strings.Contains(patch.Files[0].Hunks[0].Diff, "+user-local") || strings.Contains(patch.Files[0].Hunks[0].Diff, "-user-local") {
		t.Fatalf("patch should not re-propose existing workspace change: %s", patch.Files[0].Hunks[0].Diff)
	}
	if !strings.Contains(patch.Files[0].Hunks[0].Diff, "agent-change") {
		t.Fatalf("patch should include agent change: %s", patch.Files[0].Hunks[0].Diff)
	}

	applyResult, err := adapter.ApplyPatch(context.Background(), ApplyPatchInput{
		TaskID: handle.TaskID,
		Mode:   "all",
	})
	if err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if applyResult.Status != "success" {
		t.Fatalf("expected apply success, got %+v", applyResult)
	}

	content := readTestFile(t, filepath.Join(repo, "src", "app.txt"))
	if !strings.Contains(content, "user-local") {
		t.Fatalf("workspace change should remain after apply: %s", content)
	}
	if !strings.Contains(content, "agent-change") {
		t.Fatalf("agent change should be applied: %s", content)
	}
}

func TestCursorAgentCLIAdapterApplySelectedHunks(t *testing.T) {
	repo := newCursorAgentTestRepo(t, map[string]string{
		"src/multi.txt": strings.Join([]string{
			"line-01",
			"line-02",
			"line-03",
			"line-04",
			"line-05",
			"line-06",
			"line-07",
			"line-08",
			"line-09",
			"line-10",
			"line-11",
			"line-12",
			"line-13",
			"line-14",
			"line-15",
			"line-16",
			"line-17",
			"line-18",
			"line-19",
			"line-20",
		}, "\n") + "\n",
	})

	adapter := newTestCursorAgentCLIAdapter(t, repo, "replace_two_lines", "src/multi.txt")
	handle, err := adapter.SubmitTask(context.Background(), SubmitTaskInput{Prompt: "update two distant lines"})
	if err != nil {
		t.Fatalf("submit task: %v", err)
	}

	patch, err := adapter.GetPatch(context.Background(), handle.TaskID)
	if err != nil {
		t.Fatalf("get patch: %v", err)
	}
	if len(patch.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(patch.Files))
	}
	if len(patch.Files[0].Hunks) != 2 {
		t.Fatalf("expected 2 hunks, got %d", len(patch.Files[0].Hunks))
	}

	applyResult, err := adapter.ApplyPatch(context.Background(), ApplyPatchInput{
		TaskID: handle.TaskID,
		Mode:   "selected",
		Selected: []protocol.SelectedHunk{{
			Path:    patch.Files[0].Path,
			HunkIDs: []string{patch.Files[0].Hunks[0].HunkID},
		}},
	})
	if err != nil {
		t.Fatalf("apply selected patch: %v", err)
	}
	if applyResult.Status != "success" {
		t.Fatalf("expected apply success, got %+v", applyResult)
	}

	content := readTestFile(t, filepath.Join(repo, "src", "multi.txt"))
	if !strings.Contains(content, "line-02 updated by agent") {
		t.Fatalf("first selected hunk should be applied: %s", content)
	}
	if strings.Contains(content, "line-18 updated by agent") {
		t.Fatalf("unselected hunk should not be applied: %s", content)
	}
}

func TestCursorAgentCLIAdapterRunProfileAndContext(t *testing.T) {
	repo := newCursorAgentTestRepo(t, map[string]string{
		"README.md": "hello\n",
	})
	adapter := newTestCursorAgentCLIAdapter(t, repo, "append_line", "README.md")

	handle, err := adapter.RunProfile(context.Background(), RunProfileInput{
		ProfileID: "test_all",
		Command:   successCommand(),
	})
	if err != nil {
		t.Fatalf("run profile: %v", err)
	}

	result, err := adapter.GetRunResult(context.Background(), handle.RunID)
	if err != nil {
		t.Fatalf("get run result: %v", err)
	}
	if result.Status != "passed" {
		t.Fatalf("expected passed status, got %+v", result)
	}

	ctx, err := adapter.GetContext(context.Background(), ContextRequest{Options: protocol.ContextOptions{
		IncludeLatestError:      true,
		IncludeWorkspaceSummary: true,
	}})
	if err != nil {
		t.Fatalf("get context: %v", err)
	}
	if ctx.LastRunProfile != "test_all" {
		t.Fatalf("expected last run profile test_all, got %s", ctx.LastRunProfile)
	}
	if ctx.LastRunStatus != "passed" {
		t.Fatalf("expected last run status passed, got %s", ctx.LastRunStatus)
	}
	if ctx.LatestTerminalError != "" {
		t.Fatalf("expected empty latest terminal error, got %q", ctx.LatestTerminalError)
	}
}

func TestCursorAgentCLIAdapterDoesNotSyncIgnoredFilesByDefault(t *testing.T) {
	repo := newCursorAgentTestRepo(t, map[string]string{
		".gitignore":  ".env.local\n",
		"src/app.txt": "alpha\n",
	})
	writeTestFile(t, filepath.Join(repo, ".env.local"), "ignored-secret\n")

	adapter := newTestCursorAgentCLIAdapterWithOptions(
		t,
		repo,
		"append_required_file",
		"src/app.txt",
		nil,
		[]string{"VIBEDECK_TEST_REQUIRED_FILE=.env.local"},
	)

	_, err := adapter.SubmitTask(context.Background(), SubmitTaskInput{Prompt: "use ignored file"})
	if err == nil {
		t.Fatal("expected submit task to fail when ignored file is not synced")
	}
	if !strings.Contains(err.Error(), "required snapshot file missing") {
		t.Fatalf("expected missing ignored file error, got %v", err)
	}
}

func TestCursorAgentCLIAdapterSyncsExplicitIgnoredFiles(t *testing.T) {
	repo := newCursorAgentTestRepo(t, map[string]string{
		".gitignore":  ".env.local\n",
		"src/app.txt": "alpha\n",
	})
	writeTestFile(t, filepath.Join(repo, ".env.local"), "ignored-secret\n")

	adapter := newTestCursorAgentCLIAdapterWithOptions(
		t,
		repo,
		"append_required_file",
		"src/app.txt",
		[]string{".env.local"},
		[]string{"VIBEDECK_TEST_REQUIRED_FILE=.env.local"},
	)

	handle, err := adapter.SubmitTask(context.Background(), SubmitTaskInput{Prompt: "use ignored file"})
	if err != nil {
		t.Fatalf("submit task: %v", err)
	}

	patch, err := adapter.GetPatch(context.Background(), handle.TaskID)
	if err != nil {
		t.Fatalf("get patch: %v", err)
	}
	if patch == nil || len(patch.Files) != 1 {
		t.Fatalf("unexpected patch payload: %+v", patch)
	}
	if !strings.Contains(patch.Files[0].Hunks[0].Diff, "ignored-secret") {
		t.Fatalf("patch should include ignored file content: %s", patch.Files[0].Hunks[0].Diff)
	}

	applyResult, err := adapter.ApplyPatch(context.Background(), ApplyPatchInput{
		TaskID: handle.TaskID,
		Mode:   "all",
	})
	if err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if applyResult.Status != "success" {
		t.Fatalf("expected apply success, got %+v", applyResult)
	}

	content := readTestFile(t, filepath.Join(repo, "src", "app.txt"))
	if !strings.Contains(content, "ignored-secret") {
		t.Fatalf("ignored file content should be reflected after apply: %s", content)
	}
}

func TestCursorAgentCLIAdapterCapturesNewFileDiff(t *testing.T) {
	repo := newCursorAgentTestRepo(t, map[string]string{
		"README.md": "workspace\n",
	})

	adapter := newTestCursorAgentCLIAdapterWithOptions(
		t,
		repo,
		"create_new_file",
		"README.md",
		nil,
		[]string{"VIBEDECK_TEST_NEW_FILE=src/hello.py"},
	)

	handle, err := adapter.SubmitTask(context.Background(), SubmitTaskInput{
		Prompt: "create src/hello.py that prints hello world",
	})
	if err != nil {
		t.Fatalf("submit task: %v", err)
	}

	patch, err := adapter.GetPatch(context.Background(), handle.TaskID)
	if err != nil {
		t.Fatalf("get patch: %v", err)
	}
	if patch == nil || len(patch.Files) != 1 {
		t.Fatalf("unexpected patch payload: %+v", patch)
	}
	if patch.Files[0].Path != "src/hello.py" {
		t.Fatalf("expected new file path src/hello.py, got %+v", patch.Files[0])
	}
	if patch.Files[0].Status != "added" {
		t.Fatalf("expected added status, got %+v", patch.Files[0])
	}
	if len(patch.Files[0].Hunks) != 1 {
		t.Fatalf("expected 1 hunk for new file, got %+v", patch.Files[0])
	}
	if !strings.Contains(patch.Files[0].Hunks[0].Diff, "print('hello world')") {
		t.Fatalf("expected new file contents in diff, got %s", patch.Files[0].Hunks[0].Diff)
	}

	applyResult, err := adapter.ApplyPatch(context.Background(), ApplyPatchInput{
		TaskID: handle.TaskID,
		Mode:   "all",
	})
	if err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if applyResult.Status != "success" {
		t.Fatalf("expected apply success, got %+v", applyResult)
	}

	content := readTestFile(t, filepath.Join(repo, "src", "hello.py"))
	if strings.TrimSpace(content) != "print('hello world')" {
		t.Fatalf("expected created file contents, got %q", content)
	}
}

func TestBuildWSLCursorAgentArgs(t *testing.T) {
	args := buildWSLCursorAgentArgs("Ubuntu", "/home/test/.local/bin/cursor-agent", []string{"--print", "--output-format", "json"})
	want := []string{
		"-d",
		"Ubuntu",
		"--",
		"/home/test/.local/bin/cursor-agent",
		"--print",
		"--output-format",
		"json",
	}
	if len(args) != len(want) {
		t.Fatalf("expected %d args, got %d (%v)", len(want), len(args), args)
	}
	for i, expected := range want {
		if args[i] != expected {
			t.Fatalf("arg %d: want %q, got %q", i, expected, args[i])
		}
	}
}
func TestEnsureCursorAgentTrustFlag(t *testing.T) {
	args := ensureCursorAgentTrustFlag([]string{"--print", "--output-format", "json"}, true)
	want := []string{"--print", "--output-format", "json", "--trust"}
	if len(args) != len(want) {
		t.Fatalf("expected %d args, got %d (%v)", len(want), len(args), args)
	}
	for i, expected := range want {
		if args[i] != expected {
			t.Fatalf("arg %d: want %q, got %q", i, expected, args[i])
		}
	}

	alreadyTrusted := ensureCursorAgentTrustFlag([]string{"--print", "--trust"}, true)
	if len(alreadyTrusted) != 2 {
		t.Fatalf("expected duplicate trust to be avoided: %v", alreadyTrusted)
	}

	disabled := ensureCursorAgentTrustFlag([]string{"--print"}, false)
	if len(disabled) != 1 || disabled[0] != "--print" {
		t.Fatalf("expected disabled trust to preserve args: %v", disabled)
	}
}
func TestEnsureCursorAgentModelArg(t *testing.T) {
	args := ensureCursorAgentModelArg([]string{"--print", "--output-format", "json"}, "auto")
	want := []string{"--print", "--output-format", "json", "--model", "auto"}
	if len(args) != len(want) {
		t.Fatalf("expected %d args, got %d (%v)", len(want), len(args), args)
	}
	for i, expected := range want {
		if args[i] != expected {
			t.Fatalf("arg %d: want %q, got %q", i, expected, args[i])
		}
	}

	alreadyModel := ensureCursorAgentModelArg([]string{"--print", "--model", "opus-4.6-thinking"}, "auto")
	if len(alreadyModel) != 3 {
		t.Fatalf("expected existing model to be preserved: %v", alreadyModel)
	}

	disabled := ensureCursorAgentModelArg([]string{"--print"}, "")
	if len(disabled) != 1 || disabled[0] != "--print" {
		t.Fatalf("expected empty model to preserve args: %v", disabled)
	}
}
func newTestCursorAgentCLIAdapter(t *testing.T, repo, mode, targetFile string) *CursorAgentCLIAdapter {
	return newTestCursorAgentCLIAdapterWithOptions(t, repo, mode, targetFile, nil, nil)
}

func newTestCursorAgentCLIAdapterWithOptions(
	t *testing.T,
	repo, mode, targetFile string,
	syncIgnoredPaths []string,
	extraEnv []string,
) *CursorAgentCLIAdapter {
	t.Helper()
	cursorAgentEnv := []string{
		"GO_WANT_HELPER_PROCESS=1",
		"VIBEDECK_TEST_CURSOR_AGENT_MODE=" + mode,
		"VIBEDECK_TEST_TARGET_FILE=" + targetFile,
	}
	cursorAgentEnv = append(cursorAgentEnv, extraEnv...)
	cfg := CursorAgentCLIConfig{
		WorkspaceRoot:    repo,
		GitBin:           "git",
		CursorAgentBin:   os.Args[0],
		CursorAgentArgs:  []string{"-test.run=TestCursorAgentCLIHelperProcess", "--"},
		CursorAgentEnv:   cursorAgentEnv,
		SyncIgnoredPaths: syncIgnoredPaths,
		PromptTimeout:    30 * time.Second,
		RunTimeout:       30 * time.Second,
	}
	adapter, err := NewCursorAgentCLIAdapter(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new cursor agent cli adapter: %v", err)
	}
	return adapter
}

func newCursorAgentTestRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	repo := t.TempDir()
	runGitCommand(t, repo, "init")
	runGitCommand(t, repo, "config", "user.name", "VibeDeck Test")
	runGitCommand(t, repo, "config", "user.email", "vibedeck-test@example.local")
	for path, content := range files {
		writeTestFile(t, filepath.Join(repo, filepath.FromSlash(path)), content)
	}
	runGitCommand(t, repo, "add", "-A")
	runGitCommand(t, repo, "commit", "-m", "base")
	return repo
}

func runGitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	stdout, stderr, err := runCommand(context.Background(), dir, nil, nil, "git", args...)
	if err != nil {
		t.Fatalf("git %s failed: %v stdout=%s stderr=%s", strings.Join(args, " "), err, stdout, stderr)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

func successCommand() string {
	if runtime.GOOS == "windows" {
		return "echo vibedeck-run"
	}
	return "printf vibedeck-run"
}
