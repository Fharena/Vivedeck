export interface CursorChatProbeTabLike {
  readonly label?: string;
  readonly isActive?: boolean;
  readonly isDirty?: boolean;
  readonly input?: unknown;
}

export interface CursorChatProbeTabGroupLike {
  readonly isActive?: boolean;
  readonly tabs?: readonly CursorChatProbeTabLike[];
}

export interface CursorChatProbeTabGroupsLike {
  readonly all?: readonly CursorChatProbeTabGroupLike[];
  readonly activeTabGroup?: CursorChatProbeTabGroupLike;
}

export interface CursorChatProbeVscodeLike {
  readonly commands: {
    getCommands(filterInternal?: boolean): Promise<string[]>;
  };
  readonly window: {
    readonly tabGroups?: CursorChatProbeTabGroupsLike;
  };
  readonly chat?: {
    readonly createChatParticipant?: unknown;
  };
  readonly lm?: {
    readonly selectChatModels?: unknown;
    readonly invokeTool?: unknown;
    readonly tools?: readonly unknown[];
  };
}

export interface CursorChatTabObservation {
  readonly label: string;
  readonly isActive: boolean;
  readonly groupIsActive: boolean;
  readonly inputKind: string;
  readonly detail: string[];
}

export interface CursorChatObservabilityReport {
  readonly checkedAt: string;
  readonly chatParticipantApi: boolean;
  readonly languageModelApi: {
    readonly available: boolean;
    readonly selectChatModels: boolean;
    readonly invokeTool: boolean;
    readonly toolCount: number;
  };
  readonly tabApi: {
    readonly available: boolean;
    readonly groupCount: number;
    readonly tabCount: number;
    readonly unknownInputCount: number;
    readonly tabs: readonly CursorChatTabObservation[];
  };
  readonly commandHints: readonly string[];
  readonly publicLimitations: readonly string[];
  readonly conclusions: {
    readonly canMirrorOwnParticipantHistory: boolean;
    readonly canInspectNativeTranscript: boolean;
    readonly confidence: "high" | "medium";
    readonly summary: string;
  };
}

const COMMAND_HINT_PATTERN = /(chat|composer|agent|cursor)/i;
const MAX_COMMAND_HINTS = 24;

export async function probeCursorChatObservability(
  vscodeLike: CursorChatProbeVscodeLike,
): Promise<CursorChatObservabilityReport> {
  const availableCommands = await safeGetCommands(vscodeLike);
  const tabs = collectTabObservations(vscodeLike.window.tabGroups);
  const unknownInputCount = tabs.filter((tab) => tab.inputKind === "unknown").length;
  const chatParticipantApi = typeof vscodeLike.chat?.createChatParticipant === "function";
  const languageModelApi = {
    available: typeof vscodeLike.lm === "object" && vscodeLike.lm !== null,
    selectChatModels: typeof vscodeLike.lm?.selectChatModels === "function",
    invokeTool: typeof vscodeLike.lm?.invokeTool === "function",
    toolCount: Array.isArray(vscodeLike.lm?.tools) ? vscodeLike.lm.tools.length : 0,
  };
  const commandHints = availableCommands
    .filter((command) => COMMAND_HINT_PATTERN.test(command))
    .sort((left, right) => left.localeCompare(right))
    .slice(0, MAX_COMMAND_HINTS);
  const publicLimitations = [
    "ChatContext.history는 현재 participant의 메시지만 노출하므로 기본 Cursor 채팅 transcript 전체 접근 API로 보기 어렵습니다.",
    "language model API는 모델 호출과 tool 연동 표면이지, 이미 열린 native chat 세션 내용을 읽는 표면은 아닙니다.",
    "tabGroups는 탭 껍데기와 input 종류 정도만 보여주며, 메시지 본문이나 진행 로그 본문은 읽지 못합니다.",
  ] as const;
  const conclusions = {
    canMirrorOwnParticipantHistory: chatParticipantApi,
    canInspectNativeTranscript: false,
    confidence: unknownInputCount > 0 ? "medium" : "high",
    summary:
      unknownInputCount > 0
        ? "공개 API 기준으로는 기본 Cursor 채팅 transcript 직접 읽기가 보이지 않습니다. 다만 런타임에 unknown tab input이 있어 추가 실험 여지는 남아 있습니다."
        : "공개 API 기준으로는 기본 Cursor 채팅 transcript 직접 읽기가 보이지 않습니다.",
  } as const;

  return {
    checkedAt: new Date().toISOString(),
    chatParticipantApi,
    languageModelApi,
    tabApi: {
      available: Boolean(vscodeLike.window.tabGroups),
      groupCount: vscodeLike.window.tabGroups?.all?.length ?? 0,
      tabCount: tabs.length,
      unknownInputCount,
      tabs,
    },
    commandHints,
    publicLimitations,
    conclusions,
  };
}

export function formatCursorChatObservabilityReport(
  report: CursorChatObservabilityReport,
): string {
  const lines = [
    "VibeDeck Cursor Chat Observability Probe",
    `checked at: ${report.checkedAt}`,
    `chat participant API: ${formatYesNo(report.chatParticipantApi)}`,
    `lm namespace: ${formatYesNo(report.languageModelApi.available)}`,
    `lm.selectChatModels: ${formatYesNo(report.languageModelApi.selectChatModels)}`,
    `lm.invokeTool: ${formatYesNo(report.languageModelApi.invokeTool)}`,
    `lm.tools visible: ${report.languageModelApi.toolCount}`,
    `tabGroups API: ${formatYesNo(report.tabApi.available)}`,
    `open tab groups: ${report.tabApi.groupCount}`,
    `open tabs observed: ${report.tabApi.tabCount}`,
  ];

  if (report.tabApi.tabs.length > 0) {
    lines.push("open tab hints:");
    for (const tab of report.tabApi.tabs) {
      const prefix = tab.isActive ? "- [active]" : "-";
      const detail = tab.detail.length > 0 ? ` | ${tab.detail.join(", ")}` : "";
      lines.push(`${prefix} ${tab.label} | input=${tab.inputKind}${detail}`);
    }
  }

  if (report.commandHints.length > 0) {
    lines.push(`command hints (${report.commandHints.length}):`);
    for (const command of report.commandHints) {
      lines.push(`- ${command}`);
    }
  }

  lines.push("public limitations:");
  for (const limitation of report.publicLimitations) {
    lines.push(`- ${limitation}`);
  }

  lines.push("conclusion:");
  lines.push(
    `- participant-owned history mirror: ${formatYesNo(report.conclusions.canMirrorOwnParticipantHistory)}`,
  );
  lines.push(
    `- native Cursor chat transcript direct access: ${formatYesNo(report.conclusions.canInspectNativeTranscript)}`,
  );
  lines.push(`- confidence: ${report.conclusions.confidence}`);
  lines.push(`- summary: ${report.conclusions.summary}`);

  return lines.join("\n");
}

async function safeGetCommands(vscodeLike: CursorChatProbeVscodeLike): Promise<string[]> {
  try {
    return await vscodeLike.commands.getCommands(true);
  } catch {
    return [];
  }
}

function collectTabObservations(
  tabGroups: CursorChatProbeTabGroupsLike | undefined,
): CursorChatTabObservation[] {
  if (!tabGroups?.all?.length) {
    return [];
  }

  const observations: CursorChatTabObservation[] = [];
  for (const group of tabGroups.all) {
    const groupIsActive = Boolean(group.isActive || tabGroups.activeTabGroup === group);
    for (const tab of group.tabs ?? []) {
      observations.push(describeTab(tab, groupIsActive));
    }
  }
  return observations;
}

function describeTab(
  tab: CursorChatProbeTabLike,
  groupIsActive: boolean,
): CursorChatTabObservation {
  const input = tab.input;
  const detail: string[] = [];
  const inputKind = detectInputKind(input);
  if (tab.isDirty) {
    detail.push("dirty");
  }
  if (groupIsActive) {
    detail.push("active-group");
  }

  const viewType = readStringProperty(input, "viewType");
  if (viewType) {
    detail.push(`viewType=${viewType}`);
  }
  const editorId = readStringProperty(input, "editorId");
  if (editorId) {
    detail.push(`editorId=${editorId}`);
  }
  const uri = readUriProperty(input, "uri");
  if (uri) {
    detail.push(`uri=${uri}`);
  }
  const inputKeys = Object.keys(asObject(input)).slice(0, 4);
  if (inputKeys.length > 0) {
    detail.push(`keys=${inputKeys.join("/")}`);
  }

  return {
    label: tab.label?.trim() || "(untitled tab)",
    isActive: Boolean(tab.isActive),
    groupIsActive,
    inputKind,
    detail,
  };
}

function detectInputKind(input: unknown): string {
  if (!input || typeof input !== "object") {
    return "unknown";
  }
  const ctorName = input.constructor?.name?.trim();
  if (ctorName && ctorName !== "Object") {
    return ctorName;
  }
  const viewType = readStringProperty(input, "viewType");
  if (viewType) {
    return "webview-like";
  }
  if (readUriProperty(input, "uri")) {
    return "uri-like";
  }
  return "unknown";
}

function readStringProperty(value: unknown, key: string): string | undefined {
  const record = asObject(value);
  const candidate = record[key];
  return typeof candidate === "string" && candidate.trim().length > 0 ? candidate.trim() : undefined;
}

function readUriProperty(value: unknown, key: string): string | undefined {
  const record = asObject(value);
  const candidate = asObject(record[key]);
  const fsPath = candidate.fsPath;
  if (typeof fsPath === "string" && fsPath.trim().length > 0) {
    return fsPath.trim();
  }
  const path = candidate.path;
  if (typeof path === "string" && path.trim().length > 0) {
    return path.trim();
  }
  return undefined;
}

function asObject(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" ? (value as Record<string, unknown>) : {};
}

function formatYesNo(value: boolean): string {
  return value ? "yes" : "no";
}
