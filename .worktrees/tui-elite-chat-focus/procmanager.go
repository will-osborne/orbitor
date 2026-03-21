package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

// ProcManager is a small local process supervisor exposing a JSON HTTP API.
// It records process metadata to the Store so other components can discover them.
type ProcManager struct {
	store     *Store
	mu        sync.Mutex
	processes map[int64]*ManagedProcess // keyed by DB row id
}

type ManagedProcess struct {
	ID        int64
	SessionID string
	Cmd       *exec.Cmd
	StartedAt time.Time
	Exited    bool
	ExitCode  int
}

func NewProcManager(store *Store) *ProcManager {
	return &ProcManager{store: store, processes: make(map[int64]*ManagedProcess)}
}

// StartServer starts an HTTP server on addr to accept spawn/status/stop requests.
func (pm *ProcManager) StartServer(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/proc/spawn", pm.handleSpawn)
	mux.HandleFunc("/proc/status", pm.handleStatus)
	mux.HandleFunc("/proc/stop", pm.handleStop)
	log.Printf("procmanager listening on %s", addr)
	return http.ListenAndServe(addr, mux)
}

func (pm *ProcManager) handleSpawn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		SessionID string   `json:"sessionId"`
		Cmd       string   `json:"cmd"`
		Args      []string `json:"args"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	cmd := exec.Command(req.Cmd, req.Args...)
	if err := cmd.Start(); err != nil {
		log.Printf("procmanager: spawn error: %v", err)
		http.Error(w, fmt.Sprintf("spawn error: %v", err), http.StatusInternalServerError)
		return
	}
	pid := cmd.Process.Pid
	procID, err := pm.store.RecordProcessStart(req.SessionID, pid, req.Cmd, fmt.Sprintf("%v", req.Args))
	if err != nil {
		log.Printf("procmanager: record process: %v", err)
	}

	mp := &ManagedProcess{ID: procID, SessionID: req.SessionID, Cmd: cmd, StartedAt: time.Now()}
	pm.mu.Lock()
	pm.processes[procID] = mp
	pm.mu.Unlock()

	// Monitor in background
	go func(id int64, c *exec.Cmd) {
		err := c.Wait()
		exitCode := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				exitCode = ee.ExitCode()
			} else {
				exitCode = -1
			}
		}
		_ = pm.store.UpdateProcessExit(id, exitCode)
		pm.mu.Lock()
		if mp, ok := pm.processes[id]; ok {
			mp.Exited = true
			mp.ExitCode = exitCode
		}
		pm.mu.Unlock()
	}(procID, cmd)

	json.NewEncoder(w).Encode(map[string]any{"processId": procID, "pid": pid})
}

func (pm *ProcManager) handleStatus(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("id")
	if q == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	id, err := strconv.ParseInt(q, 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	pm.mu.Lock()
	mp, ok := pm.processes[id]
	pm.mu.Unlock()
	if ok {
		json.NewEncoder(w).Encode(map[string]any{
			"id":        id,
			"sessionId": mp.SessionID,
			"pid":       mp.Cmd.Process.Pid,
			"startedAt": mp.StartedAt,
			"exited":    mp.Exited,
			"exitCode":  mp.ExitCode,
		})
		return
	}
	// fallback to DB
	if proc, err := pm.store.GetProcess(id); err == nil {
		json.NewEncoder(w).Encode(proc)
		return
	}
	http.Error(w, "not found", http.StatusNotFound)
}

func (pm *ProcManager) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ProcessID int64 `json:"processId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	pm.mu.Lock()
	mp, ok := pm.processes[req.ProcessID]
	pm.mu.Unlock()
	if !ok || mp.Cmd == nil || mp.Cmd.Process == nil {
		http.Error(w, "process not found", http.StatusNotFound)
		return
	}
	if err := mp.Cmd.Process.Kill(); err != nil {
		http.Error(w, fmt.Sprintf("kill error: %v", err), http.StatusInternalServerError)
		return
	}
	_ = pm.store.UpdateProcessExit(req.ProcessID, -1)
	w.WriteHeader(http.StatusNoContent)
}
