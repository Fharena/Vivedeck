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
