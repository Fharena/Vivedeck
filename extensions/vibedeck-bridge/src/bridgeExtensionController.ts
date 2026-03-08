import {
  MockCursorBridge,
  createCursorExtensionBridge,
  createCursorExtensionRuntime,
  createVSCodeCursorHost,
  defaultCursorBridgeCommands,
  serveSocketBridge,
  type CursorBridgeCommands,
  type CursorExtensionRuntime,
  type DisposableLike,
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

export interface BridgeExtensionContextLike {
  subscriptions: DisposableLike[];
}

export interface BridgeExtensionStatusBarItemLike extends DisposableLike {
  text: string;
  tooltip?: string;
  command?: string;
  show(): void;
}

export interface BridgeExtensionConfigurationLike {
  get<T>(key: string, defaultValue?: T): T;
}

export interface BridgeExtensionConfigurationChangeEventLike {
  affectsConfiguration(section: string): boolean;
}

export interface BridgeExtensionCommandsLike {
  executeCommand<T = unknown>(command: string, ...args: unknown[]): Promise<T>;
  registerCommand(command: string, callback: (...args: unknown[]) => unknown): DisposableLike;
  getCommands(filterInternal?: boolean): Promise<string[]>;
}

export interface BridgeExtensionWindowLike {
  activeTextEditor?: VSCodeLike["window"]["activeTextEditor"];
  showTextDocument: VSCodeLike["window"]["showTextDocument"];
  showInformationMessage(message: string): unknown;
  showWarningMessage(message: string): unknown;
  showErrorMessage(message: string): unknown;
  createStatusBarItem(alignment: number, priority?: number): BridgeExtensionStatusBarItemLike;
}

export interface BridgeExtensionWorkspaceFolderLike {
  uri: {
    fsPath: string;
  };
}

export interface BridgeExtensionWorkspaceLike {
  textDocuments?: VSCodeLike["workspace"]["textDocuments"];
  openTextDocument: VSCodeLike["workspace"]["openTextDocument"];
  getConfiguration(section?: string): BridgeExtensionConfigurationLike;
  onDidChangeConfiguration(
    listener: (event: BridgeExtensionConfigurationChangeEventLike) => unknown,
  ): DisposableLike;
  workspaceFolders?: BridgeExtensionWorkspaceFolderLike[];
}

export interface BridgeExtensionEnvLike {
  clipboard: {
    writeText(text: string): Promise<void>;
  };
}

export interface BridgeExtensionVscodeLike {
  commands: BridgeExtensionCommandsLike;
  window: BridgeExtensionWindowLike;
  workspace: BridgeExtensionWorkspaceLike;
  env: BridgeExtensionEnvLike;
  statusBarAlignment: {
    left: number;
  };
}

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

export interface BridgeExtensionController {
  activate(context: BridgeExtensionContextLike): Promise<void>;
  deactivate(): Promise<void>;
}

export function createBridgeExtensionController(
  vscodeLike: BridgeExtensionVscodeLike,
): BridgeExtensionController {
  return new DefaultBridgeExtensionController(vscodeLike);
}

class DefaultBridgeExtensionController implements BridgeExtensionController {
  private readonly vscode: BridgeExtensionVscodeLike;
  private activeBridge: ActiveBridge | undefined;
  private statusBarItem: BridgeExtensionStatusBarItemLike | undefined;
  private lastBridgeError: string | undefined;

  constructor(vscodeLike: BridgeExtensionVscodeLike) {
    this.vscode = vscodeLike;
  }

  async activate(context: BridgeExtensionContextLike): Promise<void> {
    this.statusBarItem = this.vscode.window.createStatusBarItem(
      this.vscode.statusBarAlignment.left,
      100,
    );
    this.statusBarItem.command = "vibedeckBridge.showStatus";
    context.subscriptions.push(this.statusBarItem);

    context.subscriptions.push(
      this.vscode.commands.registerCommand("vibedeckBridge.startServer", async () => {
        await this.startServer(true);
      }),
    );
    context.subscriptions.push(
      this.vscode.commands.registerCommand("vibedeckBridge.stopServer", async () => {
        await this.stopServer(true);
      }),
    );
    context.subscriptions.push(
      this.vscode.commands.registerCommand("vibedeckBridge.showStatus", async () => {
        await this.showBridgeStatus();
      }),
    );
    context.subscriptions.push(
      this.vscode.commands.registerCommand("vibedeckBridge.validateCommands", async () => {
        await this.validateCommands(true);
      }),
    );
    context.subscriptions.push(
      this.vscode.commands.registerCommand("vibedeckBridge.copyAgentEnv", async () => {
        await this.copyAgentEnv();
      }),
    );
    context.subscriptions.push(
      this.vscode.commands.registerCommand("vibedeckBridge.copySmokeCommand", async () => {
        await this.copySmokeCommand();
      }),
    );
    context.subscriptions.push(
      this.vscode.workspace.onDidChangeConfiguration((event) => {
        if (!event.affectsConfiguration("vibedeckBridge")) {
          return;
        }
        void this.restartServer();
      }),
    );

    this.updateStatusBar();

    if (this.readSettings().autoStart) {
      await this.startServer(false);
    }
  }

  async deactivate(): Promise<void> {
    await this.stopServer(false);
  }

  private async startServer(showMessage: boolean): Promise<void> {
    await this.stopServer(false);

    const settings = this.readSettings();
    this.lastBridgeError = undefined;
    try {
      let startedBridge: ActiveBridge;

      if (settings.mode === "mock") {
        const runtime = createCursorExtensionRuntime({
          vscode: this.asExtensionRuntimeVSCode(),
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
            runtime = await this.createBuiltinCommandRuntime(settings);
          }

          const diagnostics = await this.validateCommandModeSettings(settings);
          if (diagnostics.missingRequired.length > 0) {
            throw new Error(
              `missing required commands: ${formatCommandBindings(diagnostics.missingRequired)}`,
            );
          }

          const bridge = createCursorExtensionBridge({
            host: createVSCodeCursorHost(this.asHostVSCode()),
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

      this.activeBridge = startedBridge;
      this.updateStatusBar();
      if (showMessage) {
        await this.showStartedMessage(startedBridge);
      } else if (
        startedBridge.diagnostics &&
        startedBridge.diagnostics.missingOptional.length > 0
      ) {
        void this.vscode.window.showWarningMessage(this.describeBridgeStatus(startedBridge));
      }
    } catch (error) {
      this.activeBridge = undefined;
      const message = error instanceof Error ? error.message : String(error);
      this.lastBridgeError = message;
      this.updateStatusBar();
      void this.vscode.window.showErrorMessage(`VibeDeck bridge start failed: ${message}`);
    }
  }

  private async stopServer(showMessage: boolean): Promise<void> {
    const bridge = this.activeBridge;
    this.activeBridge = undefined;
    this.lastBridgeError = undefined;

    if (bridge?.runtime) {
      bridge.runtime.dispose();
    }
    if (bridge?.server) {
      await bridge.server.close();
    }

    this.updateStatusBar();
    if (showMessage && bridge) {
      void this.vscode.window.showInformationMessage("VibeDeck bridge stopped");
    }
  }

  private async restartServer(): Promise<void> {
    if (!this.readSettings().autoStart) {
      await this.stopServer(false);
      return;
    }
    await this.startServer(false);
  }

  private updateStatusBar(): void {
    if (!this.statusBarItem) {
      return;
    }

    if (!this.activeBridge) {
      if (this.lastBridgeError) {
        this.statusBarItem.text = "VibeDeck: issue";
        this.statusBarItem.tooltip = `VibeDeck bridge stopped\nlast error: ${this.lastBridgeError}`;
      } else {
        this.statusBarItem.text = "VibeDeck: stopped";
        this.statusBarItem.tooltip = "VibeDeck localhost bridge is stopped";
      }
      this.statusBarItem.show();
      return;
    }

    this.statusBarItem.text =
      `VibeDeck: ${this.activeBridge.address}` +
      (this.activeBridge.diagnostics?.missingOptional.length ? " !" : "");
    this.statusBarItem.tooltip = this.describeBridgeStatus(this.activeBridge);
    this.statusBarItem.show();
  }

  private currentStatusMessage(): string {
    if (!this.activeBridge) {
      if (this.lastBridgeError) {
        return `VibeDeck bridge is stopped\nlast error: ${this.lastBridgeError}`;
      }

      const settings = this.readSettings();
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

    return this.describeBridgeStatus(this.activeBridge);
  }

  private readSettings(): BridgeSettings {
    const config = this.vscode.workspace.getConfiguration("vibedeckBridge");
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
      cursorAgent: readCursorAgentSettings(config, this.vscode.workspace.workspaceFolders),
    };
  }

  private asExtensionRuntimeVSCode(): VSCodeExtensionLike {
    return this.vscode as unknown as VSCodeExtensionLike;
  }

  private asHostVSCode(): VSCodeLike {
    return this.vscode as unknown as VSCodeLike;
  }

  private async createBuiltinCommandRuntime(
    settings: BridgeSettings,
  ): Promise<CursorExtensionRuntime> {
    if (!settings.cursorAgent.workspaceRoot?.trim()) {
      throw new Error(
        "workspace root is not configured. Open the project folder or set vibedeckBridge.cursorAgent.workspaceRoot.",
      );
    }

    const adapter = await createCursorAgentCommandAdapter(settings.cursorAgent);
    return createCursorExtensionRuntime({
      vscode: this.asExtensionRuntimeVSCode(),
      adapter,
      commands: settings.commands,
    });
  }

  private async showBridgeStatus(): Promise<void> {
    const message = this.currentStatusMessage();
    if (this.activeBridge?.diagnostics?.missingOptional.length || this.lastBridgeError) {
      void this.vscode.window.showWarningMessage(message);
      return;
    }

    void this.vscode.window.showInformationMessage(message);
  }

  private async validateCommands(
    showMessage: boolean,
  ): Promise<BridgeCommandDiagnostics | undefined> {
    const settings = this.readSettings();
    if (settings.mode !== "command") {
      if (showMessage) {
        void this.vscode.window.showInformationMessage(
          "VibeDeck bridge is in mock mode. Command validation is skipped.",
        );
      }
      return undefined;
    }

    const diagnostics = await this.validateCommandModeSettings(settings);
    if (showMessage) {
      const message = describeCommandDiagnostics(settings, diagnostics);
      if (diagnostics.missingRequired.length > 0) {
        void this.vscode.window.showErrorMessage(message);
      } else if (diagnostics.missingOptional.length > 0) {
        void this.vscode.window.showWarningMessage(message);
      } else {
        void this.vscode.window.showInformationMessage(message);
      }
    }

    return diagnostics;
  }

  private async copyAgentEnv(): Promise<void> {
    const settings = this.activeBridge?.settings ?? this.readSettings();
    const address = this.activeBridge?.address ?? resolveAddress(settings);
    const command = buildAgentEnvCommand(address);
    await this.vscode.env.clipboard.writeText(command);
    void this.vscode.window.showInformationMessage(`Copied agent env: ${command}`);
  }

  private async copySmokeCommand(): Promise<void> {
    const settings = this.activeBridge?.settings ?? this.readSettings();
    const address = this.activeBridge?.address ?? resolveAddress(settings);
    const command = buildSmokeCommand(settings, address);
    if (!command) {
      void this.vscode.window.showWarningMessage(
        "Smoke command is not available for the current provider.",
      );
      return;
    }
    await this.vscode.env.clipboard.writeText(command);
    void this.vscode.window.showInformationMessage(`Copied smoke command: ${command}`);
  }

  private async showStartedMessage(bridge: ActiveBridge): Promise<void> {
    const message = this.describeBridgeStatus(bridge);
    if (bridge.diagnostics?.missingOptional.length) {
      void this.vscode.window.showWarningMessage(message);
      return;
    }

    void this.vscode.window.showInformationMessage(message);
  }

  private async validateCommandModeSettings(
    settings: BridgeSettings,
  ): Promise<BridgeCommandDiagnostics> {
    const availableCommands = new Set(await this.vscode.commands.getCommands(true));
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

  private describeBridgeStatus(bridge: ActiveBridge): string {
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
}

function readCommandSettings(
  config: BridgeExtensionConfigurationLike,
): Partial<CursorBridgeCommands> {
  const commands: Partial<CursorBridgeCommands> = {};
  setOptionalCommand(commands, "submitTask", readOptionalCommand(config, "commands.submitTask"));
  setOptionalCommand(commands, "getPatch", readOptionalCommand(config, "commands.getPatch"));
  setOptionalCommand(commands, "applyPatch", readOptionalCommand(config, "commands.applyPatch"));
  setOptionalCommand(commands, "runProfile", readOptionalCommand(config, "commands.runProfile"));
  setOptionalCommand(commands, "getRunResult", readOptionalCommand(config, "commands.getRunResult"));
  setOptionalCommand(commands, "openLocation", readOptionalCommand(config, "commands.openLocation"));
  setOptionalCommand(commands, "getWorkspaceMetadata", readOptionalCommand(config, "commands.getWorkspaceMetadata"));
  setOptionalCommand(commands, "getLatestTerminalError", readOptionalCommand(config, "commands.getLatestTerminalError"));
  return commands;
}

function readCursorAgentSettings(
  config: BridgeExtensionConfigurationLike,
  workspaceFolders?: BridgeExtensionWorkspaceFolderLike[],
): CursorAgentCommandAdapterConfig {
  const workspaceRoot =
    readOptionalCommand(config, "cursorAgent.workspaceRoot") ?? workspaceFolders?.[0]?.uri.fsPath;
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
    promptTimeoutMs: normalizeDuration(
      config.get<number>("cursorAgent.promptTimeoutMs", 120000),
      120000,
    ),
    runTimeoutMs: normalizeDuration(
      config.get<number>("cursorAgent.runTimeoutMs", 120000),
      120000,
    ),
  };
}

function setOptionalCommand(
  commands: Partial<CursorBridgeCommands>,
  key: keyof CursorBridgeCommands,
  value: string | undefined,
): void {
  if (!value) {
    return;
  }
  commands[key] = value;
}
function readOptionalCommand(
  config: BridgeExtensionConfigurationLike,
  key: string,
): string | undefined {
  const value = config.get<string | undefined>(key)?.trim();
  if (!value) {
    return undefined;
  }
  return value;
}

function readStringArray(config: BridgeExtensionConfigurationLike, key: string): string[] {
  const value = config.get<unknown[]>(key, []);
  if (!Array.isArray(value)) {
    return [];
  }
  return value.filter((item): item is string => typeof item === "string" && item.trim().length > 0);
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
    return "npm --prefix extensions/vibedeck-bridge run smoke:extension";
  }
  return undefined;
}

function normalizePort(value: number): number {
  if (!Number.isFinite(value) || value < 0 || value > 65535) {
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