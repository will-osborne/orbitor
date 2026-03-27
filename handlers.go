package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

type Handlers struct {
	sm       *SessionManager
	upgrader websocket.Upgrader
	fcm      *FCMSender // may be nil if not configured
}

func NewHandlers(sm *SessionManager, fcm *FCMSender) *Handlers {
	return &Handlers{
		sm:  sm,
		fcm: fcm,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (h *Handlers) RegisterDeviceToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Platform string `json:"platform"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		http.Error(w, `{"error":"token required"}`, http.StatusBadRequest)
		return
	}
	platform := req.Platform
	if platform == "" {
		platform = "android"
	}
	if h.sm.store == nil {
		http.Error(w, `{"error":"store unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	if err := h.sm.store.UpsertDeviceToken(req.Token, platform); err != nil {
		http.Error(w, `{"error":"failed to register token"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "registered"})
}

func (h *Handlers) ListSessions(w http.ResponseWriter, r *http.Request) {
	sessions := h.sm.List()
	if sessions == nil {
		sessions = []WSSessionInfo{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}

func (h *Handlers) CreateSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WorkingDir      string `json:"workingDir"`
		Backend         string `json:"backend"`
		Model           string `json:"model"`
		SkipPermissions bool   `json:"skipPermissions"`
		PlanMode        bool   `json:"planMode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if req.WorkingDir == "" {
		http.Error(w, `{"error":"workingDir required"}`, http.StatusBadRequest)
		return
	}

	session, err := h.sm.Create(req.WorkingDir, req.Backend, req.Model, req.SkipPermissions, req.PlanMode)
	if err != nil {
		log.Printf("create session error: %v", err)
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(WSSessionInfo{
		ID:              session.ID,
		WorkingDir:      session.WorkingDir,
		Status:          session.Status,
		Backend:         session.Backend,
		Model:           session.Model,
		SkipPermissions: session.SkipPermissions,
		PlanMode:        session.PlanMode,
	})
}

func (h *Handlers) CloneSessionAndPrompt(w http.ResponseWriter, r *http.Request) {
	sourceID := r.PathValue("id")
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" {
		http.Error(w, `{"error":"text required"}`, http.StatusBadRequest)
		return
	}

	cloned, err := h.sm.Clone(sourceID)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
		return
	}

	if err := cloned.QueuePromptWhenReady(req.Text, 20*time.Second); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(WSSessionInfo{
		ID:              cloned.ID,
		WorkingDir:      cloned.WorkingDir,
		Status:          cloned.Status,
		Backend:         cloned.Backend,
		Model:           cloned.Model,
		SkipPermissions: cloned.SkipPermissions,
		PlanMode:        cloned.PlanMode,
	})
}

func (h *Handlers) DeleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.sm.Delete(id); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) UpdateSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		SkipPermissions *bool   `json:"skipPermissions,omitempty"`
		PlanMode        *bool   `json:"planMode,omitempty"`
		Model           *string `json:"model,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	session, err := h.sm.UpdateSession(id, req.SkipPermissions, req.PlanMode, req.Model)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(WSSessionInfo{
		ID:              session.ID,
		WorkingDir:      session.WorkingDir,
		ACPSession:      session.ACPSession,
		Status:          session.Status,
		Backend:         session.Backend,
		Model:           session.Model,
		SkipPermissions: session.SkipPermissions,
		PlanMode:        session.PlanMode,
	})
}

func (h *Handlers) KillSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.sm.KillSession(id); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) ReviveSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.sm.ReviveSession(id); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) PollNotifications(w http.ResponseWriter, r *http.Request) {
	after := int64(0)
	limit := 50
	if s := r.URL.Query().Get("after"); s != "" {
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil || v < 0 {
			http.Error(w, `{"error":"invalid after"}`, http.StatusBadRequest)
			return
		}
		after = v
	}
	if s := r.URL.Query().Get("limit"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v <= 0 || v > 200 {
			http.Error(w, `{"error":"invalid limit"}`, http.StatusBadRequest)
			return
		}
		limit = v
	}
	if h.sm.store == nil {
		http.Error(w, `{"error":"store unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	events, err := h.sm.store.ListNotificationsAfter(after, limit)
	if err != nil {
		http.Error(w, `{"error":"failed to fetch notifications"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"events": events,
	})
}

func (h *Handlers) BrowseDir(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("path")
	if dir == "" {
		wd, err := os.Getwd()
		if err == nil {
			dir = wd
		} else {
			dir, _ = os.UserHomeDir()
		}
	}
	dir = filepath.Clean(dir)

	entries, err := os.ReadDir(dir)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	type entry struct {
		Name  string `json:"name"`
		IsDir bool   `json:"isDir"`
		Path  string `json:"path"`
	}

	var dirs []entry
	for _, e := range entries {
		if e.Name()[0] == '.' {
			continue // skip hidden
		}
		if e.IsDir() {
			dirs = append(dirs, entry{
				Name:  e.Name(),
				IsDir: true,
				Path:  filepath.Join(dir, e.Name()),
			})
		}
	}
	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].Name < dirs[j].Name
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"path":    dir,
		"parent":  filepath.Dir(dir),
		"entries": dirs,
	})
}

// SelfUpdate rebuilds the copilot-bridge binary (and optionally the Flutter web
// app) from source, then triggers a graceful upgrade via SIGHUP so the new
// binary takes over the listening socket with zero client downtime.
//
// POST /api/self-update
//
//	{"flutter": true}   — also rebuild Flutter web (optional)
func (h *Handlers) SelfUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Flutter bool `json:"flutter"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req) // body is optional

	exePath, err := os.Executable()
	if err != nil {
		http.Error(w, `{"error":"cannot determine executable path"}`, http.StatusInternalServerError)
		return
	}

	// Resolve symlinks so we overwrite the actual binary, not the symlink target.
	exePath, _ = filepath.EvalSymlinks(exePath)

	// If running via `go run`, the binary lives in a temp dir that gets cleaned up.
	// Fall back to building into the working directory so tableflip can find it.
	if strings.Contains(exePath, "go-build") || strings.Contains(exePath, os.TempDir()) {
		wd, _ := os.Getwd()
		exePath = filepath.Join(wd, "copilot-bridge")
		log.Printf("self-update: detected go run, building to %s instead", exePath)
	}

	log.Printf("self-update: building new binary → %s", exePath)

	buildCmd := exec.Command("go", "build", "-o", exePath, ".")
	var buildOut bytes.Buffer
	buildCmd.Stdout = &buildOut
	buildCmd.Stderr = &buildOut
	if err := buildCmd.Run(); err != nil {
		log.Printf("self-update: go build failed: %s", buildOut.String())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error":  "go build failed",
			"output": buildOut.String(),
		})
		return
	}

	var flutterOutput string
	if req.Flutter {
		log.Println("self-update: rebuilding Flutter web...")
		fCmd := exec.Command("flutter", "build", "web")
		fCmd.Dir = "mobile"
		var fOut bytes.Buffer
		fCmd.Stdout = &fOut
		fCmd.Stderr = &fOut
		if err := fCmd.Run(); err != nil {
			log.Printf("self-update: flutter build failed: %s", fOut.String())
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"error":  "flutter build web failed",
				"output": fOut.String(),
			})
			return
		}
		flutterOutput = fOut.String()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":        "built",
		"binary":        exePath,
		"buildOutput":   buildOut.String(),
		"flutterOutput": flutterOutput,
	})

	// Trigger the graceful upgrade after the response has been flushed.
	// SIGHUP is handled by the tableflip upgrader in main.go.
	go func() {
		time.Sleep(500 * time.Millisecond)
		log.Println("self-update: sending SIGHUP to trigger graceful upgrade")
		syscall.Kill(syscall.Getpid(), syscall.SIGHUP)
	}()
}

func (h *Handlers) SendAPK(w http.ResponseWriter, r *http.Request) {
	var req struct {
		To      string `json:"to"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if waClient == nil || !waClient.IsConnected() {
		http.Error(w, `{"error":"whatsapp not connected — pair first via POST /api/whatsapp/pair"}`, http.StatusServiceUnavailable)
		return
	}

	to := req.To
	if to == "" && AppConfig != nil {
		to = AppConfig.WhatsApp.DefaultRecipient
	}
	if to == "" {
		http.Error(w, `{"error":"recipient phone number required (set 'to' or configure default_recipient)"}`, http.StatusBadRequest)
		return
	}

	// Respond immediately so the caller doesn't block on the build.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})

	// Run the build and WhatsApp notifications in the background. Use a background
	// context because r.Context() will be cancelled once the response completes.
	go func(to, caption string) {
		ctx := context.Background()

		// Notify that the build has started.
		if err := waClient.SendText(ctx, to, "🔨 Build started..."); err != nil {
			log.Printf("send build started notification: %v", err)
		}

		mobileDir := filepath.Join("mobile")
		buildCmd := exec.Command("flutter", "build", "apk", "--release")
		buildCmd.Dir = mobileDir
		var buildOut bytes.Buffer
		buildCmd.Stdout = &buildOut
		buildCmd.Stderr = &buildOut

		// Start the build so we can send periodic heartbeats while it runs.
		if err := buildCmd.Start(); err != nil {
			log.Printf("failed to start build: %v", err)
			_ = waClient.SendText(ctx, to, "❌ Build start failed: "+err.Error())
			return
		}

		start := time.Now()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		done := make(chan error, 1)
		go func() { done <- buildCmd.Wait() }()

		for {
			select {
			case err := <-done:
				if err != nil {
					log.Printf("flutter build apk failed: %v\n%s", err, buildOut.String())
					_ = waClient.SendText(ctx, to, "❌ Build failed: "+err.Error())
					return
				}

				// Build succeeded — verify APK exists and send it.
				apkPath := filepath.Join(mobileDir, "build", "app", "outputs", "flutter-apk", "app-release.apk")
				info, statErr := os.Stat(apkPath)
				if statErr != nil || info.IsDir() {
					log.Printf("apk not found after build: %v", statErr)
					_ = waClient.SendText(ctx, to, "❌ APK not found after build")
					return
				}

				if sendErr := waClient.SendDocument(ctx, to, apkPath, caption); sendErr != nil {
					log.Printf("send document error: %v", sendErr)
					_ = waClient.SendText(ctx, to, "❌ Send failed: "+sendErr.Error())
					return
				}
				_ = waClient.SendText(ctx, to, "✅ Build and send complete (took "+time.Since(start).Truncate(time.Second).String()+")")
				return
			case <-ticker.C:
				// Periodic heartbeat so the user knows the build is still running.
				elapsed := time.Since(start).Truncate(time.Second)
				_ = waClient.SendText(ctx, to, fmt.Sprintf("⏳ Building... elapsed %s", elapsed))
			}
		}
	}(to, func() string {
		if req.Message != "" {
			return req.Message
		}
		return "Fresh build — " + time.Now().Format("2006-01-02 15:04")
	}())
}

func (h *Handlers) WhatsAppStatus(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"connected": false,
		"paired":    false,
	}
	if waClient != nil {
		resp["connected"] = waClient.IsConnected()
		resp["paired"] = waClient.IsPaired()
		if phone := waClient.PhoneNumber(); phone != "" {
			resp["phoneNumber"] = phone
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *Handlers) WhatsAppPair(w http.ResponseWriter, r *http.Request) {
	if waClient == nil {
		http.Error(w, `{"error":"whatsapp client not initialized"}`, http.StatusServiceUnavailable)
		return
	}
	if waClient.IsPaired() {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "already_paired"})
		return
	}

	qr, _, err := waClient.StartPairing(r.Context())
	if err != nil {
		log.Printf("whatsapp pair error: %v", err)
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "pairing", "qr": qr})
}

func (h *Handlers) WhatsAppQR(w http.ResponseWriter, r *http.Request) {
	if waClient == nil {
		http.Error(w, `{"error":"whatsapp client not initialized"}`, http.StatusServiceUnavailable)
		return
	}
	qr, pairing := waClient.CurrentQR()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"qr":      qr,
		"pairing": pairing,
		"paired":  waClient.IsPaired(),
	})
}

func (h *Handlers) WhatsAppLogout(w http.ResponseWriter, r *http.Request) {
	if waClient == nil {
		http.Error(w, `{"error":"whatsapp client not initialized"}`, http.StatusServiceUnavailable)
		return
	}
	if err := waClient.Logout(r.Context()); err != nil {
		log.Printf("whatsapp logout error: %v", err)
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "logged_out"})
}

// MissionSummary returns an aggregated AI summary for the Mission Control
// dashboard. It gathers concise fields from all sessions and asks the shared
// Summarizer to produce a small JSON {title, summary} object. If the
// summarizer isn't configured, return 503.
func (h *Handlers) MissionSummary(w http.ResponseWriter, r *http.Request) {
	if h.sm == nil || h.sm.summarizer == nil {
		http.Error(w, `{"error":"summarizer not available"}`, http.StatusServiceUnavailable)
		return
	}

	sessions := h.sm.List()
	if sessions == nil {
		sessions = []WSSessionInfo{}
	}

	// Build a compact context from session fields most useful for overview.
	// Use at most the 12 most recent sessions to limit prompt size.
	maxn := 12
	if len(sessions) < maxn {
		maxn = len(sessions)
	}
	var parts []string
	for i := 0; i < maxn; i++ {
		s := sessions[i]
		parts = append(parts, fmt.Sprintf("- %s: %s | %s | %s", defaultString(s.Title, s.ID), defaultString(s.Summary, s.LastMessage), s.CurrentTool, s.Status))
	}
	context := strings.Join(parts, "\n")

	title, summary := h.sm.summarizer.SummarizeText(context)
	if title == "" && summary == "" {
		http.Error(w, `{"error":"summarization failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"title": title, "summary": summary})
}

// EnhancePrompt rewrites a rough prompt into a more precise instruction using
// the local LLM summarizer. POST /api/enhance-prompt {"text": "..."}.
func (h *Handlers) EnhancePrompt(w http.ResponseWriter, r *http.Request) {
	if h.sm == nil || h.sm.summarizer == nil {
		http.Error(w, `{"error":"summarizer not available"}`, http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Text) == "" {
		http.Error(w, `{"error":"text required"}`, http.StatusBadRequest)
		return
	}
	enhanced := h.sm.summarizer.EnhancePrompt(req.Text)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"enhanced": enhanced})
}

// SessionDebrief returns a post-run summary of a session's conversation.
// GET /api/sessions/{id}/debrief
func (h *Handlers) SessionDebrief(w http.ResponseWriter, r *http.Request) {
	if h.sm == nil || h.sm.summarizer == nil {
		http.Error(w, `{"error":"summarizer not available"}`, http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	session := h.sm.Get(id)
	if session == nil {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		return
	}
	var msgs []WSMessage
	if session.store != nil {
		msgs, _ = session.store.LoadMessages(session.ID)
	}
	debrief := h.sm.summarizer.Debrief(msgs)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"debrief": debrief})
}

// SessionSuggestions returns up to 3 follow-up prompt suggestions for a session.
// GET /api/sessions/{id}/suggestions
func (h *Handlers) SessionSuggestions(w http.ResponseWriter, r *http.Request) {
	if h.sm == nil || h.sm.summarizer == nil {
		http.Error(w, `{"error":"summarizer not available"}`, http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	session := h.sm.Get(id)
	if session == nil {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		return
	}
	var msgs []WSMessage
	if session.store != nil {
		msgs, _ = session.store.LoadMessages(session.ID)
	}
	suggestions := h.sm.summarizer.Suggestions(msgs)
	if suggestions == nil {
		suggestions = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"suggestions": suggestions})
}

// EventsWebSocket is a global WebSocket endpoint that broadcasts cross-session
// events (permission requests) so the mobile app can show notifications
// regardless of which session is currently open.
func (h *Handlers) EventsWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	client := &Client{hub: h.sm.EventHub, conn: conn, send: make(chan []byte, 64)}
	h.sm.EventHub.register <- client
	go client.WritePump()
	// Read pump (drain incoming messages, detect close).
	go func() {
		defer func() {
			h.sm.EventHub.unregister <- client
			conn.Close()
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
}

func (h *Handlers) SessionWebSocket(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	session := h.sm.Get(id)
	if session == nil {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade error: %v", err)
		return
	}

	// Load complete history from DB and send it as the first message.
	// This ensures clients always receive full history regardless of the hub's
	// in-memory cache (capped at 500 and lost on server restart for live sessions).
	// We write directly to conn here — WritePump has not started yet so there is
	// no concurrent writer, making this safe.
	var historyMsgs []WSMessage
	if session.store != nil {
		historyMsgs, _ = session.store.LoadMessages(session.ID)
	}
	envelope := WSHistoryMessage{Type: "history", Messages: historyMsgs}
	if data, err := json.Marshal(envelope); err == nil {
		_ = conn.WriteMessage(websocket.TextMessage, data)
	}

	client := &Client{
		hub:             session.hub,
		conn:            conn,
		send:            make(chan []byte, 256),
		skipHistorySeed: true, // history already sent from DB above
	}

	session.hub.register <- client

	go client.WritePump()
	client.ReadPump(func(msg []byte) {
		var base struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(msg, &base); err != nil {
			return
		}

		switch base.Type {
		case "prompt":
			var p WSPrompt
			if err := json.Unmarshal(msg, &p); err != nil {
				return
			}
			session.SendPrompt(p.Text)

		case "interrupt":
			session.Interrupt()

		case "permission_response":
			var p WSPermissionResponse
			if err := json.Unmarshal(msg, &p); err != nil {
				return
			}
			session.RespondToPermission(p.RequestID, p.OptionID)
		}
	})
}
