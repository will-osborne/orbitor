package main

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// FileChange records a single file's before/after content within a run.
type FileChange struct {
	Path         string `json:"path"`
	RelativePath string `json:"relativePath"`
	Before       string `json:"before"`
	After        string `json:"after"`
}

// RunRecord captures all file changes that occurred during one prompt→run_complete cycle.
type RunRecord struct {
	ID          string       `json:"id"`
	Prompt      string       `json:"prompt"`
	StartedAt   time.Time    `json:"startedAt"`
	CompletedAt *time.Time   `json:"completedAt,omitempty"`
	Files       []FileChange `json:"files"`
}

// RunHistory tracks per-session run records in memory.
type RunHistory struct {
	mu      sync.Mutex
	records []*RunRecord
	current *RunRecord
}

func newRunHistory() *RunHistory {
	return &RunHistory{}
}

// StartRun begins a new run record for the given prompt text.
func (h *RunHistory) StartRun(prompt string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.current = &RunRecord{
		ID:        uuid.New().String()[:8],
		Prompt:    prompt,
		StartedAt: time.Now(),
		Files:     []FileChange{},
	}
}

// RecordFileChange adds or updates a file entry in the current run.
// before is the file content before the write; after is the content after.
// If before == after (no actual change) the entry is skipped.
func (h *RunHistory) RecordFileChange(path, workingDir, before, after string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.current == nil || before == after {
		return
	}
	rel := path
	if workingDir != "" {
		if r, err := filepath.Rel(workingDir, path); err == nil {
			rel = r
		}
	}
	// If the same file was written multiple times in this run, update the after content.
	for i, f := range h.current.Files {
		if f.Path == path {
			h.current.Files[i].After = after
			return
		}
	}
	h.current.Files = append(h.current.Files, FileChange{
		Path:         path,
		RelativePath: rel,
		Before:       before,
		After:        after,
	})
}

// CompleteRun finalises the current run record and appends it to history.
// Only saved if at least one file was changed.
func (h *RunHistory) CompleteRun() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.current == nil {
		return
	}
	now := time.Now()
	h.current.CompletedAt = &now
	if len(h.current.Files) > 0 {
		h.records = append(h.records, h.current)
	}
	h.current = nil
}

// Records returns a snapshot of all completed run records, newest first.
func (h *RunHistory) Records() []RunRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]RunRecord, len(h.records))
	for i := range h.records {
		out[i] = *h.records[len(h.records)-1-i]
	}
	return out
}
