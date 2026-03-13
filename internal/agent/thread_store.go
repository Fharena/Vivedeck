package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type ThreadSummary struct {
	ID            string `json:"id"`
	SessionID     string `json:"sessionId,omitempty"`
	Title         string `json:"title"`
	State         string `json:"state,omitempty"`
	CurrentJobID  string `json:"currentJobId,omitempty"`
	LastEventKind string `json:"lastEventKind,omitempty"`
	LastEventText string `json:"lastEventText,omitempty"`
	UpdatedAt     int64  `json:"updatedAt"`
}

type ThreadEvent struct {
	ID       string         `json:"id"`
	ThreadID string         `json:"threadId"`
	JobID    string         `json:"jobId,omitempty"`
	Kind     string         `json:"kind"`
	Role     string         `json:"role"`
	Title    string         `json:"title"`
	Body     string         `json:"body,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
	At       int64          `json:"at"`
}

type ThreadDetail struct {
	Thread ThreadSummary `json:"thread"`
	Events []ThreadEvent `json:"events"`
}

type ThreadStore struct {
	mu              sync.RWMutex
	threads         map[string]*threadRecord
	jobToThread     map[string]string
	persistencePath string
}

type threadRecord struct {
	summary ThreadSummary
	events  []ThreadEvent
}

type threadStoreSnapshot struct {
	Version int            `json:"version"`
	SavedAt int64          `json:"savedAt"`
	Threads []ThreadDetail `json:"threads"`
}

func NewThreadStore() *ThreadStore {
	return &ThreadStore{
		threads:     make(map[string]*threadRecord),
		jobToThread: make(map[string]string),
	}
}

func NewPersistentThreadStore(path string) (*ThreadStore, error) {
	store := NewThreadStore()
	store.persistencePath = strings.TrimSpace(path)
	if store.persistencePath == "" {
		return store, nil
	}
	if err := store.loadFromDisk(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *ThreadStore) PersistencePath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.persistencePath
}

func (s *ThreadStore) EnsureThread(threadID, sessionID, title string) ThreadSummary {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC().UnixMilli()
	normalizedTitle := normalizeThreadTitle(title)
	if threadID == "" {
		threadID = fmt.Sprintf("thread_%d", time.Now().UTC().UnixNano())
	}

	record, ok := s.threads[threadID]
	if !ok {
		summary := ThreadSummary{
			ID:        threadID,
			SessionID: strings.TrimSpace(sessionID),
			Title:     normalizedTitle,
			State:     "draft",
			UpdatedAt: now,
		}
		record = &threadRecord{summary: summary, events: make([]ThreadEvent, 0, 16)}
		s.threads[threadID] = record
		s.persistLockedBestEffort()
		return record.summary
	}

	if record.summary.Title == "새 스레드" && normalizedTitle != "" {
		record.summary.Title = normalizedTitle
	}
	if record.summary.SessionID == "" && strings.TrimSpace(sessionID) != "" {
		record.summary.SessionID = strings.TrimSpace(sessionID)
	}
	record.summary.UpdatedAt = now
	s.persistLockedBestEffort()
	return record.summary
}

func (s *ThreadStore) AppendEvent(threadID string, event ThreadEvent) (ThreadSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.threads[threadID]
	if !ok {
		return ThreadSummary{}, fmt.Errorf("thread %s not found", threadID)
	}

	if event.ID != "" {
		for _, existing := range record.events {
			if existing.ID == event.ID {
				return record.summary, nil
			}
		}
	}
	if event.ID == "" {
		event.ID = fmt.Sprintf("evt_%d", time.Now().UTC().UnixNano())
	}
	if event.ThreadID == "" {
		event.ThreadID = threadID
	}
	if event.At == 0 {
		event.At = time.Now().UTC().UnixMilli()
	}
	if event.Title == "" {
		event.Title = humanizeEventKind(event.Kind)
	}

	record.events = append(record.events, cloneThreadEvent(event))
	record.summary.UpdatedAt = event.At
	record.summary.LastEventKind = event.Kind
	record.summary.LastEventText = firstNonEmptyText(event.Body, event.Title)
	if event.JobID != "" {
		record.summary.CurrentJobID = event.JobID
		s.jobToThread[event.JobID] = threadID
	}
	switch event.Kind {
	case "prompt_submitted":
		record.summary.State = "prompt_submitted"
	case "patch_ready":
		record.summary.State = "patch_ready"
	case "patch_applied":
		record.summary.State = valueFromData(event.Data, "status", "applied")
	case "run_finished":
		record.summary.State = valueFromData(event.Data, "status", "completed")
	}
	s.persistLockedBestEffort()
	return record.summary, nil
}

func (s *ThreadStore) AssignJob(threadID, jobID string) {
	if threadID == "" || jobID == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if record, ok := s.threads[threadID]; ok {
		record.summary.CurrentJobID = jobID
	}
	s.jobToThread[jobID] = threadID
	s.persistLockedBestEffort()
}

func (s *ThreadStore) ThreadIDForJob(jobID string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	threadID, ok := s.jobToThread[jobID]
	return threadID, ok
}

func (s *ThreadStore) List() []ThreadSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]ThreadSummary, 0, len(s.threads))
	for _, record := range s.threads {
		items = append(items, record.summary)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt == items[j].UpdatedAt {
			return items[i].ID > items[j].ID
		}
		return items[i].UpdatedAt > items[j].UpdatedAt
	})
	return items
}

func (s *ThreadStore) Get(threadID string) (ThreadDetail, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record, ok := s.threads[threadID]
	if !ok {
		return ThreadDetail{}, false
	}

	detail := ThreadDetail{
		Thread: record.summary,
		Events: make([]ThreadEvent, 0, len(record.events)),
	}
	for _, event := range record.events {
		detail.Events = append(detail.Events, cloneThreadEvent(event))
	}
	return detail, true
}

func (s *ThreadStore) loadFromDisk() error {
	if s.persistencePath == "" {
		return nil
	}

	data, err := os.ReadFile(s.persistencePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read thread store snapshot: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}

	var snapshot threadStoreSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return fmt.Errorf("decode thread store snapshot: %w", err)
	}

	s.threads = make(map[string]*threadRecord, len(snapshot.Threads))
	s.jobToThread = make(map[string]string)
	for _, detail := range snapshot.Threads {
		events := make([]ThreadEvent, 0, len(detail.Events))
		for _, event := range detail.Events {
			events = append(events, cloneThreadEvent(event))
			if event.JobID != "" {
				s.jobToThread[event.JobID] = detail.Thread.ID
			}
		}
		if detail.Thread.CurrentJobID != "" {
			s.jobToThread[detail.Thread.CurrentJobID] = detail.Thread.ID
		}
		s.threads[detail.Thread.ID] = &threadRecord{
			summary: detail.Thread,
			events:  events,
		}
	}
	return nil
}

func (s *ThreadStore) persistLockedBestEffort() {
	if s.persistencePath == "" {
		return
	}
	if err := s.persistLocked(); err != nil {
		log.Printf("thread store persist failed: %v", err)
	}
}

func (s *ThreadStore) persistLocked() error {
	snapshot := s.snapshotLocked()
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal thread store snapshot: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.persistencePath), 0o755); err != nil {
		return fmt.Errorf("mkdir thread store dir: %w", err)
	}
	tmpPath := s.persistencePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write thread store tmp snapshot: %w", err)
	}
	_ = os.Remove(s.persistencePath)
	if err := os.Rename(tmpPath, s.persistencePath); err != nil {
		return fmt.Errorf("replace thread store snapshot: %w", err)
	}
	return nil
}

func (s *ThreadStore) snapshotLocked() threadStoreSnapshot {
	threadIDs := make([]string, 0, len(s.threads))
	for threadID := range s.threads {
		threadIDs = append(threadIDs, threadID)
	}
	sort.Strings(threadIDs)

	threads := make([]ThreadDetail, 0, len(threadIDs))
	for _, threadID := range threadIDs {
		record := s.threads[threadID]
		detail := ThreadDetail{
			Thread: record.summary,
			Events: make([]ThreadEvent, 0, len(record.events)),
		}
		for _, event := range record.events {
			detail.Events = append(detail.Events, cloneThreadEvent(event))
		}
		threads = append(threads, detail)
	}

	return threadStoreSnapshot{
		Version: 1,
		SavedAt: time.Now().UTC().UnixMilli(),
		Threads: threads,
	}
}

func cloneThreadEvent(event ThreadEvent) ThreadEvent {
	cloned := event
	if event.Data != nil {
		cloned.Data = make(map[string]any, len(event.Data))
		for key, value := range event.Data {
			cloned.Data[key] = value
		}
	}
	return cloned
}

func normalizeThreadTitle(title string) string {
	title = strings.TrimSpace(firstLine(title))
	if title == "" {
		return "새 스레드"
	}
	if len(title) > 72 {
		return title[:72]
	}
	return title
}

func firstLine(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func firstNonEmptyText(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			if len(value) > 160 {
				return value[:160]
			}
			return value
		}
	}
	return ""
}

func humanizeEventKind(kind string) string {
	switch kind {
	case "prompt_submitted":
		return "프롬프트 제출"
	case "prompt_accepted":
		return "작업 시작"
	case "patch_ready":
		return "패치 준비 완료"
	case "patch_apply_requested":
		return "패치 적용 요청"
	case "patch_applied":
		return "패치 적용 결과"
	case "run_requested":
		return "실행 요청"
	case "run_finished":
		return "실행 결과"
	default:
		return kind
	}
}

func valueFromData(data map[string]any, key, fallback string) string {
	if data == nil {
		return fallback
	}
	if value, ok := data[key]; ok {
		if text, ok := value.(string); ok {
			text = strings.TrimSpace(text)
			if text != "" {
				return text
			}
		}
	}
	return fallback
}
