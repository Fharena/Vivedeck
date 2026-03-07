import { CursorExtensionBridge } from "./cursorExtensionBridge.js";
import type { CursorBridgeCommands, CursorCommandHost, CursorWorkspaceMetadata } from "./cursorHost.js";
import { MockCursorBridge } from "./mockCursorBridge.js";
import type {
  ApplyPatchInput,
  OpenLocationInput,
  RunProfileInput,
  SubmitTaskInput,
  WorkspaceAdapter,
} from "./types.js";

const fixtureCommands: CursorBridgeCommands = {
  submitTask: "vibedeck.submitTask",
  getPatch: "vibedeck.getPatch",
  applyPatch: "vibedeck.applyPatch",
  runProfile: "vibedeck.runProfile",
  getRunResult: "vibedeck.getRunResult",
  getWorkspaceMetadata: "vibedeck.getWorkspaceMetadata",
  getLatestTerminalError: "vibedeck.getLatestTerminalError",
};

export function createFixtureCursorBridge(): WorkspaceAdapter {
  return new CursorExtensionBridge({
    host: new FixtureCursorCommandHost(),
    commands: fixtureCommands,
  });
}

class FixtureCursorCommandHost implements CursorCommandHost {
  private readonly delegate = new MockCursorBridge();

  async getActiveEditor() {
    return {
      filePath: "src/auth/middleware.ts",
      selection: "if (!token) return 401",
    };
  }

  async getChangedFiles(): Promise<string[]> {
    return ["src/auth/middleware.ts", "tests/auth/middleware.test.ts"];
  }

  async executeCommand<T = unknown>(command: string, ...args: unknown[]): Promise<T> {
    switch (command) {
      case fixtureCommands.submitTask:
        return (await this.delegate.submitTask(args[0] as SubmitTaskInput)) as T;
      case fixtureCommands.getPatch:
        return (await this.delegate.getPatch(args[0] as string)) as T;
      case fixtureCommands.applyPatch:
        return (await this.delegate.applyPatch(args[0] as ApplyPatchInput)) as T;
      case fixtureCommands.runProfile:
        return (await this.delegate.runProfile(args[0] as RunProfileInput)) as T;
      case fixtureCommands.getRunResult:
        return (await this.delegate.getRunResult(args[0] as string)) as T;
      case fixtureCommands.getWorkspaceMetadata:
        return this.workspaceMetadata() as T;
      case fixtureCommands.getLatestTerminalError:
        return "expected 401 got 500" as T;
      default:
        throw new Error(`unsupported fixture command: ${command}`);
    }
  }

  async openLocation(input: OpenLocationInput): Promise<void> {
    await this.delegate.openLocation(input);
  }

  private workspaceMetadata(): CursorWorkspaceMetadata {
    return {
      changedFiles: ["src/auth/middleware.ts", "tests/auth/middleware.test.ts"],
      lastRunProfile: "test_all",
      lastRunStatus: "failed",
      latestTerminalError: "expected 401 got 500",
    };
  }
}