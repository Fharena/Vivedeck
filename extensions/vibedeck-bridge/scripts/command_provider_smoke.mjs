import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import {
  createCursorExtensionBridge,
  createCursorExtensionRuntime,
  createVSCodeCursorHost,
} from "@vibedeck/cursor-bridge";
import { createCursorAgentCommandAdapter } from "../dist/cursorAgentCommandAdapter.js";

const tempRoot = await mkdtemp(path.join(os.tmpdir(), "vibedeck-command-provider-"));

try {
  const workspaceRoot = path.join(tempRoot, "workspace");
  await seedGitWorkspace(workspaceRoot);

  const fakeAgentPath = path.join(tempRoot, "fake-cursor-agent.mjs");
  await writeFile(
    fakeAgentPath,
    [
      'import { readFileSync, writeFileSync } from "node:fs";',
      'import path from "node:path";',
      'const secret = readFileSync(path.join(process.cwd(), ".env.local"), "utf8").trim();',
      'writeFileSync(path.join(process.cwd(), "notes.txt"), `base\\n${secret}\\n`, "utf8");',
      'process.stdout.write(JSON.stringify({ summary: "updated notes.txt" }));',
    ].join("\n"),
    "utf8",
  );

  const adapter = await createCursorAgentCommandAdapter({
    workspaceRoot,
    tempRoot,
    gitBin: "git",
    cursorAgentBin: process.execPath,
    cursorAgentArgs: [fakeAgentPath],
    cursorAgentEnv: [],
    syncIgnoredPaths: [".env.local"],
    useWsl: false,
    promptTimeoutMs: 30000,
    runTimeoutMs: 30000,
  });

  const registry = new Map();
  const document = {
    fileName: path.join(workspaceRoot, "notes.txt"),
    isDirty: true,
    getText() {
      return "base";
    },
  };

  const fakeVscode = {
    commands: {
      async executeCommand(command, ...args) {
        const handler = registry.get(command);
        if (!handler) {
          throw new Error("missing command: " + command);
        }
        return await handler(...args);
      },
      registerCommand(command, callback) {
        registry.set(command, callback);
        return {
          dispose() {
            registry.delete(command);
          },
        };
      },
    },
    window: {
      activeTextEditor: {
        document,
        selection: null,
      },
      async showTextDocument() {
        return undefined;
      },
    },
    workspace: {
      textDocuments: [document],
      async openTextDocument(openPath) {
        return {
          fileName: openPath,
          getText() {
            return "";
          },
        };
      },
    },
  };

  const runtime = createCursorExtensionRuntime({
    vscode: fakeVscode,
    adapter,
  });

  try {
    const bridge = createCursorExtensionBridge({
      host: createVSCodeCursorHost(fakeVscode),
    });

    const task = await bridge.submitTask({
      prompt: "Update notes.txt for smoke",
      template: "smoke",
      context: {},
    });
    const patch = await bridge.getPatch(task.taskId);
    assert.ok(patch, "patch should be returned");
    assert.equal(patch.files.length, 1);
    assert.equal(patch.files[0].path, "notes.txt");
    assert.match(patch.files[0].hunks[0].diff, /ignored-secret/);

    const apply = await bridge.applyPatch({
      taskId: task.taskId,
      mode: "all",
    });
    assert.equal(apply.status, "success");

    const run = await bridge.runProfile({
      taskId: task.taskId,
      jobId: "job_smoke",
      profileId: "smoke",
      command: "git diff -- notes.txt",
    });
    const runResult = await bridge.getRunResult(run.runId);
    assert.ok(runResult, "run result should be returned");
    assert.equal(runResult.status, "passed");

    const notesContent = await readFile(path.join(workspaceRoot, "notes.txt"), "utf8");
    assert.equal(notesContent.trim(), "base\nignored-secret");

    const context = await bridge.getContext({
      options: {
        includeActiveFile: false,
        includeSelection: false,
        includeLatestError: true,
        includeWorkspaceSummary: true,
      },
    });
    assert.equal(context.lastRunProfile, "smoke");
    assert.equal(context.lastRunStatus, "passed");

    console.log(
      JSON.stringify(
        {
          taskId: task.taskId,
          patchSummary: patch.summary,
          patchFiles: patch.files.map((file) => file.path),
          applyStatus: apply.status,
          runStatus: runResult.status,
          lastRunProfile: context.lastRunProfile,
          workspaceRoot,
        },
        null,
        2,
      ),
    );
  } finally {
    runtime.dispose();
  }
} finally {
  await rm(tempRoot, { recursive: true, force: true });
}

async function seedGitWorkspace(workspaceRoot) {
  await mkdir(workspaceRoot, { recursive: true });
  runGit(workspaceRoot, ["init"]);
  runGit(workspaceRoot, ["config", "user.name", "VibeDeck Smoke"]);
  runGit(workspaceRoot, ["config", "user.email", "vibedeck-smoke@example.local"]);
  await writeFile(path.join(workspaceRoot, ".gitignore"), ".env.local\n", "utf8");
  await writeFile(path.join(workspaceRoot, "notes.txt"), "base\n", "utf8");
  await writeFile(path.join(workspaceRoot, ".env.local"), "ignored-secret\n", "utf8");
  runGit(workspaceRoot, ["add", "-A"]);
  runGit(workspaceRoot, ["commit", "-m", "base"]);
}

function runGit(cwd, args) {
  const result = spawnSync("git", args, {
    cwd,
    encoding: "utf8",
    windowsHide: true,
  });
  if (result.error) {
    throw result.error;
  }
  if (result.status !== 0) {
    throw new Error(
      ["git " + args.join(" ") + " failed", result.stdout, result.stderr]
        .filter(Boolean)
        .join("\n") || "git failed",
    );
  }
}
