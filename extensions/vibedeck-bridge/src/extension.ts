import * as vscode from "vscode";
import {
  MockCursorBridge,
  createCursorExtensionBridge,
  createCursorExtensionRuntime,
  createVSCodeCursorHost,
  defaultCursorBridgeCommands,
  serveSocketBridge,
  type CursorBridgeCommands,
  type CursorExtensionRuntime,
  type SocketBridgeServer,
  type VSCodeExtensionLike,
  type VSCodeLike,
} from "@vibedeck/cursor-bridge";

type BridgeMode = "command" | "mock";
type CommandKey = keyof CursorBridgeCommands;

const REQUIRED_COMMAND_KEYS = [
  "submitTask",
  "getPatch",
  "applyPatch",
  "runProfile",
  "getRunResult",
] as const satisfies readonly CommandKey[];

const OPTIONAL_COMMAND_KEYS = [
  "openLocation",
  "getWorkspaceMetadata",
  "getLatestTerminalError",
] as const satisfies readonly CommandKey[];

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
  settings: BridgeSettings;
  diagnostics?: BridgeCommandDiagnostics;
}

interface BridgeCommandBinding {
  key: CommandKey;
  commandId: string;
}

interface BridgeCommandDiagnostics {
  checkedAt: string;
  availableCount: number;
  required: BridgeCommandBinding[];
  optional: BridgeCommandBinding[];
  missingRequired: BridgeCommandBinding[];
  missingOptional: BridgeCommandBinding[];
}

let activeBridge: ActiveBridge | undefined;
let statusBarItem: vscode.StatusBarItem | undefined;
let lastBridgeError: string | undefined;

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
      await showBridgeStatus();
    }),
  );
  context.subscriptions.push(
    vscode.commands.registerCommand("vibedeckBridge.validateCommands", async () => {
      await validateCommands(true);
    }),
  );
  context.subscriptions.push(
    vscode.commands.registerCommand("vibedeckBridge.copyAgentEnv", async () => {
      await copyAgentEnv();
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
  lastBridgeError = undefined;
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
        settings,
      };
    } else {
      const diagnostics = await validateCommandModeSettings(settings);
      if (diagnostics.missingRequired.length > 0) {
        throw new Error(
          `missing required commands: ${formatCommandBindings(diagnostics.missingRequired)}`,
        );
      }

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
        settings,
        diagnostics,
      };
    }

    activeBridge = startedBridge;
    updateStatusBar();
    if (showMessage) {
      await showStartedMessage(startedBridge);
    } else if (startedBridge.diagnostics && startedBridge.diagnostics.missingOptional.length > 0) {
      void vscode.window.showWarningMessage(describeBridgeStatus(startedBridge));
    }
  } catch (error) {
    activeBridge = undefined;
    const message = error instanceof Error ? error.message : String(error);
    lastBridgeError = message;
    updateStatusBar();
    void vscode.window.showErrorMessage(`VibeDeck bridge start failed: ${message}`);
  }
}

async function stopServer(showMessage: boolean): Promise<void> {
  const bridge = activeBridge;
  activeBridge = undefined;
  lastBridgeError = undefined;

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
    if (lastBridgeError) {
      statusBarItem.text = "VibeDeck: issue";
      statusBarItem.tooltip = `VibeDeck bridge stopped\nlast error: ${lastBridgeError}`;
    } else {
      statusBarItem.text = "VibeDeck: stopped";
      statusBarItem.tooltip = "VibeDeck localhost bridge is stopped";
    }
    statusBarItem.show();
    return;
  }

  statusBarItem.text =
    `VibeDeck: ${activeBridge.address}` +
    (activeBridge.diagnostics?.missingOptional.length ? " !" : "");
  statusBarItem.tooltip = describeBridgeStatus(activeBridge);
  statusBarItem.show();
}

function currentStatusMessage(): string {
  if (!activeBridge) {
    if (lastBridgeError) {
      return `VibeDeck bridge is stopped\nlast error: ${lastBridgeError}`;
    }

    const settings = readSettings();
    return [
      "VibeDeck bridge is stopped",
      `configured address: ${resolveAddress(settings)}`,
      `agent env: ${buildAgentEnvCommand(resolveAddress(settings))}`,
    ].join("\n");
  }

  return describeBridgeStatus(activeBridge);
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

async function showBridgeStatus(): Promise<void> {
  const message = currentStatusMessage();
  if (activeBridge?.diagnostics?.missingOptional.length || lastBridgeError) {
    void vscode.window.showWarningMessage(message);
    return;
  }

  void vscode.window.showInformationMessage(message);
}

async function validateCommands(showMessage: boolean): Promise<BridgeCommandDiagnostics | undefined> {
  const settings = readSettings();
  if (settings.mode !== "command") {
    if (showMessage) {
      void vscode.window.showInformationMessage(
        "VibeDeck bridge is in mock mode. Command validation is skipped.",
      );
    }
    return undefined;
  }

  const diagnostics = await validateCommandModeSettings(settings);
  if (showMessage) {
    const message = describeCommandDiagnostics(settings, diagnostics);
    if (diagnostics.missingRequired.length > 0) {
      void vscode.window.showErrorMessage(message);
    } else if (diagnostics.missingOptional.length > 0) {
      void vscode.window.showWarningMessage(message);
    } else {
      void vscode.window.showInformationMessage(message);
    }
  }

  return diagnostics;
}

async function copyAgentEnv(): Promise<void> {
  const settings = activeBridge?.settings ?? readSettings();
  const address = activeBridge?.address ?? resolveAddress(settings);
  const command = buildAgentEnvCommand(address);
  await vscode.env.clipboard.writeText(command);
  void vscode.window.showInformationMessage(`Copied agent env: ${command}`);
}

async function showStartedMessage(bridge: ActiveBridge): Promise<void> {
  const message = describeBridgeStatus(bridge);
  if (bridge.diagnostics?.missingOptional.length) {
    void vscode.window.showWarningMessage(message);
    return;
  }

  void vscode.window.showInformationMessage(message);
}

async function validateCommandModeSettings(
  settings: BridgeSettings,
): Promise<BridgeCommandDiagnostics> {
  const availableCommands = new Set(await vscode.commands.getCommands(true));
  const resolvedCommands = resolveCommands(settings.commands);
  const required = collectCommandBindings(resolvedCommands, REQUIRED_COMMAND_KEYS);
  const optional = collectCommandBindings(resolvedCommands, OPTIONAL_COMMAND_KEYS);

  return {
    checkedAt: new Date().toISOString(),
    availableCount: availableCommands.size,
    required,
    optional,
    missingRequired: required.filter((binding) => !availableCommands.has(binding.commandId)),
    missingOptional: optional.filter((binding) => !availableCommands.has(binding.commandId)),
  };
}

function describeBridgeStatus(bridge: ActiveBridge): string {
  const lines = [
    `VibeDeck bridge: ${bridge.address} (${bridge.mode})`,
    `agent env: ${buildAgentEnvCommand(bridge.address)}`,
  ];

  if (!bridge.diagnostics) {
    return lines.join("\n");
  }

  lines.push(
    `required commands ready: ${bridge.diagnostics.required.length - bridge.diagnostics.missingRequired.length}/${bridge.diagnostics.required.length}`,
  );

  if (bridge.diagnostics.optional.length > 0) {
    lines.push(
      `optional commands ready: ${bridge.diagnostics.optional.length - bridge.diagnostics.missingOptional.length}/${bridge.diagnostics.optional.length}`,
    );
  }

  if (bridge.diagnostics.missingOptional.length > 0) {
    lines.push(`missing optional: ${formatCommandBindings(bridge.diagnostics.missingOptional)}`);
  }

  return lines.join("\n");
}

function describeCommandDiagnostics(
  settings: BridgeSettings,
  diagnostics: BridgeCommandDiagnostics,
): string {
  const lines = [
    `VibeDeck command validation: ${resolveAddress(settings)}`,
    `registered commands seen: ${diagnostics.availableCount}`,
    `required commands ready: ${diagnostics.required.length - diagnostics.missingRequired.length}/${diagnostics.required.length}`,
    `agent env: ${buildAgentEnvCommand(resolveAddress(settings))}`,
  ];

  if (diagnostics.optional.length > 0) {
    lines.push(
      `optional commands ready: ${diagnostics.optional.length - diagnostics.missingOptional.length}/${diagnostics.optional.length}`,
    );
  }

  if (diagnostics.missingRequired.length > 0) {
    lines.push(`missing required: ${formatCommandBindings(diagnostics.missingRequired)}`);
  }

  if (diagnostics.missingOptional.length > 0) {
    lines.push(`missing optional: ${formatCommandBindings(diagnostics.missingOptional)}`);
  }

  lines.push(`checked at: ${diagnostics.checkedAt}`);
  return lines.join("\n");
}

function resolveCommands(commands: Partial<CursorBridgeCommands>): CursorBridgeCommands {
  return {
    ...defaultCursorBridgeCommands,
    ...commands,
  };
}

function collectCommandBindings(
  commands: CursorBridgeCommands,
  keys: readonly CommandKey[],
): BridgeCommandBinding[] {
  const bindings: BridgeCommandBinding[] = [];
  for (const key of keys) {
    const commandId = commands[key];
    if (typeof commandId !== "string" || commandId.length === 0) {
      continue;
    }

    bindings.push({ key, commandId });
  }

  return bindings;
}

function formatCommandBindings(bindings: BridgeCommandBinding[]): string {
  return bindings.map((binding) => `${binding.key}=${binding.commandId}`).join(", ");
}

function resolveAddress(settings: BridgeSettings): string {
  return `${settings.tcpHost}:${settings.tcpPort}`;
}

function buildAgentEnvCommand(address: string): string {
  return `$env:CURSOR_BRIDGE_TCP_ADDR = "${address}"`;
}

function normalizePort(value: number): number {
  if (!Number.isFinite(value) || value <= 0 || value > 65535) {
    return 7797;
  }
  return Math.trunc(value);
}
