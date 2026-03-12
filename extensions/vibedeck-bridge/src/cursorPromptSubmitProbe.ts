export interface CursorPromptSubmitProbeVscodeLike {
  readonly commands: {
    getCommands(filterInternal?: boolean): Promise<string[]>;
  };
}

export type CursorPromptSubmitCandidateRole =
  | "direct"
  | "open"
  | "focus"
  | "paste";

export type CursorPromptSubmitStrategyKind =
  | "direct_command"
  | "open_then_paste"
  | "manual_only"
  | "unavailable";

export interface CursorPromptSubmitCandidate {
  readonly id: string;
  readonly role: CursorPromptSubmitCandidateRole;
  readonly available: boolean;
  readonly note: string;
}

export interface CursorPromptSubmitStrategy {
  readonly kind: CursorPromptSubmitStrategyKind;
  readonly confidence: "high" | "medium" | "low";
  readonly commandIds: readonly string[];
  readonly canAutomateSubmit: boolean;
  readonly summary: string;
}

export interface CursorPromptSubmitReport {
  readonly checkedAt: string;
  readonly candidates: readonly CursorPromptSubmitCandidate[];
  readonly availableCandidateCount: number;
  readonly recommendedStrategy: CursorPromptSubmitStrategy;
  readonly nextActions: readonly string[];
}

interface CursorPromptSubmitCandidateDefinition {
  readonly id: string;
  readonly role: CursorPromptSubmitCandidateRole;
  readonly note: string;
}

const CANDIDATE_DEFINITIONS: readonly CursorPromptSubmitCandidateDefinition[] = [
  {
    id: "cursor.startComposerPrompt",
    role: "direct",
    note: "Cursor 2.3에서 direct prompt command로 보고된 가장 유력한 후보입니다.",
  },
  {
    id: "composer.newAgentChat",
    role: "open",
    note: "새 agent chat 세션을 여는 후보입니다. prompt 인자 시그니처는 아직 확정되지 않았습니다.",
  },
  {
    id: "workbench.action.chat.open",
    role: "open",
    note: "VS Code chat 표면을 여는 공식 command 후보입니다. open only인지 prompt 전달까지 되는지는 런타임 확인이 더 필요합니다.",
  },
  {
    id: "workbench.action.chat.newChat",
    role: "open",
    note: "새 chat 창을 여는 표면 후보입니다.",
  },
  {
    id: "cursor.composer.new",
    role: "open",
    note: "Cursor composer 표면을 새로 여는 후보입니다.",
  },
  {
    id: "cursor.chat.focus",
    role: "focus",
    note: "기존 Cursor chat 입력창으로 focus를 옮기는 후보입니다.",
  },
  {
    id: "workbench.action.quickchat.toggle",
    role: "open",
    note: "VS Code quick chat을 여는 후보입니다.",
  },
  {
    id: "editor.action.clipboardPasteAction",
    role: "paste",
    note: "chat 입력창 focus 이후 clipboard paste fallback에 쓰일 수 있는 후보입니다.",
  },
] as const;

export async function probeCursorPromptSubmitPath(
  vscodeLike: CursorPromptSubmitProbeVscodeLike,
): Promise<CursorPromptSubmitReport> {
  const commands = new Set(await safeGetCommands(vscodeLike));
  const candidates = CANDIDATE_DEFINITIONS.map((candidate) => ({
    ...candidate,
    available: commands.has(candidate.id),
  }));
  const recommendedStrategy = resolveRecommendedStrategy(candidates);

  return {
    checkedAt: new Date().toISOString(),
    candidates,
    availableCandidateCount: candidates.filter((candidate) => candidate.available).length,
    recommendedStrategy,
    nextActions: buildNextActions(recommendedStrategy),
  };
}

export function formatCursorPromptSubmitReport(
  report: CursorPromptSubmitReport,
): string {
  const lines = [
    "VibeDeck Cursor Prompt Submit Probe",
    `checked at: ${report.checkedAt}`,
    `candidate commands available: ${report.availableCandidateCount}/${report.candidates.length}`,
    "candidate commands:",
  ];

  for (const candidate of report.candidates) {
    lines.push(
      `- ${candidate.available ? "yes" : "no"} | ${candidate.role} | ${candidate.id} | ${candidate.note}`,
    );
  }

  lines.push("recommended strategy:");
  lines.push(`- kind: ${report.recommendedStrategy.kind}`);
  lines.push(`- confidence: ${report.recommendedStrategy.confidence}`);
  lines.push(
    `- can automate submit: ${formatYesNo(report.recommendedStrategy.canAutomateSubmit)}`,
  );
  lines.push(
    `- commands: ${report.recommendedStrategy.commandIds.length > 0 ? report.recommendedStrategy.commandIds.join(", ") : "(none)"}`,
  );
  lines.push(`- summary: ${report.recommendedStrategy.summary}`);

  lines.push("next actions:");
  for (const action of report.nextActions) {
    lines.push(`- ${action}`);
  }

  return lines.join("\n");
}

function resolveRecommendedStrategy(
  candidates: readonly CursorPromptSubmitCandidate[],
): CursorPromptSubmitStrategy {
  const available = new Set(
    candidates.filter((candidate) => candidate.available).map((candidate) => candidate.id),
  );

  if (available.has("cursor.startComposerPrompt")) {
    return {
      kind: "direct_command",
      confidence: "high",
      commandIds: ["cursor.startComposerPrompt"],
      canAutomateSubmit: true,
      summary:
        "`cursor.startComposerPrompt`가 보여서 가장 유력한 direct submit 경로로 판단했습니다.",
    };
  }

  const hasOpenSurface =
    available.has("composer.newAgentChat") ||
    available.has("workbench.action.chat.open") ||
    available.has("workbench.action.chat.newChat") ||
    available.has("cursor.composer.new") ||
    available.has("workbench.action.quickchat.toggle");
  const focusCommand = available.has("cursor.chat.focus") ? ["cursor.chat.focus"] : [];
  const openCommands = pickAvailableCommands(available, [
    "composer.newAgentChat",
    "workbench.action.chat.open",
    "workbench.action.chat.newChat",
    "cursor.composer.new",
    "workbench.action.quickchat.toggle",
  ]);

  if (hasOpenSurface && available.has("editor.action.clipboardPasteAction")) {
    return {
      kind: "open_then_paste",
      confidence: "medium",
      commandIds: [...openCommands.slice(0, 1), ...focusCommand, "editor.action.clipboardPasteAction"],
      canAutomateSubmit: false,
      summary:
        "직접 prompt command는 안 보이지만, chat/composer를 열고 paste 하는 fallback 경로는 만들 수 있어 보입니다.",
    };
  }

  if (hasOpenSurface) {
    return {
      kind: "manual_only",
      confidence: "low",
      commandIds: [...openCommands.slice(0, 1), ...focusCommand],
      canAutomateSubmit: false,
      summary:
        "chat 표면을 여는 command는 보이지만, 자동 paste까지 이어질 안전한 경로는 아직 부족합니다.",
    };
  }

  return {
    kind: "unavailable",
    confidence: "low",
    commandIds: [],
    canAutomateSubmit: false,
    summary:
      "현재 공개 command 목록에서는 Cursor 기본 채팅에 프롬프트를 넣을 경로를 찾지 못했습니다.",
  };
}

function buildNextActions(strategy: CursorPromptSubmitStrategy): readonly string[] {
  if (strategy.kind === "direct_command") {
    return [
      "모바일 요청을 PC extension submit path에 연결할 수 있습니다.",
      "다음 단계에서 실제 prompt submit wrapper를 붙여도 됩니다.",
    ];
  }

  if (strategy.kind === "open_then_paste") {
    return [
      "direct submit command가 없으므로 open/focus 이후 paste fallback을 실험해야 합니다.",
      "실제 실행 전에 clipboard 복원과 focus 안정성을 먼저 검토해야 합니다.",
    ];
  }

  if (strategy.kind === "manual_only") {
    return [
      "자동 submit보다 우선 SQLite reader와 세션 매핑을 준비하는 편이 안전합니다.",
      "필요하면 OS-level automation이나 더 깊은 Cursor command 조사가 뒤따라야 합니다.",
    ];
  }

  return [
    "Cursor local chat history reader를 먼저 붙여 읽기 경로를 확보합니다.",
    "보내기 경로는 추가 command 조사나 외부 자동화 없이 확정하지 않는 편이 좋습니다.",
  ];
}

function pickAvailableCommands(
  available: ReadonlySet<string>,
  preferredOrder: readonly string[],
): string[] {
  return preferredOrder.filter((command) => available.has(command));
}

async function safeGetCommands(vscodeLike: CursorPromptSubmitProbeVscodeLike): Promise<string[]> {
  try {
    return await vscodeLike.commands.getCommands(true);
  } catch {
    return [];
  }
}

function formatYesNo(value: boolean): string {
  return value ? "yes" : "no";
}
