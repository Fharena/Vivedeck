package agent

import "strings"

func (s *SessionStore) UpdateActivity(sessionID string, activity SessionActivityState) {
	if s == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.activityBySession[sessionID] = activity
}

func (s *SessionStore) UpdateReasoning(sessionID string, reasoning SessionReasoningState) {
	if s == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.reasoningBySession[sessionID] = reasoning
}

func (s *SessionStore) UpdatePlan(sessionID string, plan SessionPlanState) {
	if s == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.planBySession[sessionID] = plan
}

func (s *SessionStore) UpdateTools(sessionID string, tools SessionToolState) {
	if s == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.toolsBySession[sessionID] = tools
}

func (s *SessionStore) UpdateTerminal(sessionID string, terminal SessionTerminalState) {
	if s == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.terminalBySession[sessionID] = terminal
}

func (s *SessionStore) UpdateWorkspace(sessionID string, workspace SessionWorkspaceState) {
	if s == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.workspaceBySession[sessionID] = workspace
}
