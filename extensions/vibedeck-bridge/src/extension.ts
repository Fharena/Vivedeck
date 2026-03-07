import * as vscode from "vscode";
import {
  MockCursorBridge,
  createCursorExtensionBridge,
  createCursorExtensionRuntime,
  createVSCodeCursorHost,
  serveSocketBridge,
  type CursorBridgeCommands,
  type CursorExtensionRuntime,
  type SocketBridgeServer,
  type VSCodeExtensionLike,
  type VSCodeLike,
} from "@vibedeck/cursor-bridge";

type BridgeMode = "command" | "mock";

interface BridgeSettings {
  autoStart: boolean;
  mode: BridgeMode;
  tcpHost: string;
  tcpPort: number;
  commands: Partial<CursorBridgeCommands>;
}

interface ActiveBridge {
  server: SocketBridgeServer;
  runtime?: CursorExtensionRuntime;
  mode: BridgeMode;
  address: string;
}

let activeBridge: ActiveBridge | undefined;
let statusBarItem: vscode.StatusBarItem | undefined;

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  statusBarItem = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 100);
  statusBarItem.command = "vibedeckBridge.showStatus";
  context.subscriptions.push(statusBarItem);

  context.subscriptions.push(
    vscode.commands.registerCommand("vibedeckBridge.startServer", async () => {
      await startServer(true);
    }),
  );
  context.subscriptions.push(
    vscode.commands.registerCommand("vibedeckBridge.stopServer", async () => {
      await stopServer(true);
    }),
  );
  context.subscriptions.push(
    vscode.commands.registerCommand("vibedeckBridge.showStatus", async () => {
      void vscode.window.showInformationMessage(currentStatusMessage());
    }),
  );
  context.subscriptions.push(
    vscode.workspace.onDidChangeConfiguration((event) => {
      if (!event.affectsConfiguration("vibedeckBridge")) {
        return;
      }
      void restartServer();
    }),
  );

  updateStatusBar();

  if (readSettings().autoStart) {
    await startServer(false);
  }
}

export async function deactivate(): Promise<void> {
  await stopServer(false);
}

async function startServer(showMessage: boolean): Promise<void> {
  await stopServer(false);

  const settings = readSettings();
  try {
    let startedBridge: ActiveBridge;

    if (settings.mode === "mock") {
      const runtime = createCursorExtensionRuntime({
        vscode: asExtensionRuntimeVSCode(),
        adapter: new MockCursorBridge(),
      });
      const server = await serveSocketBridge(runtime.bridge, {
        host: settings.tcpHost,
        port: settings.tcpPort,
      });
      startedBridge = {
        server,
        runtime,
        mode: settings.mode,
        address: server.address,
      };
    } else {
      const bridge = createCursorExtensionBridge({
        host: createVSCodeCursorHost(asHostVSCode()),
        commands: settings.commands,
      });
      const server = await serveSocketBridge(bridge, {
        host: settings.tcpHost,
        port: settings.tcpPort,
      });
      startedBridge = {
        server,
        mode: settings.mode,
        address: server.address,
      };
    }

    activeBridge = startedBridge;
    updateStatusBar();
    if (showMessage) {
      void vscode.window.showInformationMessage(
        `VibeDeck bridge started on ${startedBridge.address} (${startedBridge.mode})`,
      );
    }
  } catch (error) {
    activeBridge = undefined;
    updateStatusBar();
    const message = error instanceof Error ? error.message : String(error);
    void vscode.window.showErrorMessage(`VibeDeck bridge start failed: ${message}`);
  }
}

async function stopServer(showMessage: boolean): Promise<void> {
  const bridge = activeBridge;
  activeBridge = undefined;

  if (bridge?.runtime) {
    bridge.runtime.dispose();
  }
  if (bridge?.server) {
    await bridge.server.close();
  }

  updateStatusBar();
  if (showMessage && bridge) {
    void vscode.window.showInformationMessage("VibeDeck bridge stopped");
  }
}

async function restartServer(): Promise<void> {
  if (!readSettings().autoStart) {
    await stopServer(false);
    return;
  }
  await startServer(false);
}

function updateStatusBar(): void {
  if (!statusBarItem) {
    return;
  }

  if (!activeBridge) {
    statusBarItem.text = "VibeDeck: stopped";
    statusBarItem.tooltip = "VibeDeck localhost bridge is stopped";
    statusBarItem.show();
    return;
  }

  statusBarItem.text = `VibeDeck: ${activeBridge.address}`;
  statusBarItem.tooltip = `mode=${activeBridge.mode}`;
  statusBarItem.show();
}

function currentStatusMessage(): string {
  if (!activeBridge) {
    return "VibeDeck bridge is stopped";
  }
  return `VibeDeck bridge: ${activeBridge.address} (${activeBridge.mode})`;
}

function readSettings(): BridgeSettings {
  const config = vscode.workspace.getConfiguration("vibedeckBridge");
  const modeValue = config.get<string>("mode") === "mock" ? "mock" : "command";
  return {
    autoStart: config.get<boolean>("autoStart", true),
    mode: modeValue,
    tcpHost: config.get<string>("tcpHost", "127.0.0.1").trim() || "127.0.0.1",
    tcpPort: normalizePort(config.get<number>("tcpPort", 7797)),
    commands: readCommandSettings(config),
  };
}

function readCommandSettings(
  config: vscode.WorkspaceConfiguration,
): Partial<CursorBridgeCommands> {
  return {
    submitTask: readOptionalCommand(config, "commands.submitTask"),
    getPatch: readOptionalCommand(config, "commands.getPatch"),
    applyPatch: readOptionalCommand(config, "commands.applyPatch"),
    runProfile: readOptionalCommand(config, "commands.runProfile"),
    getRunResult: readOptionalCommand(config, "commands.getRunResult"),
    openLocation: readOptionalCommand(config, "commands.openLocation"),
    getWorkspaceMetadata: readOptionalCommand(config, "commands.getWorkspaceMetadata"),
    getLatestTerminalError: readOptionalCommand(config, "commands.getLatestTerminalError"),
  };
}

function readOptionalCommand(
  config: vscode.WorkspaceConfiguration,
  key: string,
): string | undefined {
  const value = config.get<string>(key)?.trim();
  if (!value) {
    return undefined;
  }
  return value;
}

function asExtensionRuntimeVSCode(): VSCodeExtensionLike {
  return vscode as unknown as VSCodeExtensionLike;
}

function asHostVSCode(): VSCodeLike {
  return vscode as unknown as VSCodeLike;
}

function normalizePort(value: number): number {
  if (!Number.isFinite(value) || value <= 0 || value > 65535) {
    return 7797;
  }
  return Math.trunc(value);
}
