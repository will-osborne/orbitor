package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
)

// prURLRe matches GitHub pull request URLs in any text/output.
var prURLRe = regexp.MustCompile(`https://github\.com/[A-Za-z0-9_.\-]+/[A-Za-z0-9_.\-]+/pull/\d+`)

// runCompleteCmd is sent to the coalescer to drain any buffered agent text and
// then broadcast run_complete, guaranteeing strict ordering between the two.
type runCompleteCmd struct {
	payload any
	done    chan struct{}
}

type Session struct {
	ID              string
	WorkingDir      string
	Backend         string    // "copilot" or "claude"
	Model           string    // model to use (e.g. "claude-sonnet-4.5", "gpt-5")
	SkipPermissions bool      // pass --yolo (copilot) or --dangerously-skip-permissions (claude)
	PlanMode        bool      // enable plan mode (mode=plan via ACP)
	ACPSession      string    // ACP session ID returned by agent
	ResumeSession   string    // original session ID for copilot --resume (preserved across respawns)
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
	prURL         string // first GitHub PR URL detected in agent output

	// toolCallCache remembers kind/title from the initial tool_call event so
	// that subsequent tool_call_update deltas (which often omit those fields)
	// can be displayed with the correct tool name and label.
	toolCallCacheMu sync.Mutex
	toolCallCache   map[string]WSToolCall // keyed by ToolCallID

	// subAgents tracks active/completed sub-agent tool calls (Task, dispatch_agent, etc.)
	subAgentsMu sync.RWMutex
	subAgents   []SubAgentInfo

	// bgContMu guards the background-continuation debounce timer.
	// After run_complete, if background sub-agents are still running, their
	// completion triggers a silent continuation prompt so Claude can respond
	// with the results without waiting for the user to send a new message.
	bgContMu    sync.Mutex
	bgContTimer *time.Timer

	// agentTextCh batches frequent agent_text chunks to reduce broadcast frequency
	agentTextCh            chan string
	agentTextDone          chan struct{}
	agentTextFlush         chan chan struct{}   // request a synchronous flush of the text buffer
	agentTextRunCompleteCh chan runCompleteCmd // drain text then broadcast run_complete atomically

	store      *Store      // persistence hook
	allocPort  func() int  // returns a fresh port number (from SessionManager)
	eventHub   *Hub        // global event hub for cross-session notifications
	summarizer *Summarizer // optional LLM summarizer (nil = disabled)
	fcm        *FCMSender  // optional FCM push sender (nil = disabled)
	history    *RunHistory // per-session file change history
}

func (s *Session) persistSession() {
	if s.store == nil {
		return
	}
	_ = s.store.UpsertSession(s)
}

// isSubAgentKind returns true if the tool kind indicates a sub-agent invocation
// (e.g. Claude Code's Task tool or Copilot's dispatch_agent).
func isSubAgentKind(kind string) bool {
	k := strings.ToLower(kind)
	return strings.Contains(k, "task") || strings.Contains(k, "agent")
}

// updateSubAgent creates or updates a tracked sub-agent entry.
func (s *Session) updateSubAgent(toolCallID, title, status string) {
	s.subAgentsMu.Lock()
	defer s.subAgentsMu.Unlock()
	for i, sa := range s.subAgents {
		if sa.ToolCallID == toolCallID {
			if title != "" {
				s.subAgents[i].Title = title
			}
			if status != "" {
				s.subAgents[i].Status = status
			}
			return
		}
	}
	if status == "" {
		status = "running"
	}
	s.subAgents = append(s.subAgents, SubAgentInfo{
		ToolCallID: toolCallID,
		Title:      title,
		Status:     status,
		StartedAt:  time.Now(),
	})
}

// completeSubAgent marks a sub-agent as completed when its result arrives.
func (s *Session) completeSubAgent(toolCallID string) {
	s.subAgentsMu.Lock()
	defer s.subAgentsMu.Unlock()
	for i, sa := range s.subAgents {
		if sa.ToolCallID == toolCallID {
			if s.subAgents[i].Status == "running" {
				s.subAgents[i].Status = "completed"
			}
			_ = sa
			return
		}
	}
}

// scheduleBackgroundContinuation debounces a silent follow-up prompt after
// background sub-agents complete post-run_complete. The follow-up is sent to
// the ACP backend so Claude can generate a text response about the results
// (which would otherwise only appear when the user sends the next prompt).
func (s *Session) scheduleBackgroundContinuation() {
	s.summaryMu.RLock()
	running := s.isRunning
	s.summaryMu.RUnlock()
	if running {
		return // Main prompt still active — don't schedule
	}
	s.bgContMu.Lock()
	defer s.bgContMu.Unlock()
	if s.bgContTimer != nil {
		s.bgContTimer.Stop()
	}
	// Debounce 2 s: if multiple sub-agents complete close together, only send
	// one follow-up prompt after the last one settles.
	s.bgContTimer = time.AfterFunc(2*time.Second, func() {
		s.summaryMu.RLock()
		running := s.isRunning
		s.summaryMu.RUnlock()
		if running {
			return // A user prompt arrived before the timer fired
		}
		if s.queue != nil {
			s.queue.Enqueue("\u200b") // zero-width space = silent continuation
		}
	})
}

type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	store    *Store
	// EventHub broadcasts global events (e.g. permission requests) to
	// connected /ws/events clients.
	EventHub   *Hub
	summarizer *Summarizer // optional LLM summarizer for session titles/summaries
	fcm        *FCMSender  // optional FCM push notification sender (nil = disabled)
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
					PlanMode:        r.PlanMode,
					ACPSession:      r.ACPSession,
					ResumeSession:   r.ResumeSession,
					Status:          r.Status,
					CreatedAt:       r.CreatedAt,
					port:            r.Port,
					procID:          r.ProcID,
					lastMessage:     r.LastMessage,
					currentTool:     r.CurrentTool,
					title:           r.Title,
					summary:         r.Summary,
					prURL:           r.PRURL,
					hub:             NewHub(),
					terminal:        NewTerminalManager(),
					permPending:     make(map[string]chan string),
						toolCallCache:   make(map[string]WSToolCall),
					store:           store,
					allocPort:       sm.AllocPort,
					eventHub:        eventHub,
					summarizer:      summarizer,
					history:         newRunHistory(),
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
		// Wire in FCM sender now that sm.fcm may have been set after construction.
		if s.fcm == nil && sm.fcm != nil {
			s.fcm = sm.fcm
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
		prURL := s.prURL
		s.summaryMu.RUnlock()

		s.permMu.Lock()
		hasPerm := len(s.permPending) > 0
		s.permMu.Unlock()

		queueDepth := 0
		if s.queue != nil {
			queueDepth = s.queue.QueueDepth()
		}

		s.subAgentsMu.RLock()
		subAgents := make([]SubAgentInfo, len(s.subAgents))
		copy(subAgents, s.subAgents)
		s.subAgentsMu.RUnlock()

		out = append(out, WSSessionInfo{
			ID:                s.ID,
			WorkingDir:        s.WorkingDir,
			ACPSession:        s.ACPSession,
			Status:            s.Status,
			Backend:           s.Backend,
			Model:             s.Model,
			SkipPermissions:   s.SkipPermissions,
			PlanMode:          s.PlanMode,
			LastMessage:       lastMsg,
			CurrentTool:       curTool,
			CurrentPrompt:     curPrompt,
			IsRunning:         running,
			QueueDepth:        queueDepth,
			PendingPermission: hasPerm,
			CreatedAt:         s.CreatedAt,
			Title:             title,
			Summary:           summary,
			PRURL:             prURL,
			SubAgents:         subAgents,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		// Order primarily by creation time (newest first). If two sessions
		// have the same CreatedAt timestamp fall back to a deterministic
		// secondary key (ID) so the ordering is stable across calls.
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func (sm *SessionManager) Get(id string) *Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.sessions[id]
}

func (sm *SessionManager) Create(workingDir, backend, model string, skipPermissions, planMode bool) (*Session, error) {
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
		PlanMode:        planMode,
		Status:          "starting",
		CreatedAt:       time.Now(),
		port:            port,
		hub:             NewHub(),
		terminal:        NewTerminalManager(),
		permPending:     make(map[string]chan string),
		toolCallCache:   make(map[string]WSToolCall),
		store:           sm.store,
		allocPort:       sm.AllocPort,
		eventHub:        sm.EventHub,
		summarizer:      sm.summarizer,
		fcm:             sm.fcm,
		history:         newRunHistory(),
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

// Clone creates a new session using the same runtime config as sourceID.
func (sm *SessionManager) Clone(sourceID string) (*Session, error) {
	sm.mu.RLock()
	src := sm.sessions[sourceID]
	if src == nil {
		sm.mu.RUnlock()
		return nil, fmt.Errorf("session not found: %s", sourceID)
	}
	workingDir := src.WorkingDir
	backend := src.Backend
	model := src.Model
	skip := src.SkipPermissions
	plan := src.PlanMode
	sm.mu.RUnlock()
	return sm.Create(workingDir, backend, model, skip, plan)
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
	// Pass MCP server config from ~/.copilot/mcp-config.json via CLI flag.
	// Copilot's ACP implementation doesn't support mcpServers in session/new,
	// so we load any project-local .mcp.json and pass it as inline JSON.
	if workingDir != "" {
		mcpPath := filepath.Join(workingDir, ".mcp.json")
		if data, err := os.ReadFile(mcpPath); err == nil {
			args = append(args, "--additional-mcp-config", string(data))
		}
	}

	// If we have a persisted session id, ask the agent to resume it.
	// Prefer ResumeSession (the original conversation ID preserved across
	// respawns) over ACPSession (which may have been replaced by a new
	// ACP-level session after a failed SessionResume).
	resumeID := s.ResumeSession
	if resumeID == "" {
		resumeID = s.ACPSession
	}
	if resumeID != "" {
		args = append(args, "--resume", resumeID)
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
		s.agentTextRunCompleteCh = make(chan runCompleteCmd, 1)
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
				case rc := <-s.agentTextRunCompleteCh:
					// Drain remaining text, flush it, then broadcast run_complete.
					// This guarantees run_complete is never sent before the last
					// agent_text chunk — fixing the ordering race with the coalescer.
				drainLoopRC:
					for {
						select {
						case t, ok := <-s.agentTextCh:
							if !ok {
								break drainLoopRC
							}
							buf.WriteString(t)
						default:
							break drainLoopRC
						}
					}
					flushBuf()
					s.hub.Broadcast("run_complete", rc.payload)
					close(rc.done)
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
		// Only pass MCP servers via ACP for backends that support it (Claude).
		// Copilot reads ~/.copilot/mcp-config.json natively and gets project-
		// local .mcp.json via --additional-mcp-config CLI flag.
		var mcpServers []any
		if s.Backend != "copilot" {
			mcpServers = LoadMCPServers(s.Backend, workingDir)
			if len(mcpServers) > 0 {
				log.Printf("session %s: loading %d MCP server(s) from native config", s.ID, len(mcpServers))
			}
		}
		var err error
		acpSessionID, err = s.acp.SessionNew(workingDir, mcpServers)
		if err != nil {
			log.Printf("session %s: session/new failed: %v", s.ID, err)
			s.Status = "error"
			s.hub.Broadcast("error", WSError{Message: fmt.Sprintf("session/new failed: %v", err)})
			return
		}
	}

	// For copilot, preserve the original conversation ID for --resume across
	// respawns. The CLI --resume flag handles conversation continuity even
	// when the ACP-level SessionResume fails (e.g. after an interrupt kills
	// the process). Without this, the ACPSession gets overwritten with a new
	// session ID and the conversation context is lost on next respawn.
	if s.Backend == "copilot" && s.ACPSession != "" && acpSessionID != s.ACPSession {
		if s.ResumeSession == "" {
			s.ResumeSession = s.ACPSession
		}
		log.Printf("session %s: ACP session changed %s → %s (resume preserved: %s)",
			s.ID, s.ACPSession, acpSessionID, s.ResumeSession)
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

	// Set mode via ACP protocol:
	// bypassPermissions takes precedence over plan mode (they are mutually exclusive).
	if s.SkipPermissions {
		if err := s.acp.SessionSetConfigOption(acpSessionID, "mode", "bypassPermissions"); err != nil {
			log.Printf("session %s: set bypassPermissions failed: %v", s.ID, err)
		} else {
			log.Printf("session %s: configured mode=bypassPermissions", s.ID)
		}
	} else if s.PlanMode {
		if err := s.acp.SessionSetConfigOption(acpSessionID, "mode", "plan"); err != nil {
			log.Printf("session %s: set plan mode failed: %v", s.ID, err)
		} else {
			log.Printf("session %s: configured mode=plan", s.ID)
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
		// The zero-width space sentinel (\u200b) is an auto-generated continuation
		// prompt sent after background sub-agents complete. It is silent: no
		// prompt_sent broadcast, no persistence, and it sends a minimal "." to
		// Claude so it can respond with the background task results.
		isContinuation := item.Text == "\u200b"
		acpText := item.Text
		if isContinuation {
			acpText = "."
		}

		s.summaryMu.Lock()
		s.lastMessage = ""
		s.currentTool = ""
		s.currentPrompt = item.Text
		s.isRunning = true
		s.summaryMu.Unlock()
		if !isContinuation {
			s.history.StartRun(item.Text)
		}
		s.persistSession()
		var promptID int64
		if !isContinuation {
			promptPayload := map[string]string{"text": item.Text}
			s.hub.Broadcast("prompt_sent", promptPayload)
			// Broadcast updated queue depth (decremented since this item just dequeued).
			s.hub.Broadcast("queue_update", map[string]int{"depth": s.queue.QueueDepth()})
			if s.store != nil {
				id, err := s.store.InsertPrompt(s.ID, item.Text)
				if err == nil {
					promptID = id
				}
				_ = s.store.SaveMessageJSON(s.ID, "prompt_sent", promptPayload)
			}
		}

		ctx, cancel := context.WithCancel(context.Background())
		s.interruptMu.Lock()
		s.interruptCancel = cancel
		s.interruptMu.Unlock()
		result, err := s.acp.SessionPromptContext(ctx, acpSessionID, acpText)
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
		prURL := s.prURL
		s.summaryMu.Unlock()
		s.history.CompleteRun()
		s.persistSession()
		completePayload := map[string]string{"stopReason": result.StopReason, "prUrl": prURL}
		// Route run_complete through the coalescer so it is strictly ordered
		// after all agent_text chunks already queued. This prevents clients
		// from receiving run_complete before the last text has been flushed.
		if s.agentTextRunCompleteCh != nil {
			rcDone := make(chan struct{})
			select {
			case s.agentTextRunCompleteCh <- runCompleteCmd{payload: completePayload, done: rcDone}:
				select {
				case <-rcDone:
				case <-time.After(500 * time.Millisecond):
					log.Printf("session %s: run_complete drain timed out, broadcasting directly", s.ID)
					s.hub.Broadcast("run_complete", completePayload)
				}
			default:
				// Channel full (shouldn't happen in normal flow); fall back.
				s.flushAgentText()
				s.hub.Broadcast("run_complete", completePayload)
			}
		} else {
			s.flushAgentText()
			s.hub.Broadcast("run_complete", completePayload)
		}
		if s.eventHub != nil && !isContinuation {
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
				"prUrl":          prURL,
				"createdAt":      createdAt,
			})
			fcmData := map[string]string{
				"eventType": "run_complete",
				"sessionId": s.ID,
			}
			if prURL != "" {
				fcmData["prUrl"] = prURL
			}
			go s.fcm.Send(title, body, fcmData)
		}
		if s.store != nil && !isContinuation {
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
		// If session was intentionally closed/suspended/killed, don't respawn.
		if s.Status == "closed" || s.Status == "suspended" || s.Status == "killed" {
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

func (sm *SessionManager) KillSession(id string) error {
	sm.mu.RLock()
	s, ok := sm.sessions[id]
	sm.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}
	s.KillProcess()
	return nil
}

func (sm *SessionManager) ReviveSession(id string) error {
	sm.mu.RLock()
	s, ok := sm.sessions[id]
	sm.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}
	return s.Revive()
}

// UpdateSession updates mutable session properties and applies runtime changes.
// For copilot, model changes require respawn so process args are updated.
func (sm *SessionManager) UpdateSession(id string, skipPermissions, planMode *bool, model *string) (*Session, error) {
	sm.mu.RLock()
	s, ok := sm.sessions[id]
	sm.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("session %s not found", id)
	}

	modelChanged := false
	if model != nil {
		next := strings.TrimSpace(*model)
		if next != s.Model {
			modelChanged = true
			s.Model = next
		}
	}
	if skipPermissions != nil {
		s.SkipPermissions = *skipPermissions
	}
	if planMode != nil {
		s.PlanMode = *planMode
	}
	if sm.store != nil {
		_ = sm.store.UpsertSession(s)
	}

	// Apply ACP config changes for live sessions.
	if s.acp != nil && s.ACPSession != "" {
		if modelChanged {
			if s.Model == "" {
				_ = s.acp.SessionSetConfigOption(s.ACPSession, "model", nil)
			} else {
				_ = s.acp.SessionSetConfigOption(s.ACPSession, "model", s.Model)
			}
		}
		if skipPermissions != nil || planMode != nil {
			if s.SkipPermissions {
				_ = s.acp.SessionSetConfigOption(s.ACPSession, "mode", "bypassPermissions")
			} else if s.PlanMode {
				_ = s.acp.SessionSetConfigOption(s.ACPSession, "mode", "plan")
			} else {
				_ = s.acp.SessionSetConfigOption(s.ACPSession, "mode", "default")
			}
		}
	}

	// Copilot model changes must respawn to ensure CLI startup flags match.
	// All other changes are applied live via ACP session/configure.
	if modelChanged && s.Backend == "copilot" {
		s.hub.Broadcast("status", map[string]string{"status": "respawning"})
		if s.process != nil && s.process.Process != nil {
			_ = s.process.Process.Signal(syscall.SIGTERM)
		} else if s.procID != 0 {
			// Session launched via local procmanager; request stop by processId.
			stopReq := map[string]any{"processId": s.procID}
			b, _ := json.Marshal(stopReq)
			_, _ = http.Post("http://127.0.0.1:19101/proc/stop", "application/json", bytes.NewReader(b))
		}
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
	s.bgContMu.Lock()
	if s.bgContTimer != nil {
		s.bgContTimer.Stop()
		s.bgContTimer = nil
	}
	s.bgContMu.Unlock()

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
		if s.prURL == "" {
			if m := prURLRe.FindString(chunk.Content.Text); m != "" {
				s.prURL = m
			}
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
		// Merge with cached data: tool_call_update events often omit kind/title
		// since they were already sent in the initial tool_call event. Pull them
		// from the cache so the TUI always displays the correct tool name.
		s.toolCallCacheMu.Lock()
		cached := s.toolCallCache[tc.ToolCallID]
		if tc.Kind == "" {
			tc.Kind = cached.Kind
		}
		if tc.Title == "" {
			tc.Title = cached.Title
		}
		// Update cache with any non-empty values from this event.
		if tc.Kind != "" {
			cached.Kind = tc.Kind
		}
		if tc.Title != "" {
			cached.Title = tc.Title
		}
		s.toolCallCache[tc.ToolCallID] = cached
		s.toolCallCacheMu.Unlock()

		s.summaryMu.Lock()
		if tc.Status == "pending" || tc.Status == "running" || tc.Status == "" {
			s.currentTool = tc.Title
		} else {
			s.currentTool = ""
		}
		s.summaryMu.Unlock()
		s.persistSession()
		// Track sub-agent invocations (Task tool, dispatch_agent, etc.)
		if isSubAgentKind(tc.Kind) {
			subStatus := tc.Status
			if subStatus == "pending" || subStatus == "" {
				subStatus = "running"
			}
			s.updateSubAgent(tc.ToolCallID, tc.Title, subStatus)
			// When a background sub-agent completes after the main turn, schedule
			// a silent follow-up so Claude can reply with the results.
			if tc.Status == "completed" || tc.Status == "failed" {
				s.scheduleBackgroundContinuation()
			}
		}
		// Try rich content array first, then fall back to rawInput for context.
		content := formatToolContent(tc.Content)
		if content == "" {
			content = formatToolContent(tc.RawInput)
		}
		if content != "" {
			s.summaryMu.Lock()
			if s.prURL == "" {
				if m := prURLRe.FindString(content); m != "" {
					s.prURL = m
				}
			}
			s.summaryMu.Unlock()
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
		s.completeSubAgent(tr.ToolCallID)
		s.scheduleBackgroundContinuation()
		content := formatToolContent(tr.Result)
		if content == "" && len(tr.Result) > 0 {
			content = string(tr.Result)
		}
		if result := content; result != "" {
			s.summaryMu.Lock()
			if s.prURL == "" {
				if m := prURLRe.FindString(result); m != "" {
					s.prURL = m
				}
			}
			s.summaryMu.Unlock()
		}
		resultPayload := WSToolResult{
			ToolCallID: tr.ToolCallID,
			Content:    content,
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
		var wp FSWriteParams
		before := ""
		if json.Unmarshal(params, &wp) == nil && wp.Path != "" {
			if data, err := os.ReadFile(wp.Path); err == nil {
				before = string(data)
			}
		}
		result, err := handleFSWrite(params)
		if err != nil {
			_ = s.acp.RespondError(id, -32603, err.Error())
			return
		}
		if wp.Path != "" {
			if after, err := os.ReadFile(wp.Path); err == nil {
				s.history.RecordFileChange(wp.Path, s.WorkingDir, before, string(after))
			}
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
		go s.fcm.Send(title, body, map[string]string{
			"eventType": "permission_request",
			"sessionId": s.ID,
			"requestId": requestID,
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
	// Cancel the in-flight context if a prompt is still blocking so
	// SessionPromptContext unblocks immediately.
	if cancel != nil {
		cancel()
	}
	if s.queue != nil {
		s.queue.Clear()
	}
	// Send SIGINT to the agent process so it stops even if the prompt call
	// already returned and the agent is mid-response.
	if s.process != nil && s.process.Process != nil {
		_ = s.process.Process.Signal(syscall.SIGINT)
	}
	if s.acp != nil && s.ACPSession != "" {
		_ = s.acp.Notify("session/interrupt", map[string]string{"sessionId": s.ACPSession})
	}
	s.flushAgentText()
	s.hub.Broadcast("interrupted", map[string]string{})
}

// KillProcess forcefully kills the agent process (SIGKILL) without waiting
// for a graceful shutdown. The session remains in memory so it can be
// revived later via Revive().
func (s *Session) KillProcess() {
	// Unblock any in-flight prompt immediately.
	s.interruptMu.Lock()
	cancel := s.interruptCancel
	s.interruptMu.Unlock()
	if cancel != nil {
		cancel()
	}

	// Drain the queue so nothing starts after the kill.
	if s.queue != nil {
		s.queue.Clear()
		s.queue.Close()
		s.queue = nil
	}

	// Mark as killed BEFORE killing the process so the respawn monitor
	// goroutine (watching acp.Done) sees the status and does not respawn.
	s.Status = "killed"
	if s.store != nil {
		_ = s.store.UpsertSession(s)
	}

	if s.acp != nil {
		s.acp.Close()
	}

	if s.process != nil && s.process.Process != nil {
		_ = s.process.Process.Kill()
		s.process = nil
	}

	s.hub.Broadcast("status", map[string]string{"status": "killed"})
}

// Revive restarts the agent process for a session that was previously killed.
func (s *Session) Revive() error {
	if s.Status != "killed" {
		return fmt.Errorf("session is not in killed state (status: %s)", s.Status)
	}

	// Clean up any stale ACP reference from before the kill.
	if s.acp != nil {
		s.acp.Close()
		s.acp = nil
	}

	if s.Backend == "copilot" && s.allocPort != nil {
		s.port = s.allocPort()
	}

	s.Status = "starting"
	s.hub.Broadcast("status", map[string]string{"status": "starting"})

	switch s.Backend {
	case "copilot":
		s.startCopilot(s.WorkingDir, s.port)
	case "claude":
		s.startClaude(s.WorkingDir)
	default:
		return fmt.Errorf("unknown backend: %s", s.Backend)
	}

	return nil
}

func (s *Session) SendPrompt(text string) {
	if err := s.EnqueuePrompt(text); err != nil {
		s.hub.Broadcast("error", WSError{Message: err.Error()})
	}
}

func (s *Session) EnqueuePrompt(text string) error {
	if s.queue == nil {
		return fmt.Errorf("session not ready")
	}
	s.queue.Enqueue(text)
	depth := s.queue.QueueDepth()
	if depth > 0 {
		s.hub.Broadcast("queue_update", map[string]int{"depth": depth})
	}
	return nil
}

// QueuePromptWhenReady waits for queue initialization and then enqueues text.
func (s *Session) QueuePromptWhenReady(text string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if err := s.EnqueuePrompt(text); err == nil {
			return nil
		}
		switch s.Status {
		case "error", "closed", "killed", "suspended":
			return fmt.Errorf("session unavailable: %s", s.Status)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for session readiness")
		}
		time.Sleep(80 * time.Millisecond)
	}
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
				diffText := extractToolDiffText(item)
				switch {
				case path != "" && diffText != "":
					parts = append(parts, path+"\n"+diffText)
				case diffText != "":
					parts = append(parts, diffText)
				case path != "":
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
		// str_replace_editor: synthesise a unified diff from old_str / new_str.
		if diff := strReplaceEditorToDiff(obj); diff != "" {
			return diff
		}
		// apply_patch: convert *** Update File: format to unified diff.
		if patch, ok := obj["patch"].(string); ok && patch != "" {
			if converted := normaliseApplyPatch(patch); converted != "" {
				return converted
			}
		}
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

	// Try plain JSON string.
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		return truncate(str, 500)
	}

	// Fall back to raw string representation.
	return truncate(string(raw), 500)
}

func extractToolDiffText(v any) string {
	switch x := v.(type) {
	case string:
		s := strings.TrimSpace(x)
		if looksLikeUnifiedDiff(s) {
			return s
		}
		return ""
	case map[string]any:
		for _, k := range []string{"diff", "patch", "text", "unified", "unifiedDiff", "content"} {
			if s := extractToolDiffText(x[k]); s != "" {
				return s
			}
		}
		for _, value := range x {
			if s := extractToolDiffText(value); s != "" {
				return s
			}
		}
	case []any:
		for _, item := range x {
			if s := extractToolDiffText(item); s != "" {
				return s
			}
		}
	}
	return ""
}

func looksLikeUnifiedDiff(s string) bool {
	switch {
	case s == "":
		return false
	case strings.Contains(s, "\ndiff --git ") || strings.HasPrefix(s, "diff --git "):
		return true
	case strings.Contains(s, "\n@@ ") || strings.HasPrefix(s, "@@ "):
		return true
	case strings.Contains(s, "\n+++ ") && strings.Contains(s, "\n--- "):
		return true
	default:
		return false
	}
}

// strReplaceEditorToDiff synthesises a unified diff from a str_replace_editor
// tool input that carries old_str and new_str fields.
func strReplaceEditorToDiff(obj map[string]any) string {
	oldStr, hasOld := obj["old_str"].(string)
	newStr, hasNew := obj["new_str"].(string)
	if !hasOld || !hasNew {
		return ""
	}
	path, _ := obj["path"].(string)
	if path == "" {
		path, _ = obj["file_path"].(string)
	}
	if path == "" {
		path = "file"
	}
	var sb strings.Builder
	sb.WriteString("--- a/" + path + "\n")
	sb.WriteString("+++ b/" + path + "\n")
	sb.WriteString("@@ ... @@\n")
	for _, line := range strings.Split(oldStr, "\n") {
		sb.WriteString("-" + line + "\n")
	}
	for _, line := range strings.Split(newStr, "\n") {
		sb.WriteString("+" + line + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// normaliseApplyPatch converts Claude Code's apply_patch format to a standard
// unified diff that renderDiffBlock can consume.
//
// apply_patch format:
//
//	*** Begin Patch
//	*** Update File: path/to/file
//	@@ context line @@
//	-removed line
//	+added line
//	 context line
//	*** End of File
//	*** End Patch
func normaliseApplyPatch(s string) string {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "*** Update File:") && !strings.Contains(s, "*** Add File:") && !strings.Contains(s, "*** Delete File:") {
		return ""
	}
	lines := strings.Split(s, "\n")
	var out []string
	// If there is no *** Begin Patch wrapper, treat the whole string as patch content.
	inPatch := !strings.Contains(s, "*** Begin Patch")
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "*** Begin Patch"):
			inPatch = true
		case strings.HasPrefix(line, "*** End Patch"):
			inPatch = false
		case !inPatch:
			// skip preamble before *** Begin Patch
		case strings.HasPrefix(line, "*** Add File: "):
			f := strings.TrimPrefix(line, "*** Add File: ")
			out = append(out, "--- /dev/null")
			out = append(out, "+++ b/"+f)
		case strings.HasPrefix(line, "*** Update File: "):
			f := strings.TrimPrefix(line, "*** Update File: ")
			out = append(out, "--- a/"+f)
			out = append(out, "+++ b/"+f)
		case strings.HasPrefix(line, "*** Delete File: "):
			f := strings.TrimPrefix(line, "*** Delete File: ")
			out = append(out, "--- a/"+f)
			out = append(out, "+++ /dev/null")
		case strings.HasPrefix(line, "*** End of File"):
			// skip — separator between files in multi-file patches
		default:
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
