package agent

import (
	"sort"
	"strings"
	"sync"

	"github.com/Fharena/VibeDeck/internal/protocol"
)

type SharedSessionSummary struct {
	ID               string `json:"id"`
	ThreadID         string `json:"threadId"`
	ControlSessionID string `json:"controlSessionId,omitempty"`
	Title            string `json:"title"`
	Provider         string `json:"provider,omitempty"`
	WorkspaceRoot    string `json:"workspaceRoot,omitempty"`
	CurrentJobID     string `json:"currentJobId,omitempty"`
	Phase            string `json:"phase,omitempty"`
	LastEventKind    string `json:"lastEventKind,omitempty"`
	LastEventText    string `json:"lastEventText,omitempty"`
	UpdatedAt        int64  `json:"updatedAt"`
}

type SharedSessionDetail struct {
	Session        SharedSessionSummary  `json:"session"`
	LiveState      SessionLiveState      `json:"liveState"`
	OperationState SessionOperationState `json:"operationState"`
	Timeline       []ThreadEvent         `json:"timeline"`
}

type SessionLiveState struct {
	Participants []SessionParticipant `json:"participants,omitempty"`
	Composer     SessionComposerState `json:"composer"`
	Focus        SessionFocusState    `json:"focus"`
	Activity     SessionActivityState `json:"activity"`
}

type SessionParticipant struct {
	ParticipantID string `json:"participantId"`
	ClientType    string `json:"clientType,omitempty"`
	DisplayName   string `json:"displayName,omitempty"`
	Active        bool   `json:"active"`
	LastSeenAt    int64  `json:"lastSeenAt,omitempty"`
}

type SessionComposerState struct {
	DraftText string `json:"draftText,omitempty"`
	IsTyping  bool   `json:"isTyping"`
	UpdatedAt int64  `json:"updatedAt,omitempty"`
}

type SessionFocusState struct {
	ActiveFilePath string `json:"activeFilePath,omitempty"`
	Selection      string `json:"selection,omitempty"`
	PatchPath      string `json:"patchPath,omitempty"`
	RunErrorPath   string `json:"runErrorPath,omitempty"`
	RunErrorLine   int    `json:"runErrorLine,omitempty"`
	UpdatedAt      int64  `json:"updatedAt,omitempty"`
}

type SessionActivityState struct {
	Phase     string `json:"phase,omitempty"`
	Summary   string `json:"summary,omitempty"`
	UpdatedAt int64  `json:"updatedAt,omitempty"`
}

type SessionOperationState struct {
	CurrentJobID       string   `json:"currentJobId,omitempty"`
	Phase              string   `json:"phase,omitempty"`
	PatchSummary       string   `json:"patchSummary,omitempty"`
	PatchFileCount     int      `json:"patchFileCount,omitempty"`
	PatchResultStatus  string   `json:"patchResultStatus,omitempty"`
	PatchResultMessage string   `json:"patchResultMessage,omitempty"`
	RunProfileID       string   `json:"runProfileId,omitempty"`
	RunStatus          string   `json:"runStatus,omitempty"`
	RunSummary         string   `json:"runSummary,omitempty"`
	CurrentJobFiles    []string `json:"currentJobFiles,omitempty"`
	LastError          string   `json:"lastError,omitempty"`
}

type SessionStore struct {
	threadStore   *ThreadStore
	workspaceRoot string
	provider      string

	mu                    sync.RWMutex
	participantsBySession map[string]map[string]SessionParticipant
	composerBySession     map[string]SessionComposerState
	focusBySession        map[string]SessionFocusState
	activityBySession     map[string]SessionActivityState
}

func NewSessionStore(threadStore *ThreadStore, adapterInfo AdapterRuntimeInfo) *SessionStore {
	return &SessionStore{
		threadStore:           threadStore,
		workspaceRoot:         strings.TrimSpace(adapterInfo.WorkspaceRoot),
		provider:              inferProviderName(adapterInfo),
		participantsBySession: make(map[string]map[string]SessionParticipant),
		composerBySession:     make(map[string]SessionComposerState),
		focusBySession:        make(map[string]SessionFocusState),
		activityBySession:     make(map[string]SessionActivityState),
	}
}

func (s *SessionStore) CurrentSessionID() string {
	sessions := s.List()
	if len(sessions) == 0 {
		return ""
	}
	return sessions[0].ID
}

func (s *SessionStore) List() []SharedSessionSummary {
	if s == nil || s.threadStore == nil {
		return nil
	}

	threads := s.threadStore.List()
	sessions := make([]SharedSessionSummary, 0, len(threads))
	for _, thread := range threads {
		sessions = append(sessions, SharedSessionSummary{
			ID:               thread.ID,
			ThreadID:         thread.ID,
			ControlSessionID: thread.SessionID,
			Title:            thread.Title,
			Provider:         s.provider,
			WorkspaceRoot:    s.workspaceRoot,
			CurrentJobID:     thread.CurrentJobID,
			Phase:            phaseFromThreadSummary(thread),
			LastEventKind:    thread.LastEventKind,
			LastEventText:    thread.LastEventText,
			UpdatedAt:        thread.UpdatedAt,
		})
	}
	return sessions
}

func (s *SessionStore) Get(sessionID string) (SharedSessionDetail, bool) {
	if s == nil || s.threadStore == nil {
		return SharedSessionDetail{}, false
	}

	detail, ok := s.threadStore.Get(strings.TrimSpace(sessionID))
	if !ok {
		return SharedSessionDetail{}, false
	}

	operation := deriveSessionOperationState(detail)
	live := deriveSessionLiveState(detail, operation)
	live = s.applyLiveOverrides(detail.Thread.ID, live)

	return SharedSessionDetail{
		Session: SharedSessionSummary{
			ID:               detail.Thread.ID,
			ThreadID:         detail.Thread.ID,
			ControlSessionID: detail.Thread.SessionID,
			Title:            detail.Thread.Title,
			Provider:         s.provider,
			WorkspaceRoot:    s.workspaceRoot,
			CurrentJobID:     operation.CurrentJobID,
			Phase:            operation.Phase,
			LastEventKind:    detail.Thread.LastEventKind,
			LastEventText:    detail.Thread.LastEventText,
			UpdatedAt:        detail.Thread.UpdatedAt,
		},
		LiveState:      live,
		OperationState: operation,
		Timeline:       detail.Events,
	}, true
}

func (s *SessionStore) UpsertParticipant(sessionID string, participant SessionParticipant) {
	if s == nil {
		return
	}
	participant.ParticipantID = strings.TrimSpace(participant.ParticipantID)
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || participant.ParticipantID == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	participants := s.participantsBySession[sessionID]
	if participants == nil {
		participants = make(map[string]SessionParticipant)
		s.participantsBySession[sessionID] = participants
	}
	participants[participant.ParticipantID] = participant
}

func (s *SessionStore) UpdateComposer(sessionID string, composer SessionComposerState) {
	if s == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.composerBySession[sessionID] = composer
}

func (s *SessionStore) UpdateFocus(sessionID string, focus SessionFocusState) {
	if s == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.focusBySession[sessionID] = focus
}

func (s *SessionStore) applyLiveOverrides(sessionID string, base SessionLiveState) SessionLiveState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if participants := s.participantsBySession[sessionID]; len(participants) > 0 {
		base.Participants = participantsSlice(participants)
	}
	if composer, ok := s.composerBySession[sessionID]; ok {
		base.Composer = composer
	}
	if focus, ok := s.focusBySession[sessionID]; ok {
		base.Focus = focus
	}
	if activity, ok := s.activityBySession[sessionID]; ok {
		base.Activity = activity
	}
	return base
}

func participantsSlice(items map[string]SessionParticipant) []SessionParticipant {
	out := make([]SessionParticipant, 0, len(items))
	for _, participant := range items {
		out = append(out, participant)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LastSeenAt == out[j].LastSeenAt {
			return out[i].ParticipantID < out[j].ParticipantID
		}
		return out[i].LastSeenAt > out[j].LastSeenAt
	})
	return out
}

func mergeComposerState(base, override SessionComposerState) SessionComposerState {
	if strings.TrimSpace(override.DraftText) != "" {
		base.DraftText = override.DraftText
	}
	if override.IsTyping {
		base.IsTyping = true
	}
	if override.UpdatedAt != 0 {
		base.UpdatedAt = override.UpdatedAt
	}
	return base
}

func mergeFocusState(base, override SessionFocusState) SessionFocusState {
	if strings.TrimSpace(override.ActiveFilePath) != "" {
		base.ActiveFilePath = override.ActiveFilePath
	}
	if strings.TrimSpace(override.Selection) != "" {
		base.Selection = override.Selection
	}
	if strings.TrimSpace(override.PatchPath) != "" {
		base.PatchPath = override.PatchPath
	}
	if strings.TrimSpace(override.RunErrorPath) != "" {
		base.RunErrorPath = override.RunErrorPath
	}
	if override.RunErrorLine != 0 {
		base.RunErrorLine = override.RunErrorLine
	}
	if override.UpdatedAt != 0 {
		base.UpdatedAt = override.UpdatedAt
	}
	return base
}

func deriveSessionOperationState(detail ThreadDetail) SessionOperationState {
	operation := SessionOperationState{
		CurrentJobID: detail.Thread.CurrentJobID,
		Phase:        phaseFromThreadSummary(detail.Thread),
	}

	for _, event := range detail.Events {
		if event.JobID != "" {
			operation.CurrentJobID = event.JobID
		}

		switch event.Kind {
		case "prompt_submitted", "prompt_accepted":
			operation.Phase = "prompting"
		case "patch_ready":
			operation.Phase = "reviewing"
			operation.PatchSummary = firstNonEmptyText(stringValue(event.Data["summary"]), event.Body)
			operation.CurrentJobFiles = patchPathsFromAny(event.Data["files"])
			operation.PatchFileCount = len(operation.CurrentJobFiles)
		case "patch_apply_requested":
			operation.Phase = "applying"
		case "patch_applied":
			operation.PatchResultStatus = valueFromData(event.Data, "status", operation.PatchResultStatus)
			operation.PatchResultMessage = firstNonEmptyText(valueFromData(event.Data, "message", ""), event.Body)
			if strings.EqualFold(operation.PatchResultStatus, "failed") {
				operation.Phase = "waiting_input"
				operation.LastError = firstNonEmptyText(operation.PatchResultMessage, operation.LastError)
			} else {
				operation.Phase = "applied"
			}
		case "run_requested":
			operation.Phase = "running"
			operation.RunProfileID = valueFromData(event.Data, "profileId", operation.RunProfileID)
		case "run_finished":
			operation.Phase = "waiting_input"
			operation.RunProfileID = valueFromData(event.Data, "profileId", operation.RunProfileID)
			operation.RunStatus = valueFromData(event.Data, "status", operation.RunStatus)
			operation.RunSummary = firstNonEmptyText(valueFromData(event.Data, "summary", ""), event.Body)
			changedFiles := stringSliceFromAny(event.Data["changedFiles"])
			if len(changedFiles) > 0 {
				operation.CurrentJobFiles = changedFiles
			}
			if strings.EqualFold(operation.RunStatus, "failed") {
				operation.LastError = firstNonEmptyText(operation.RunSummary, firstParsedErrorMessage(event.Data["topErrors"]))
			}
		}
	}

	if operation.Phase == "" {
		operation.Phase = "idle"
	}
	return operation
}

func deriveSessionLiveState(detail ThreadDetail, operation SessionOperationState) SessionLiveState {
	live := SessionLiveState{}
	for _, event := range detail.Events {
		switch event.Kind {
		case "prompt_submitted":
			live.Composer = SessionComposerState{
				DraftText: event.Body,
				UpdatedAt: event.At,
			}
			live.Focus.ActiveFilePath = stringValue(event.Data["activeFilePath"])
			live.Focus.Selection = stringValue(event.Data["selection"])
			live.Focus.UpdatedAt = event.At
		case "patch_ready":
			paths := patchPathsFromAny(event.Data["files"])
			if len(paths) > 0 {
				live.Focus.PatchPath = paths[0]
				live.Focus.UpdatedAt = event.At
			}
		case "run_finished":
			path, line, _ := firstParsedError(event.Data["topErrors"])
			if path != "" {
				live.Focus.RunErrorPath = path
				live.Focus.RunErrorLine = line
				live.Focus.UpdatedAt = event.At
			}
		}
	}

	if len(detail.Events) > 0 {
		last := detail.Events[len(detail.Events)-1]
		live.Activity = SessionActivityState{
			Phase:     operation.Phase,
			Summary:   firstNonEmptyText(last.Body, last.Title),
			UpdatedAt: last.At,
		}
	} else {
		live.Activity.Phase = operation.Phase
	}

	return live
}

func phaseFromThreadSummary(thread ThreadSummary) string {
	if thread.LastEventKind == "patch_apply_requested" {
		return "applying"
	}
	if thread.LastEventKind == "run_requested" {
		return "running"
	}

	switch strings.ToLower(strings.TrimSpace(thread.State)) {
	case "", "draft":
		return "idle"
	case "prompt_submitted":
		return "prompting"
	case "patch_ready":
		return "reviewing"
	case "applied", "success", "partial", "conflict":
		return "applied"
	case "completed", "failed":
		return "waiting_input"
	default:
		switch thread.LastEventKind {
		case "prompt_submitted", "prompt_accepted":
			return "prompting"
		case "patch_ready":
			return "reviewing"
		case "patch_applied":
			return "applied"
		case "run_finished":
			return "waiting_input"
		default:
			return "idle"
		}
	}
}

func patchPathsFromAny(value any) []string {
	switch typed := value.(type) {
	case []protocol.FilePatch:
		return patchFilePaths(typed)
	case []any:
		paths := make([]string, 0, len(typed))
		seen := make(map[string]struct{}, len(typed))
		for _, item := range typed {
			path := ""
			switch file := item.(type) {
			case map[string]any:
				path = stringValue(file["path"])
			case protocol.FilePatch:
				path = strings.TrimSpace(file.Path)
			}
			if path == "" {
				continue
			}
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			paths = append(paths, path)
		}
		return paths
	default:
		return nil
	}
}

func stringSliceFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text := stringValue(item)
			if text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func firstParsedErrorMessage(value any) string {
	_, _, message := firstParsedError(value)
	return message
}

func firstParsedError(value any) (string, int, string) {
	switch typed := value.(type) {
	case []protocol.ParsedError:
		for _, item := range typed {
			message := strings.TrimSpace(item.Message)
			path := strings.TrimSpace(item.Path)
			if message == "" && path == "" && item.Line == 0 {
				continue
			}
			return path, item.Line, message
		}
	case []any:
		for _, raw := range typed {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			message := stringValue(item["message"])
			path := stringValue(item["path"])
			line := intValue(item["line"])
			if message == "" && path == "" && line == 0 {
				continue
			}
			return path, line, message
		}
	}
	return "", 0, ""
}

func stringValue(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}
