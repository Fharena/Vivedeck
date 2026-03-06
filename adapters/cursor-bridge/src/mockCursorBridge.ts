import {
  AdapterCapabilities,
  ApplyPatchInput,
  ApplyPatchResult,
  ContextRequest,
  OpenLocationInput,
  PatchBundle,
  RunHandle,
  RunProfileInput,
  RunResult,
  SubmitTaskInput,
  TaskHandle,
  WorkspaceAdapter,
  WorkspaceContext,
} from "./types.js";

export class MockCursorBridge implements WorkspaceAdapter {
  private taskSeq = 0;
  private runSeq = 0;
  private readonly patches = new Map<string, PatchBundle>();

  name(): string {
    return "cursor-mock-bridge";
  }

  capabilities(): AdapterCapabilities {
    return {
      supportsPartialApply: true,
      supportsStructuredPatch: true,
      supportsContextSelection: true,
      supportsArtifacts: false,
      supportsOpenLocation: true,
    };
  }

  async getContext(_input: ContextRequest): Promise<WorkspaceContext> {
    return {
      activeFilePath: "src/auth/middleware.ts",
      selection: "if (!token) return 401",
      latestTerminalError: "expected 401 got 500",
      changedFiles: ["src/auth/middleware.ts", "tests/auth/middleware.test.ts"],
      lastRunProfile: "test_all",
      lastRunStatus: "failed",
    };
  }

  async submitTask(input: SubmitTaskInput): Promise<TaskHandle> {
    this.taskSeq += 1;
    const taskId = `task_${this.taskSeq}`;

    this.patches.set(taskId, {
      jobId: "",
      summary: `Mock patch for: ${input.prompt}`,
      files: [
        {
          path: "src/auth/middleware.ts",
          status: "modified",
          hunks: [
            {
              hunkId: "h1",
              header: "@@ -12,7 +12,9 @@",
              diff: "- if (!token) throw new Error()\n+ if (!token) return res.status(401).send()",
              risk: "low",
            },
          ],
        },
      ],
    });

    return { taskId };
  }

  async getPatch(taskId: string): Promise<PatchBundle | null> {
    return this.patches.get(taskId) ?? null;
  }

  async applyPatch(input: ApplyPatchInput): Promise<ApplyPatchResult> {
    if (input.mode === "selected" && (!input.selected || input.selected.length === 0)) {
      return { status: "failed", message: "selected mode requires at least one hunk" };
    }

    if (input.mode === "selected") {
      return { status: "partial", message: "selected hunks applied" };
    }

    return { status: "success", message: "patch applied" };
  }

  async runProfile(input: RunProfileInput): Promise<RunHandle> {
    this.runSeq += 1;
    return { runId: `run_${input.profileId}_${this.runSeq}` };
  }

  async getRunResult(runId: string): Promise<RunResult | null> {
    return {
      runId,
      profileId: "test_all",
      status: "failed",
      summary: "1 failing test in auth middleware",
      topErrors: [
        {
          message: "expected 401 got 500",
          path: "tests/auth/middleware.test.ts",
          line: 44,
          column: 13,
        },
      ],
      excerpt: "AssertionError: expected 401 got 500",
    };
  }

  async openLocation(_input: OpenLocationInput): Promise<void> {
    return;
  }
}
