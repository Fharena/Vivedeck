package agent

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type SessionLiveUpdateRequest struct {
	Participant *SessionParticipant    `json:"participant,omitempty"`
	Composer    *SessionComposerState  `json:"composer,omitempty"`
	Focus       *SessionFocusState     `json:"focus,omitempty"`
	Activity    *SessionActivityState  `json:"activity,omitempty"`
	Reasoning   *SessionReasoningState `json:"reasoning,omitempty"`
	Plan        *SessionPlanState      `json:"plan,omitempty"`
	Tools       *SessionToolState      `json:"tools,omitempty"`
	Terminal    *SessionTerminalState  `json:"terminal,omitempty"`
	Workspace   *SessionWorkspaceState `json:"workspace,omitempty"`
}

type SessionTimelineAppendEvent struct {
	ID    string         `json:"id,omitempty"`
	JobID string         `json:"jobId,omitempty"`
	Kind  string         `json:"kind"`
	Role  string         `json:"role,omitempty"`
	Title string         `json:"title,omitempty"`
	Body  string         `json:"body,omitempty"`
	Data  map[string]any `json:"data,omitempty"`
	At    int64          `json:"at,omitempty"`
}

type SessionTimelineAppendRequest struct {
	Events []SessionTimelineAppendEvent `json:"events"`
}

func (s *HTTPServer) handleSessionPath(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/v1/agent/sessions/"))
	path = strings.Trim(path, "/")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session id is required"})
		return
	}

	parts := strings.Split(path, "/")
	sessionID := strings.TrimSpace(parts[0])
	if sessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session id is required"})
		return
	}

	if len(parts) == 1 {
		s.handleSessionDetailRequest(w, r, sessionID)
		return
	}

	if len(parts) == 2 {
		switch parts[1] {
		case "stream":
			s.handleSessionStream(w, r, sessionID)
			return
		case "live":
			s.handleSessionLiveUpdate(w, r, sessionID)
			return
		case "events":
			s.handleSessionTimelineAppend(w, r, sessionID)
			return
		}
	}

	writeJSON(w, http.StatusNotFound, map[string]string{"error": "session path not found"})
}

func (s *HTTPServer) handleSessionDetailRequest(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	detail, ok := s.sessionDetail(sessionID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *HTTPServer) handleSessionLiveUpdate(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	store := s.sessionStore()
	if store == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}
	if _, ok := store.Get(sessionID); !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}

	var req SessionLiveUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid session live update request"})
		return
	}

	if req.Participant != nil {
		store.UpsertParticipant(sessionID, *req.Participant)
	}
	if req.Composer != nil {
		store.UpdateComposer(sessionID, *req.Composer)
	}
	if req.Focus != nil {
		store.UpdateFocus(sessionID, *req.Focus)
	}
	if req.Activity != nil {
		store.UpdateActivity(sessionID, *req.Activity)
	}
	if req.Reasoning != nil {
		store.UpdateReasoning(sessionID, *req.Reasoning)
	}
	if req.Plan != nil {
		store.UpdatePlan(sessionID, *req.Plan)
	}
	if req.Tools != nil {
		store.UpdateTools(sessionID, *req.Tools)
	}
	if req.Terminal != nil {
		store.UpdateTerminal(sessionID, *req.Terminal)
	}
	if req.Workspace != nil {
		store.UpdateWorkspace(sessionID, *req.Workspace)
	}

	detail, ok := store.Get(sessionID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *HTTPServer) handleSessionTimelineAppend(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	store := s.sessionStore()
	if store == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}
	if _, ok := store.Get(sessionID); !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}
	if s.orchestrator == nil || s.orchestrator.ThreadStore() == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session thread store not found"})
		return
	}

	var req SessionTimelineAppendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid session timeline append request"})
		return
	}
	if len(req.Events) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "at least one event is required"})
		return
	}

	threadStore := s.orchestrator.ThreadStore()
	for _, item := range req.Events {
		kind := strings.TrimSpace(item.Kind)
		if kind == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "event kind is required"})
			return
		}
		role := strings.TrimSpace(item.Role)
		if role == "" {
			role = "system"
		}
		if _, err := threadStore.AppendEvent(sessionID, ThreadEvent{
			ID:       strings.TrimSpace(item.ID),
			ThreadID: sessionID,
			JobID:    strings.TrimSpace(item.JobID),
			Kind:     kind,
			Role:     role,
			Title:    strings.TrimSpace(item.Title),
			Body:     strings.TrimSpace(item.Body),
			Data:     item.Data,
			At:       item.At,
		}); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}

	detail, ok := store.Get(sessionID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *HTTPServer) handleSessionStream(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	detail, ok := s.sessionDetail(sessionID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming is not supported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	lastPayload := sessionDetailPayload(detail)
	if !writeSessionStreamEvent(w, flusher, lastPayload) {
		return
	}

	pollTicker := time.NewTicker(350 * time.Millisecond)
	defer pollTicker.Stop()

	heartbeatTicker := time.NewTicker(15 * time.Second)
	defer heartbeatTicker.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-pollTicker.C:
			nextDetail, exists := s.sessionDetail(sessionID)
			if !exists {
				return
			}
			nextPayload := sessionDetailPayload(nextDetail)
			if bytes.Equal(nextPayload, lastPayload) {
				continue
			}
			lastPayload = nextPayload
			if !writeSessionStreamEvent(w, flusher, nextPayload) {
				return
			}
		case <-heartbeatTicker.C:
			if _, err := w.Write([]byte(": keepalive\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *HTTPServer) sessionStore() *SessionStore {
	if s.orchestrator == nil {
		return nil
	}
	return s.orchestrator.SessionStore()
}

func (s *HTTPServer) sessionDetail(sessionID string) (SharedSessionDetail, bool) {
	store := s.sessionStore()
	if store == nil {
		return SharedSessionDetail{}, false
	}
	return store.Get(sessionID)
}

func sessionDetailPayload(detail SharedSessionDetail) []byte {
	payload, err := json.Marshal(detail)
	if err != nil {
		return []byte("{}")
	}
	return payload
}

func writeSessionStreamEvent(w http.ResponseWriter, flusher http.Flusher, payload []byte) bool {
	if _, err := w.Write([]byte("event: session\n")); err != nil {
		return false
	}
	if _, err := w.Write([]byte("data: ")); err != nil {
		return false
	}
	if _, err := w.Write(payload); err != nil {
		return false
	}
	if _, err := w.Write([]byte("\n\n")); err != nil {
		return false
	}
	flusher.Flush()
	return true
}
