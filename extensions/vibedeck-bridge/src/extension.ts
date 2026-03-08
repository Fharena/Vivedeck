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
import {
  createCursorAgentCommandAdapter,
  type CursorAgentCommandAdapterConfig,
} from "./cursorAgentCommandAdapter.js";

type BridgeMode = "command" | "mock";
type CommandProviderMode = "builtin_cursor_agent" | "external";
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
  commandProvider: CommandProviderMode;
  tcpHost: string;
  tcpPort: number;
  commands: Partial<CursorBridgeCommands>;
  cursorAgent: CursorAgentCommandAdapterConfig;
}

interface ActiveBridge {
  server: SocketBridgeServer;
  runtime?: CursorExtensionRuntime;
  mode: BridgeMode;
  providerMode?: CommandProviderMode;
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
    vscode.commands.registerCommand("vibedeckBridge.copySmokeCommand", async () => {
      await copySmokeCommand();
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
      let runtime: CursorExtensionRuntime | undefined;
      try {
        if (settings.commandProvider === "builtin_cursor_agent") {
          runtime = await createBuiltinCommandRuntime(settings);
        }

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
          runtime,
          mode: settings.mode,
          providerMode: settings.commandProvider,
          address: server.address,
          settings,
          diagnostics,
        };
      } catch (error) {
        runtime?.dispose();
        throw error;
      }
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
    const address = resolveAddress(settings);
    const lines = [
      "VibeDeck bridge is stopped",
      `configured address: ${address}`,
      `agent env: ${buildAgentEnvCommand(address)}`,
      `provider: ${describeProvider(settings)}`,
    ];
    const smokeCommand = buildSmokeCommand(settings, address);
    if (smokeCommand) {
      lines.push(`smoke: ${smokeCommand}`);
    }
    return lines.join("\n");
  }

  return describeBridgeStatus(activeBridge);
}

function readSettings(): BridgeSettings {
  const config = vscode.workspace.getConfiguration("vibedeckBridge");
  const modeValue = config.get<string>("mode") === "mock" ? "mock" : "command";
  const providerValue =
    config.get<string>("commandProvider") === "external"
      ? "external"
      : "builtin_cursor_agent";
  return {
    autoStart: config.get<boolean>("autoStart", true),
    mode: modeValue,
    commandProvider: providerValue,
    tcpHost: config.get<string>("tcpHost", "127.0.0.1").trim() || "127.0.0.1",
    tcpPort: normalizePort(config.get<number>("tcpPort", 7797)),
    commands: readCommandSettings(config),
    cursorAgent: readCursorAgentSettings(config),
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

function readCursorAgentSettings(
  config: vscode.WorkspaceConfiguration,
): CursorAgentCommandAdapterConfig {
  const workspaceRoot = readOptionalCommand(config, "cursorAgent.workspaceRoot") ?? currentWorkspaceRoot();
  const extraArgs = readStringArray(config, "cursorAgent.extraArgs");
  const trustWorkspace = config.get<boolean>("cursorAgent.trustWorkspace", true);
  const model = (config.get<string>("cursorAgent.model", "auto") ?? "auto").trim();
  return {
    workspaceRoot,
    tempRoot: readOptionalCommand(config, "cursorAgent.tempRoot"),
    gitBin: readOptionalCommand(config, "cursorAgent.gitBin") ?? "git",
    cursorAgentBin: readOptionalCommand(config, "cursorAgent.bin") ?? "cursor-agent",
    cursorAgentArgs: ensureCursorAgentModelArg(
      ensureCursorAgentTrustFlag(extraArgs, trustWorkspace),
      model,
    ),
    cursorAgentEnv: readStringArray(config, "cursorAgent.extraEnv"),
    useWsl: config.get<boolean>("cursorAgent.useWsl", false),
    wslDistro: readOptionalCommand(config, "cursorAgent.wslDistro"),
    promptTimeoutMs: normalizeDuration(config.get<number>("cursorAgent.promptTimeoutMs", 120000), 120000),
    runTimeoutMs: normalizeDuration(config.get<number>("cursorAgent.runTimeoutMs", 120000), 120000),
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

function readStringArray(config: vscode.WorkspaceConfiguration, key: string): string[] {
  const value = config.get<unknown[]>(key, []);
  if (!Array.isArray(value)) {
    return [];
  }
  return value.filter((item): item is string => typeof item === "string" && item.trim().length > 0);
}

function currentWorkspaceRoot(): string | undefined {
  return vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
}

function asExtensionRuntimeVSCode(): VSCodeExtensionLike {
  return vscode as unknown as VSCodeExtensionLike;
}

function asHostVSCode(): VSCodeLike {
  return vscode as unknown as VSCodeLike;
}

async function createBuiltinCommandRuntime(
  settings: BridgeSettings,
): Promise<CursorExtensionRuntime> {
  if (!settings.cursorAgent.workspaceRoot?.trim()) {
    throw new Error(
      "workspace root is not configured. Open the project folder or set vibedeckBridge.cursorAgent.workspaceRoot.",
    );
  }

  const adapter = await createCursorAgentCommandAdapter(settings.cursorAgent);
  return createCursorExtensionRuntime({
    vscode: asExtensionRuntimeVSCode(),
    adapter,
    commands: settings.commands,
  });
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

async function copySmokeCommand(): Promise<void> {
  const settings = activeBridge?.settings ?? readSettings();
  const address = activeBridge?.address ?? resolveAddress(settings);
  const command = buildSmokeCommand(settings, address);
  if (!command) {
    void vscode.window.showWarningMessage(
      "Smoke command is available only in mock mode.",
    );
    return;
  }
  await vscode.env.clipboard.writeText(command);
  void vscode.window.showInformationMessage(`Copied smoke command: ${command}`);
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
    `provider: ${describeProvider(bridge.settings)}`,
  ];

  const smokeCommand = buildSmokeCommand(bridge.settings, bridge.address);
  if (smokeCommand) {
    lines.push(`smoke: ${smokeCommand}`);
  }

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
    `provider: ${describeProvider(settings)}`,
    `registered commands seen: ${diagnostics.availableCount}`,
    `required commands ready: ${diagnostics.required.length - diagnostics.missingRequired.length}/${diagnostics.required.length}`,
    `agent env: ${buildAgentEnvCommand(resolveAddress(settings))}`,
  ];

  const smokeCommand = buildSmokeCommand(settings, resolveAddress(settings));
  if (smokeCommand) {
    lines.push(`smoke: ${smokeCommand}`);
  }

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

function describeProvider(settings: BridgeSettings): string {
  if (settings.mode === "mock") {
    return "mock runtime";
  }
  if (settings.commandProvider === "external") {
    return "external command registry";
  }
  return settings.cursorAgent.useWsl
    ? `builtin cursor-agent via WSL${settings.cursorAgent.wslDistro ? ` (${settings.cursorAgent.wslDistro})` : ""}`
    : "builtin cursor-agent";
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

function buildSmokeCommand(settings: BridgeSettings, address: string): string | undefined {
  if (settings.mode === "mock") {
    return `powershell -ExecutionPolicy Bypass -File .\\scripts\\extension_host_smoke.ps1 -BridgeAddress "${address}"`;
  }
  if (settings.commandProvider === "builtin_cursor_agent") {
    return `powershell -ExecutionPolicy Bypass -File .\\scripts\\extension_command_smoke.ps1 -BridgeAddress "${address}"`;
  }
  return undefined;
}

function normalizePort(value: number): number {
  if (!Number.isFinite(value) || value <= 0 || value > 65535) {
    return 7797;
  }
  return Math.trunc(value);
}

function normalizeDuration(value: number, fallback: number): number {
  if (!Number.isFinite(value) || value < 1000) {
    return fallback;
  }
  return Math.trunc(value);
}

function ensureCursorAgentTrustFlag(args: string[], enabled: boolean): string[] {
  if (!enabled) {
    return [...args];
  }
  if (args.includes("--trust")) {
    return [...args];
  }
  return [...args, "--trust"];
}

function ensureCursorAgentModelArg(args: string[], model: string): string[] {
  const normalized = model.trim();
  if (!normalized) {
    return [...args];
  }
  for (let index = 0; index < args.length; index += 1) {
    const arg = args[index];
    if (arg === "--model" && index + 1 < args.length) {
      return [...args];
    }
    if (arg.startsWith("--model=")) {
      return [...args];
    }
  }
  return [...args, "--model", normalized];
}