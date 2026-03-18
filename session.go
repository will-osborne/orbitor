package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
)

type Session struct {
	ID              string
	WorkingDir      string
	Backend         string    // "copilot" or "claude"
	Model           string    // model to use (e.g. "claude-sonnet-4.5", "gpt-5")
	SkipPermissions bool      // pass --yolo (copilot) or --dangerously-skip-permissions (claude)
	ACPSession      string    // ACP session ID returned by agent
	Status          string    // "starting", "ready", "error", "closed"
	CreatedAt       time.Time // when the session was first created

	port     int
	procID   int64
	process  *exec.Cmd
	acp      *ACPClient
	hub      *Hub
	queue    *PromptQueue
	terminal *TerminalManager

	permMu      sync.Mutex
	permPending map[string]chan string

	interruptMu     sync.Mutex
	interruptCancel context.CancelFunc // set while a prompt is in flight

	summaryMu     sync.RWMutex
	lastMessage   string // last agent text snippet
	currentTool   string // current tool being used
	currentPrompt string // text of the prompt currently being processed
	isRunning     bool   // true while a prompt is being processed
	title         string // LLM-generated session title (set after run completes)
	summary       string // LLM-generated one-sentence summary

	// agentTextCh batches frequent agent_text chunks to reduce broadcast frequency
	agentTextCh    chan string
	agentTextDone  chan struct{}
	agentTextFlush chan chan struct{} // request a synchronous flush of the text buffer

	store      *Store      // persistence hook
	allocPort  func() int  // returns a fresh port number (from SessionManager)
	eventHub   *Hub        // global event hub for cross-session notifications
	summarizer *Summarizer // optional LLM summarizer (nil = disabled)
}

func (s *Session) persistSession() {
	if s.store == nil {
		return
	}
	_ = s.store.UpsertSession(s)
}

type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	store    *Store
	// EventHub broadcasts global events (e.g. permission requests) to
	// connected /ws/events clients.
	EventHub   *Hub
	summarizer *Summarizer // optional LLM summarizer for session titles/summaries
}

// AllocPort finds a free TCP port by briefly listening on :0 and returning
// the OS-assigned port. This avoids collisions with orphaned copilot
// processes from previous server runs that may still hold old ports.
func (sm *SessionManager) AllocPort() int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Printf("AllocPort: listen failed: %v, falling back to random", err)
		return 19200 + int(time.Now().UnixNano()%1000)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func NewSessionManager(store *Store, summarizer *Summarizer) *SessionManager {
	eventHub := NewHub()
	// Global notification stream should be live-only; replay is handled via
	// /api/notifications polling to avoid duplicate local notifications.
	eventHub.maxHistory = 0
	go eventHub.Run()
	sm := &SessionManager{
		sessions:   make(map[string]*Session),
		store:      store,
		EventHub:   eventHub,
		summarizer: summarizer,
	}

	// Load persisted sessions so the server can serve WS history immediately.
	// Actual backend respawning is deferred to RespawnSessions() so the caller
	// can wait for the previous process to exit first (important during upgrades).
	if store != nil {
		if recs, err := store.LoadSessions(); err == nil {
			for _, r := range recs {
				s := &Session{
					ID:              r.ID,
					WorkingDir:      r.WorkingDir,
					Backend:         r.Backend,
					Model:           r.Model,
					SkipPermissions: r.SkipPermissions,
					ACPSession:      r.ACPSession,
					Status:          r.Status,
					CreatedAt:       r.CreatedAt,
					port:            r.Port,
					procID:          r.ProcID,
					lastMessage:     r.LastMessage,
					currentTool:     r.CurrentTool,
					title:           r.Title,
					summary:         r.Summary,
					hub:             NewHub(),
					terminal:        NewTerminalManager(),
					permPending:     make(map[string]chan string),
					store:           store,
					allocPort:       sm.AllocPort,
					eventHub:        eventHub,
					summarizer:      summarizer,
				}

				// Seed hub history from DB before starting Run to avoid race conditions.
				// Only load up to maxHistory recent messages to avoid reading huge datasets.
				if msgs, err := store.LoadRecentMessages(s.ID, s.hub.maxHistory); err == nil && len(msgs) > 0 {
					s.hub.SeedHistory(msgs)
				}

				// Start the hub for WS clients immediately.
				go s.hub.Run()

				sm.sessions[s.ID] = s
			}
		}
	}

	return sm
}

// RespawnSessions spawns backends for all sessions that were suspended or active.
// Call this AFTER the previous process has exited to avoid port conflicts.
func (sm *SessionManager) RespawnSessions() {
	sm.mu.RLock()
	var toSpawn []*Session
	for _, s := range sm.sessions {
		if s.Status != "closed" {
			toSpawn = append(toSpawn, s)
		}
	}
	sm.mu.RUnlock()

	for _, s := range toSpawn {
		s.Status = "starting"

		// Always allocate a fresh port for copilot sessions. The old port
		// may still be held by an orphaned copilot process from a previous
		// server run (s.process isn't persisted, so we can't kill it).
		if s.Backend == "copilot" {
			s.port = sm.AllocPort()
		}

		if s.store != nil {
			_ = s.store.UpsertSession(s)
		}
		log.Printf("session %s: reinitializing (backend=%s, port=%d)", s.ID, s.Backend, s.port)

		go func(ss *Session) {
			switch ss.Backend {
			case "copilot":
				ss.startCopilot(ss.WorkingDir, ss.port)
			case "claude":
				ss.startClaude(ss.WorkingDir)
			default:
				log.Printf("session %s: unknown backend '%s'", ss.ID, ss.Backend)
			}
		}(s)
	}
}

func (sm *SessionManager) List() []WSSessionInfo {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	var out []WSSessionInfo
	for _, s := range sm.sessions {
		s.summaryMu.RLock()
		lastMsg := s.lastMessage
		curTool := s.currentTool
		curPrompt := s.currentPrompt
		running := s.isRunning
		title := s.title
		summary := s.summary
		s.summaryMu.RUnlock()

		s.permMu.Lock()
		hasPerm := len(s.permPending) > 0
		s.permMu.Unlock()

		out = append(out, WSSessionInfo{
			ID:                s.ID,
			WorkingDir:        s.WorkingDir,
			ACPSession:        s.ACPSession,
			Status:            s.Status,
			Backend:           s.Backend,
			Model:             s.Model,
			SkipPermissions:   s.SkipPermissions,
			LastMessage:       lastMsg,
			CurrentTool:       curTool,
			CurrentPrompt:     curPrompt,
			IsRunning:         running,
			PendingPermission: hasPerm,
			CreatedAt:         s.CreatedAt,
			Title:             title,
			Summary:           summary,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func (sm *SessionManager) Get(id string) *Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.sessions[id]
}

func (sm *SessionManager) Create(workingDir, backend, model string, skipPermissions bool) (*Session, error) {
	if backend == "" {
		backend = "copilot"
	}
	if backend != "copilot" && backend != "claude" {
		return nil, fmt.Errorf("unknown backend: %s (must be copilot or claude)", backend)
	}

	port := sm.AllocPort()

	id := uuid.New().String()[:8]

	s := &Session{
		ID:              id,
		WorkingDir:      workingDir,
		Backend:         backend,
		Model:           model,
		SkipPermissions: skipPermissions,
		Status:          "starting",
		CreatedAt:       time.Now(),
		port:            port,
		hub:             NewHub(),
		terminal:        NewTerminalManager(),
		permPending:     make(map[string]chan string),
		store:           sm.store,
		allocPort:       sm.AllocPort,
		eventHub:        sm.EventHub,
		summarizer:      sm.summarizer,
	}

	go s.hub.Run()

	switch backend {
	case "copilot":
		go s.startCopilot(workingDir, port)
	case "claude":
		go s.startClaude(workingDir)
	}

	sm.mu.Lock()
	sm.sessions[id] = s
	sm.mu.Unlock()

	if sm.store != nil {
		_ = sm.store.UpsertSession(s)
	}

	return s, nil
}

// startCopilot spawns `copilot --acp --port <port>` and connects via TCP.
func (s *Session) startCopilot(workingDir string, port int) {
	args := []string{"--acp", "--port", fmt.Sprintf("%d", port)}
	if s.SkipPermissions {
		args = append(args, "--yolo")
	}
	if s.Model != "" {
		args = append(args, "--model", s.Model)
		// For specific lightweight models prefer higher reasoning effort
		// to improve answer depth even on smaller-model variants.
		if s.Model == "gpt-5-mini" {
			args = append(args, "--reasoning-effort", "high")
		}
	}
	// If we have a persisted ACP session id, ask the agent to resume it.
	// This allows the agent process to reattach to the previous session state.
	if s.ACPSession != "" {
		args = append(args, "--resume", s.ACPSession)
	}
	// Try spawning via local procmanager first. If unavailable, fall back to in-process spawn.
	spawnReq := map[string]any{"sessionId": s.ID, "cmd": "copilot", "args": args}
	b, _ := json.Marshal(spawnReq)
	procManagerURL := "http://127.0.0.1:19101/proc/spawn"
	resp, err := http.Post(procManagerURL, "application/json", bytes.NewReader(b))
	var spawnedByPM bool
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			var body map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&body); err == nil {
				if idv, ok := body["processId"]; ok {
					// processId may be returned as float64 from the JSON decoder
					switch v := idv.(type) {
					case float64:
						s.procID = int64(v)
					case int64:
						s.procID = v
					}
				}
				spawnedByPM = true
			}
		}
	}

	// Channel that closes when the process exits (used to abort TCP retry early).
	procDead := make(chan struct{})

	if !spawnedByPM {
		// Fallback: spawn locally in-process
		s.process = exec.Command("copilot", args...)
		s.process.Dir = workingDir

		// Capture stderr so we can surface why the process died.
		var stderrBuf bytes.Buffer
		s.process.Stderr = &stderrBuf

		if err := s.process.Start(); err != nil {
			log.Printf("session %s: spawn copilot: %v", s.ID, err)
			s.Status = "error"
			s.hub.Broadcast("error", WSError{Message: fmt.Sprintf("spawn copilot: %v", err)})
			return
		}
		log.Printf("session %s: spawned copilot pid=%d, args=%v", s.ID, s.process.Process.Pid, args)

		// Record process start in DB if store is available
		if s.store != nil {
			if pid := s.process.Process.Pid; pid != 0 {
				if procID, err := s.store.RecordProcessStart(s.ID, pid, "copilot", fmt.Sprintf("%v", args)); err == nil {
					s.procID = procID
				}
			}
		}

		// Monitor process exit in background.
		go func(c *exec.Cmd) {
			err := c.Wait()
			exitCode := 0
			if err != nil {
				if ee, ok := err.(*exec.ExitError); ok {
					exitCode = ee.ExitCode()
				} else {
					exitCode = -1
				}
			}
			stderr := strings.TrimSpace(stderrBuf.String())
			if stderr != "" {
				log.Printf("session %s: copilot stderr: %s", s.ID, stderr)
			}
			log.Printf("session %s: copilot exited code=%d", s.ID, exitCode)
			if s.store != nil && s.procID != 0 {
				_ = s.store.UpdateProcessExit(s.procID, exitCode)
			}
			close(procDead)
		}(s.process)
	}

	// Persist session (procID may have been set)
	if s.store != nil {
		_ = s.store.UpsertSession(s)
	}

	// Retry TCP connect until copilot is listening, or the process dies.
	var conn net.Conn
	for i := 0; i < 30; i++ {
		// If the process already exited, stop trying.
		select {
		case <-procDead:
			log.Printf("session %s: copilot process died before accepting connections", s.ID)
			s.Status = "error"
			errMsg := "copilot process exited before accepting connections"
			s.hub.Broadcast("error", WSError{Message: errMsg})
			return
		default:
		}
		var cerr error
		conn, cerr = net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if cerr == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if conn == nil {
		log.Printf("session %s: tcp connect failed after spawn (port %d)", s.ID, port)
		s.Status = "error"
		s.hub.Broadcast("error", WSError{Message: fmt.Sprintf("tcp connect to copilot timed out on port %d", port)})
		return
	}

	s.acp = NewACPClient(conn, conn, conn.Close)
	s.finishACPSetup(workingDir)
}

// startClaude spawns `claude-agent-acp` and connects via stdio pipes.
func (s *Session) startClaude(workingDir string) {
	// Note: --dangerously-skip-permissions and --model are NOT CLI flags for
	// claude-agent-acp. They are set via the ACP protocol in finishACPSetup.
	s.process = exec.Command("claude-agent-acp")
	s.process.Dir = workingDir
	var stderrBuf bytes.Buffer
	s.process.Stderr = &stderrBuf

	stdin, err := s.process.StdinPipe()
	if err != nil {
		log.Printf("session %s: stdin pipe: %v", s.ID, err)
		s.Status = "error"
		s.hub.Broadcast("error", WSError{Message: fmt.Sprintf("stdin pipe: %v", err)})
		return
	}
	stdout, err := s.process.StdoutPipe()
	if err != nil {
		log.Printf("session %s: stdout pipe: %v", s.ID, err)
		s.Status = "error"
		s.hub.Broadcast("error", WSError{Message: fmt.Sprintf("stdout pipe: %v", err)})
		return
	}

	if err := s.process.Start(); err != nil {
		log.Printf("session %s: spawn claude-agent-acp: %v", s.ID, err)
		s.Status = "error"
		s.hub.Broadcast("error", WSError{Message: fmt.Sprintf("spawn claude-agent-acp: %v", err)})
		return
	}

	if s.store != nil {
		if pid := s.process.Process.Pid; pid != 0 {
			if procID, err := s.store.RecordProcessStart(s.ID, pid, "claude-agent-acp", ""); err == nil {
				s.procID = procID
			}
		}
	}
	s.persistSession()

	go func(c *exec.Cmd) {
		err := c.Wait()
		exitCode := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				exitCode = ee.ExitCode()
			} else {
				exitCode = -1
			}
		}
		stderr := strings.TrimSpace(stderrBuf.String())
		if stderr != "" {
			log.Printf("session %s: claude-agent-acp stderr: %s", s.ID, stderr)
		}
		log.Printf("session %s: claude-agent-acp exited code=%d", s.ID, exitCode)
		if s.store != nil && s.procID != 0 {
			_ = s.store.UpdateProcessExit(s.procID, exitCode)
		}
	}(s.process)

	s.acp = NewACPClient(stdout, stdin, func() error {
		stdin.Close()
		return stdout.Close()
	})
	s.finishACPSetup(workingDir)
}

// finishACPSetup runs the ACP handshake and starts the prompt queue.
// Shared by both backends once the transport is wired up.
func (s *Session) finishACPSetup(workingDir string) {
	s.acp.onNotification = s.handleNotification
	s.acp.onRequest = s.handleAgentRequest
	// Initialize agent_text buffering/coalescer to reduce Hub broadcast rate.
	if s.agentTextCh == nil {
		s.agentTextCh = make(chan string, 256)
		s.agentTextDone = make(chan struct{})
		s.agentTextFlush = make(chan chan struct{})
		go func() {
			// flush interval and buffer size tuned to balance responsiveness and UI load
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			var buf strings.Builder
			flushBuf := func() {
				if buf.Len() > 0 {
					payload := WSAgentText{Text: buf.String()}
					s.hub.Broadcast("agent_text", payload)
					if s.store != nil {
						_ = s.store.SaveMessageJSON(s.ID, "agent_text", payload)
					}
					s.persistSession()
					buf.Reset()
				}
			}
			for {
				select {
				case t, ok := <-s.agentTextCh:
					if !ok {
						flushBuf()
						close(s.agentTextDone)
						return
					}
					buf.WriteString(t)
					if buf.Len() > 4096 {
						flushBuf()
					}
				case <-ticker.C:
					flushBuf()
				case done := <-s.agentTextFlush:
					// Drain any text still sitting in the channel before flushing.
				drainLoop:
					for {
						select {
						case t, ok := <-s.agentTextCh:
							if !ok {
								break drainLoop
							}
							buf.WriteString(t)
						default:
							break drainLoop
						}
					}
					flushBuf()
					close(done)
				}
			}
		}()
	}
	s.acp.Start()

	if _, err := s.acp.Initialize(); err != nil {
		log.Printf("session %s: initialize failed: %v", s.ID, err)
		s.Status = "error"
		s.hub.Broadcast("error", WSError{Message: fmt.Sprintf("ACP initialize failed: %v", err)})
		return
	}

	// If we have a persisted ACP session ID, try to resume it first.
	// Fall back to creating a new session if resume fails.
	var acpSessionID string
	if s.ACPSession != "" {
		resumed, err := s.acp.SessionResume(s.ACPSession, workingDir)
		if err != nil {
			log.Printf("session %s: session/resume failed, creating new: %v", s.ID, err)
		} else {
			acpSessionID = resumed
		}
	}
	if acpSessionID == "" {
		var err error
		acpSessionID, err = s.acp.SessionNew(workingDir)
		if err != nil {
			log.Printf("session %s: session/new failed: %v", s.ID, err)
			s.Status = "error"
			s.hub.Broadcast("error", WSError{Message: fmt.Sprintf("session/new failed: %v", err)})
			return
		}
	}

	s.ACPSession = acpSessionID

	// Set model via session/configure if one was requested.
	if s.Model != "" {
		if err := s.acp.SessionSetConfigOption(acpSessionID, "model", s.Model); err != nil {
			log.Printf("session %s: session/configure model=%s failed: %v", s.ID, s.Model, err)
		} else {
			log.Printf("session %s: configured model=%s", s.ID, s.Model)
		}
	}

	// Set bypass permissions mode via ACP protocol if requested.
	// The --dangerously-skip-permissions CLI flag is not processed by claude-agent-acp;
	// it must be set through the ACP session/set_config_option call.
	if s.SkipPermissions {
		if err := s.acp.SessionSetConfigOption(acpSessionID, "mode", "bypassPermissions"); err != nil {
			log.Printf("session %s: set bypassPermissions failed: %v", s.ID, err)
		} else {
			log.Printf("session %s: configured mode=bypassPermissions", s.ID)
		}
	}

	s.Status = "ready"
	s.persistSession()
	readyPayload := map[string]string{"status": "ready", "acpSessionId": acpSessionID}
	s.hub.Broadcast("status", readyPayload)
	if s.store != nil {
		_ = s.store.SaveMessageJSON(s.ID, "status", readyPayload)
	}

	s.queue = NewPromptQueue(func(item PromptItem) {
		defer close(item.Done)
		s.summaryMu.Lock()
		s.lastMessage = ""
		s.currentTool = ""
		s.currentPrompt = item.Text
		s.isRunning = true
		s.summaryMu.Unlock()
		s.persistSession()
		promptPayload := map[string]string{"text": item.Text}
		s.hub.Broadcast("prompt_sent", promptPayload)
		var promptID int64
		if s.store != nil {
			id, err := s.store.InsertPrompt(s.ID, item.Text)
			if err == nil {
				promptID = id
			}
			_ = s.store.SaveMessageJSON(s.ID, "prompt_sent", promptPayload)
		}

		ctx, cancel := context.WithCancel(context.Background())
		s.interruptMu.Lock()
		s.interruptCancel = cancel
		s.interruptMu.Unlock()
		result, err := s.acp.SessionPromptContext(ctx, acpSessionID, item.Text)
		s.interruptMu.Lock()
		s.interruptCancel = nil
		s.interruptMu.Unlock()
		cancel()

		if err != nil {
			log.Printf("session %s: prompt error: %v", s.ID, err)
			s.summaryMu.Lock()
			s.isRunning = false
			s.summaryMu.Unlock()
			s.persistSession()
			s.flushAgentText()
			s.hub.Broadcast("error", WSError{Message: err.Error()})
			if s.store != nil && promptID != 0 {
				_ = s.store.UpdatePromptStatus(promptID, "error")
			}
			return
		}
		s.summaryMu.Lock()
		s.isRunning = false
		s.summaryMu.Unlock()
		s.persistSession()
		s.flushAgentText()
		completePayload := map[string]string{"stopReason": result.StopReason}
		s.hub.Broadcast("run_complete", completePayload)
		if s.eventHub != nil {
			sessionName := filepath.Base(s.WorkingDir)
			title := "Agent finished"
			body := sessionName + " — " + result.StopReason
			notificationID := int64(0)
			if s.store != nil {
				metaMap := map[string]any{
					"stopReason":     result.StopReason,
					"backend":        s.Backend,
					"model":          s.Model,
					"sessionTitle":   s.title,
					"sessionSummary": s.summary,
				}
				metaRaw, _ := json.Marshal(metaMap)
				if id, err := s.store.InsertNotification("run_complete", s.ID, sessionName, title, body, metaRaw); err == nil {
					notificationID = id
				}
			}
			createdAt := time.Now().UTC().Format(time.RFC3339)
			s.eventHub.Broadcast("run_complete", map[string]any{
				"id":             notificationID,
				"sessionId":      s.ID,
				"sessionName":    sessionName,
				"title":          title,
				"body":           body,
				"stopReason":     result.StopReason,
				"backend":        s.Backend,
				"model":          s.Model,
				"sessionTitle":   s.title,
				"sessionSummary": s.summary,
				"createdAt":      createdAt,
			})
		}
		if s.store != nil {
			_ = s.store.SaveMessageJSON(s.ID, "run_complete", completePayload)
			if promptID != 0 {
				_ = s.store.UpdatePromptStatus(promptID, "done")
			}
		}
		// Asynchronously generate a title and summary via local LLM.
		if s.summarizer != nil && s.store != nil {
			go func() {
				msgs, err := s.store.LoadMessages(s.ID)
				if err != nil {
					return
				}
				t, sum := s.summarizer.Summarize(msgs)
				if t == "" && sum == "" {
					return
				}
				s.summaryMu.Lock()
				s.title = t
				s.summary = sum
				s.summaryMu.Unlock()
				_ = s.store.UpdateSessionSummary(s.ID, t, sum)
			}()
		}
	})

	log.Printf("session %s: ready (backend=%s, acp session %s)", s.ID, s.Backend, acpSessionID)

	// Monitor the ACP connection. If the agent drops (idle timeout, crash, etc.)
	// attempt to respawn automatically so the session self-heals.
	go func() {
		<-s.acp.Done()
		// If session was intentionally closed/suspended, don't respawn.
		if s.Status == "closed" || s.Status == "suspended" {
			return
		}
		log.Printf("session %s: agent connection lost, attempting respawn...", s.ID)
		s.Status = "starting"
		s.hub.Broadcast("status", map[string]string{"status": "respawning"})
		if s.store != nil {
			_ = s.store.UpsertSession(s)
		}

		// Clean up the dead ACP client and process before respawning.
		if s.acp != nil {
			s.acp.Close()
			s.acp = nil
		}
		if s.queue != nil {
			s.queue.Close()
			s.queue = nil
		}
		if s.process != nil && s.process.Process != nil {
			_ = s.process.Process.Kill()
			time.Sleep(100 * time.Millisecond)
			s.process = nil
		}

		// Small delay before respawn to avoid tight crash loops.
		time.Sleep(time.Second)

		// Allocate a fresh port for copilot to avoid EADDRINUSE from
		// the previous (now-dead) copilot process still holding the old port.
		if s.Backend == "copilot" && s.allocPort != nil {
			s.port = s.allocPort()
		}

		switch s.Backend {
		case "copilot":
			s.startCopilot(s.WorkingDir, s.port)
		case "claude":
			s.startClaude(s.WorkingDir)
		}
	}()
}

func (sm *SessionManager) Delete(id string) error {
	sm.mu.Lock()
	s, ok := sm.sessions[id]
	if ok {
		delete(sm.sessions, id)
	}
	sm.mu.Unlock()
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}
	s.Close()
	if sm.store != nil {
		_ = sm.store.DeleteSession(id)
	}
	return nil
}

// UpdateSession updates mutable session properties and respawns the backend
// process so the new settings take effect.
func (sm *SessionManager) UpdateSession(id string, skipPermissions bool) (*Session, error) {
	sm.mu.RLock()
	s, ok := sm.sessions[id]
	sm.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("session %s not found", id)
	}

	s.SkipPermissions = skipPermissions
	if sm.store != nil {
		_ = sm.store.UpsertSession(s)
	}

	// Kill the backend process. The respawn monitor goroutine (watching
	// <-s.acp.Done()) will detect the death and restart the process with
	// the updated SkipPermissions flag.
	if s.process != nil && s.process.Process != nil {
		s.hub.Broadcast("status", map[string]string{"status": "respawning"})
		_ = s.process.Process.Signal(syscall.SIGTERM)
	}

	return s, nil
}

func (sm *SessionManager) Shutdown() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	for id, s := range sm.sessions {
		log.Printf("shutting down session %s", id)
		// Suspend (not close) so sessions respawn on next startup.
		// Only explicitly deleted sessions should be "closed".
		s.suspend()
		if sm.store != nil {
			_ = sm.store.UpsertSession(s)
		}
	}
	sm.sessions = make(map[string]*Session)
}

// suspend tears down the running process/transport but marks the session as
// "suspended" so it will be respawned on next server startup.
func (s *Session) suspend() {
	s.Status = "suspended"
	s.teardown()
}

// Close terminates the session permanently (user-initiated delete).
func (s *Session) Close() {
	s.Status = "closed"
	s.teardown()
}

func (s *Session) teardown() {
	if s.queue != nil {
		s.queue.Close()
	}

	// Gracefully close the ACP session so the agent can persist state
	// for resumption. This is critical for --resume to work after reload.
	// Note: session/close is not a supported ACP notification, so we just
	// close the transport directly.
	if s.acp != nil {
		s.acp.Close()
	}

	// Close agent text buffer and wait for coalescer to flush
	if s.agentTextCh != nil {
		close(s.agentTextCh)
		select {
		case <-s.agentTextDone:
		case <-time.After(500 * time.Millisecond):
		}
	}

	if s.process != nil && s.process.Process != nil {
		// Send SIGTERM first so the agent can save state for resume.
		// Only fall back to SIGKILL if it doesn't exit in time.
		_ = s.process.Process.Signal(syscall.SIGTERM)
		exited := make(chan struct{})
		go func() {
			// The monitor goroutine in startCopilot calls Wait(), so we
			// just watch the done channel rather than calling Wait() again.
			if s.acp != nil {
				select {
				case <-s.acp.Done():
				case <-time.After(3 * time.Second):
				}
			} else {
				time.Sleep(3 * time.Second)
			}
			close(exited)
		}()
		select {
		case <-exited:
			// Process exited gracefully
		case <-time.After(4 * time.Second):
			// Force kill if SIGTERM didn't work
			log.Printf("session %s: agent didn't exit after SIGTERM, sending SIGKILL", s.ID)
			_ = s.process.Process.Kill()
			time.Sleep(100 * time.Millisecond)
		}
	}

	if s.store != nil {
		_ = s.store.UpsertSession(s)
	}
	s.hub.Broadcast("session_ended", nil)
	// Disconnect all WebSocket clients so they trigger auto-reconnect
	// promptly (important during graceful upgrades).
	s.hub.DisconnectClients()
}

func (s *Session) handleNotification(method string, params json.RawMessage) {
	switch method {
	case "session/update":
		s.handleSessionUpdate(params)
	default:
		log.Printf("session %s: unhandled notification: %s", s.ID, method)
	}
}

func (s *Session) handleSessionUpdate(params json.RawMessage) {
	var p SessionUpdateParams
	if err := json.Unmarshal(params, &p); err != nil {
		log.Printf("session %s: bad session/update: %v", s.ID, err)
		return
	}

	var base SessionUpdate
	if err := json.Unmarshal(p.Update, &base); err != nil {
		return
	}

	switch base.SessionUpdate {
	case "agent_message_chunk":
		var chunk AgentMessageChunk
		if err := json.Unmarshal(p.Update, &chunk); err != nil {
			return
		}
		s.summaryMu.Lock()
		s.lastMessage += chunk.Content.Text
		// Keep only last 200 chars for summary
		if len(s.lastMessage) > 200 {
			s.lastMessage = s.lastMessage[len(s.lastMessage)-200:]
		}
		s.summaryMu.Unlock()
		// Buffer the agent text and let the per-session coalescer flush periodically.
		// The coalescer handles both broadcasting and persistence, so we do NOT
		// save to the store here to avoid double-writes.
		if s.agentTextCh != nil {
			select {
			case s.agentTextCh <- chunk.Content.Text:
			default:
				// channel full, fall back to immediate broadcast to avoid losing text
				payload := WSAgentText{Text: chunk.Content.Text}
				s.hub.Broadcast("agent_text", payload)
				if s.store != nil {
					_ = s.store.SaveMessageJSON(s.ID, "agent_text", payload)
				}
				s.persistSession()
			}
		} else {
			// If coalescer not set up, broadcast immediately
			payload := WSAgentText{Text: chunk.Content.Text}
			s.hub.Broadcast("agent_text", payload)
			if s.store != nil {
				_ = s.store.SaveMessageJSON(s.ID, "agent_text", payload)
			}
			s.persistSession()
		}

	case "tool_call", "tool_call_update":
		var tc ToolCallFlat
		if err := json.Unmarshal(p.Update, &tc); err != nil {
			return
		}
		s.summaryMu.Lock()
		if tc.Status == "pending" || tc.Status == "running" || tc.Status == "" {
			s.currentTool = tc.Title
		} else {
			s.currentTool = ""
		}
		s.summaryMu.Unlock()
		s.persistSession()
		// Try rich content array first, then fall back to rawInput for context.
		content := formatToolContent(tc.Content)
		if content == "" {
			content = formatToolContent(tc.RawInput)
		}
		toolPayload := WSToolCall{
			ToolCallID: tc.ToolCallID,
			Title:      tc.Title,
			Kind:       tc.Kind,
			Status:     tc.Status,
			Content:    content,
		}
		s.hub.Broadcast("tool_call", toolPayload)
		if s.store != nil {
			_ = s.store.SaveMessageJSON(s.ID, "tool_call", toolPayload)
		}

	case "tool_call_result":
		var tr ToolCallResultUpdate
		if err := json.Unmarshal(p.Update, &tr); err != nil {
			return
		}
		s.summaryMu.Lock()
		s.currentTool = ""
		s.summaryMu.Unlock()
		s.persistSession()
		resultPayload := WSToolResult{
			ToolCallID: tr.ToolCallID,
			Content:    string(tr.Result),
		}
		s.hub.Broadcast("tool_result", resultPayload)
		if s.store != nil {
			_ = s.store.SaveMessageJSON(s.ID, "tool_result", resultPayload)
		}

	default:
		acpPayload := map[string]any{
			"updateType": base.SessionUpdate,
			"data":       p.Update,
		}
		s.hub.Broadcast("acp_update", acpPayload)
		if s.store != nil {
			_ = s.store.SaveMessageJSON(s.ID, "acp_update", acpPayload)
		}
	}
}

func (s *Session) handleAgentRequest(method string, id *json.RawMessage, params json.RawMessage) {
	switch method {
	case "session/request_permission":
		s.handlePermissionRequest(id, params)
	case "fs/read_text_file":
		result, err := handleFSRead(params)
		if err != nil {
			_ = s.acp.RespondError(id, -32603, err.Error())
			return
		}
		_ = s.acp.Respond(id, result)
	case "fs/write_text_file":
		result, err := handleFSWrite(params)
		if err != nil {
			_ = s.acp.RespondError(id, -32603, err.Error())
			return
		}
		_ = s.acp.Respond(id, result)
	case "terminal/create":
		result, err := s.terminal.Create(params)
		if err != nil {
			_ = s.acp.RespondError(id, -32603, err.Error())
			return
		}
		_ = s.acp.Respond(id, result)
	case "terminal/output":
		result, err := s.terminal.Output(params)
		if err != nil {
			_ = s.acp.RespondError(id, -32603, err.Error())
			return
		}
		_ = s.acp.Respond(id, result)
	case "terminal/wait_for_exit":
		go func() {
			result, err := s.terminal.WaitForExit(params)
			if err != nil {
				_ = s.acp.RespondError(id, -32603, err.Error())
				return
			}
			_ = s.acp.Respond(id, result)
		}()
	case "terminal/kill":
		result, err := s.terminal.Kill(params)
		if err != nil {
			_ = s.acp.RespondError(id, -32603, err.Error())
			return
		}
		_ = s.acp.Respond(id, result)
	case "terminal/release":
		result, err := s.terminal.Release(params)
		if err != nil {
			_ = s.acp.RespondError(id, -32603, err.Error())
			return
		}
		_ = s.acp.Respond(id, result)
	default:
		log.Printf("session %s: unhandled agent request: %s", s.ID, method)
		_ = s.acp.RespondError(id, -32601, "method not found: "+method)
	}
}

func (s *Session) handlePermissionRequest(id *json.RawMessage, params json.RawMessage) {
	var p PermissionRequestParams
	if err := json.Unmarshal(params, &p); err != nil {
		_ = s.acp.RespondError(id, -32602, "invalid params")
		return
	}

	// Use a unique request ID — copilot reuses the same toolCallId
	// (e.g. "shell-permission") for every shell command, so we can't key on it.
	requestID := uuid.New().String()[:8]
	ch := make(chan string, 1)

	s.permMu.Lock()
	s.permPending[requestID] = ch
	s.permMu.Unlock()

	// Extract the command string from rawInput for shell-type permissions.
	var command string
	if p.ToolCall.RawInput != nil {
		var raw struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(p.ToolCall.RawInput, &raw) == nil {
			command = raw.Command
		}
	}

	permPayload := WSPermissionRequest{
		RequestID: requestID,
		Title:     p.ToolCall.Title,
		Kind:      p.ToolCall.Kind,
		Command:   command,
		Options:   p.Options,
	}
	s.hub.Broadcast("permission_request", permPayload)

	// Notify global event hub so the mobile app can show a notification
	// even when this session's chat screen isn't open.
	if s.eventHub != nil {
		sessionName := filepath.Base(s.WorkingDir)
		title := p.ToolCall.Title
		if title == "" {
			title = "Tool approval needed"
		}
		body := sessionName + " is waiting for approval"
		notificationID := int64(0)
		if s.store != nil {
			metaMap := map[string]any{
				"kind":      p.ToolCall.Kind,
				"command":   command,
				"backend":   s.Backend,
				"model":     s.Model,
				"requestId": requestID,
			}
			metaRaw, _ := json.Marshal(metaMap)
			if id, err := s.store.InsertNotification("permission_request", s.ID, sessionName, title, body, metaRaw); err == nil {
				notificationID = id
			}
		}
		createdAt := time.Now().UTC().Format(time.RFC3339)
		s.eventHub.Broadcast("permission_request", map[string]any{
			"id":          notificationID,
			"sessionId":   s.ID,
			"sessionName": sessionName,
			"title":       title,
			"body":        body,
			"kind":        p.ToolCall.Kind,
			"command":     command,
			"backend":     s.Backend,
			"model":       s.Model,
			"requestId":   requestID,
			"createdAt":   createdAt,
		})
	}

	go func() {
		optionID := <-ch
		s.permMu.Lock()
		delete(s.permPending, requestID)
		s.permMu.Unlock()

		_ = s.acp.Respond(id, PermissionOutcome{
			Outcome: struct {
				Outcome  string `json:"outcome"`
				OptionID string `json:"optionId,omitempty"`
			}{
				Outcome:  "selected",
				OptionID: optionID,
			},
		})

		s.hub.Broadcast("permission_resolved", map[string]string{
			"requestId": requestID,
			"optionId":  optionID,
		})
	}()
}

func (s *Session) RespondToPermission(requestID, optionID string) {
	s.permMu.Lock()
	ch, ok := s.permPending[requestID]
	s.permMu.Unlock()
	if ok {
		select {
		case ch <- optionID:
		default:
		}
	}
}

// Interrupt cancels any in-progress prompt. The agent is notified via a
// session/interrupt notification so it can stop cleanly; the pending Call is
// also cancelled so the bridge stops waiting regardless of agent behaviour.
func (s *Session) Interrupt() {
	s.interruptMu.Lock()
	cancel := s.interruptCancel
	s.interruptMu.Unlock()
	if cancel == nil {
		return
	}
	if s.acp != nil && s.ACPSession != "" {
		_ = s.acp.Notify("session/interrupt", map[string]string{"sessionId": s.ACPSession})
	}
	cancel()
	s.flushAgentText()
	s.hub.Broadcast("interrupted", map[string]string{})
}

func (s *Session) SendPrompt(text string) {
	if s.queue == nil {
		s.hub.Broadcast("error", WSError{Message: "session not ready"})
		return
	}
	s.queue.Enqueue(text)
}

// flushAgentText synchronously drains any buffered agent text so that
// terminal messages (run_complete, interrupted, error) are ordered after
// the final text chunk on the WebSocket.
func (s *Session) flushAgentText() {
	if s.agentTextFlush == nil {
		return
	}
	done := make(chan struct{})
	select {
	case s.agentTextFlush <- done:
		<-done
	case <-time.After(200 * time.Millisecond):
	}
}

// formatToolContent converts tool call content (json.RawMessage) into a
// human-readable string for display in the mobile UI. It handles:
//   - ACP rich content arrays: [{type:"content", content:{type:"text", text:"..."}}, {type:"diff", ...}]
//   - Flat JSON objects (rawInput): {file_path:"...", command:"...", pattern:"..."}
//   - Raw strings
func formatToolContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	// Try ACP rich content array first.
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err == nil {
		if len(arr) == 0 {
			return ""
		}
		var parts []string
		for _, item := range arr {
			switch item["type"] {
			case "content":
				if inner, ok := item["content"].(map[string]any); ok {
					if text, ok := inner["text"].(string); ok && text != "" {
						parts = append(parts, truncate(text, 300))
					}
				}
			case "diff":
				path, _ := item["path"].(string)
				if path != "" {
					parts = append(parts, path)
				}
			case "terminal":
				// Terminal output handled separately
			}
		}
		return strings.Join(parts, "\n")
	}

	// Try flat JSON object (rawInput).
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		var parts []string
		// Show the most useful keys in a sensible order.
		for _, k := range []string{"file_path", "command", "pattern", "query", "url", "description", "content"} {
			if v, ok := obj[k]; ok {
				if s, ok := v.(string); ok && s != "" {
					parts = append(parts, truncate(s, 200))
				}
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
		// Fall back to all string keys.
		for k, v := range obj {
			if s, ok := v.(string); ok && s != "" {
				parts = append(parts, fmt.Sprintf("%s: %s", k, truncate(s, 150)))
			}
		}
		return strings.Join(parts, "\n")
	}

	// Fall back to raw string representation.
	return truncate(string(raw), 500)
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
