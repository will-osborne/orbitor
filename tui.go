package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gorilla/websocket"
)

// ── colour palette ────────────────────────────────────────────────────────────

var (
	colGreen  = lipgloss.Color("42")
	colOrange = lipgloss.Color("214")
	colYellow = lipgloss.Color("220")
	colRed    = lipgloss.Color("196")
	colCyan   = lipgloss.Color("39")
	colGray   = lipgloss.Color("240")
	colMuted  = lipgloss.Color("244")
	colText   = lipgloss.Color("252")
	colSep    = lipgloss.Color("237")

	styleGreen  = lipgloss.NewStyle().Foreground(colGreen).Bold(true)
	styleOrange = lipgloss.NewStyle().Foreground(colOrange).Bold(true)
	styleYellow = lipgloss.NewStyle().Foreground(colYellow).Bold(true)
	styleRed    = lipgloss.NewStyle().Foreground(colRed).Bold(true)
	styleCyan   = lipgloss.NewStyle().Foreground(colCyan).Bold(true)
	styleGray   = lipgloss.NewStyle().Foreground(colGray)
	styleMuted  = lipgloss.NewStyle().Foreground(colMuted)
	styleText   = lipgloss.NewStyle().Foreground(colText)
	styleSep    = lipgloss.NewStyle().Foreground(colSep)
	styleLabel  = lipgloss.NewStyle().Foreground(colMuted)
)

// ── spinner ───────────────────────────────────────────────────────────────────

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// ── wizard options ────────────────────────────────────────────────────────────

var (
	wizardBackends      = []string{"copilot", "claude"}
	wizardCopilotModels = []string{
		"(default)",
		"gpt-4o",
		"gpt-4o-mini",
		"o1",
		"o3-mini",
		"o4-mini",
	}
	wizardClaudeModels = []string{
		"(default)",
		"claude-opus-4-6",
		"claude-sonnet-4-6",
		"claude-haiku-4-5",
		"claude-opus-4-5",
		"claude-sonnet-4-5",
	}
	wizardSkipOptions = []string{
		"no  –  ask for each permission",
		"yes  –  skip all  (dangerous!)",
	}
)

func stateStyle(state string) lipgloss.Style {
	switch state {
	case "idle":
		return styleGreen
	case "working":
		return styleOrange
	case "waiting-input":
		return styleYellow
	case "error":
		return styleRed
	case "starting":
		return styleCyan
	default:
		return styleGray
	}
}

// ── API client ────────────────────────────────────────────────────────────────

type tuiAPIClient struct {
	baseURL string
	http    *http.Client
}

func newTUIAPIClient(baseURL string) (*tuiAPIClient, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("server URL is required")
	}
	if _, err := url.Parse(baseURL); err != nil {
		return nil, fmt.Errorf("invalid server URL: %w", err)
	}
	return &tuiAPIClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (c *tuiAPIClient) wsBase() (string, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	default:
		u.Scheme = "ws"
	}
	return u.String(), nil
}

func (c *tuiAPIClient) listSessions() ([]WSSessionInfo, error) {
	resp, err := c.http.Get(c.baseURL + "/api/sessions")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list sessions failed: %s", strings.TrimSpace(string(body)))
	}
	var out []WSSessionInfo
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *tuiAPIClient) missionSummary() (map[string]string, error) {
	resp, err := c.http.Get(c.baseURL + "/api/mission-summary")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("mission summary failed: %s", strings.TrimSpace(string(body)))
	}
	var out map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *tuiAPIClient) createSession(workingDir, backend, model string, skipPermissions bool) (WSSessionInfo, error) {
	payload := map[string]any{
		"workingDir":      workingDir,
		"backend":         backend,
		"skipPermissions": skipPermissions,
	}
	if strings.TrimSpace(model) != "" {
		payload["model"] = strings.TrimSpace(model)
	}
	body, _ := json.Marshal(payload)
	resp, err := c.http.Post(c.baseURL+"/api/sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		return WSSessionInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		msg, _ := io.ReadAll(resp.Body)
		return WSSessionInfo{}, fmt.Errorf("create session failed: %s", strings.TrimSpace(string(msg)))
	}
	var out WSSessionInfo
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return WSSessionInfo{}, err
	}
	return out, nil
}

func (c *tuiAPIClient) deleteSession(id string) error {
	req, _ := http.NewRequest(http.MethodDelete, c.baseURL+"/api/sessions/"+id, nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete session failed: %s", strings.TrimSpace(string(msg)))
	}
	return nil
}

func (c *tuiAPIClient) updateSessionSkipPermissions(id string, skip bool) (WSSessionInfo, error) {
	body, _ := json.Marshal(map[string]any{"skipPermissions": skip})
	req, _ := http.NewRequest(http.MethodPatch, c.baseURL+"/api/sessions/"+id, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return WSSessionInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return WSSessionInfo{}, fmt.Errorf("update session failed: %s", strings.TrimSpace(string(msg)))
	}
	var out WSSessionInfo
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return WSSessionInfo{}, err
	}
	return out, nil
}

// ── message types ─────────────────────────────────────────────────────────────

type sessionsMsg struct {
	sessions       []WSSessionInfo
	err            error
	missionTitle   string
	missionSummary string
}
type attachMsg struct {
	sessionID string
	conn      *websocket.Conn
	err       error
}
type createSessionMsg struct {
	session WSSessionInfo
	err     error
}
type deleteSessionMsg struct {
	id  string
	err error
}
type updateSessionMsg struct {
	session WSSessionInfo
	err     error
}
type wsPayloadMsg struct{ payload []byte }
type infoMsg struct{ text string }
type errMsg struct{ err error }
type tickMsg time.Time
type spinnerTickMsg time.Time

// ── model ─────────────────────────────────────────────────────────────────────

type tuiModel struct {
	api *tuiAPIClient

	width  int
	height int

	sessions        []WSSessionInfo
	selected        int
	activeSessionID string

	logs []string

	input    textinput.Model
	viewport viewport.Model

	extCh chan tea.Msg

	connMu sync.Mutex
	conn   *websocket.Conn

	// spinner
	spinnerFrame int

	// input history — up/down cycles when input is non-empty
	inputHistory []string // oldest first, max 100
	historyPos   int      // 0 = live input; 1 = most recent; 2 = second most recent…
	historyLive  string   // saves live input when browsing history

	// per-session elapsed-time tracking
	sessionStateStart map[string]time.Time
	sessionLastState  map[string]string

	// delete confirmation
	deleteConfirmID string // non-empty = awaiting y/N confirmation

	// cached mission summary from server
	missionTitle   string
	missionSummary string

	// reconnect state
	wsReconnecting   bool
	wsReconnectSince time.Time

	// new-session wizard
	wizardActive   bool
	wizardFocus    int // 0=dir, 1=backend, 2=model, 3=skip
	wizardBackend  int
	wizardModel    int
	wizardSkip     int
	wizardDirInput textinput.Model
}

func RunTUI(serverURL string, createNew bool, backend, model string, skip bool) error {
	api, err := newTUIAPIClient(serverURL)
	if err != nil {
		return err
	}

	in := textinput.New()
	in.Placeholder = "Type prompt or /help"
	in.CharLimit = 4000
	in.Focus()

	m := &tuiModel{
		api:               api,
		input:             in,
		viewport:          viewport.New(60, 20),
		extCh:             make(chan tea.Msg, 256),
		sessionStateStart: make(map[string]time.Time),
		sessionLastState:  make(map[string]string),
		historyPos:        0,
	}
	m.logSystem("Connected to " + api.baseURL)
	m.logSystem("↑/↓ navigate (or j/k empty input)  ·  Enter connect/send  ·  n new session  ·  Ctrl+D delete")
	m.logSystem("PgUp/PgDn scroll feed  ·  g/G top/bottom  ·  Ctrl+L clear  ·  Ctrl+R refresh  ·  Ctrl+C quit")
	m.logSystem("/help for all commands")

	if createNew {
		wd, err := os.Getwd()
		if err != nil {
			m.logSystem("failed to get cwd: " + err.Error())
		} else {
			created, err := api.createSession(wd, backend, model, skip)
			if err != nil {
				m.logSystem("Create session failed: " + err.Error())
			} else {
				m.logSystem("Created session " + created.ID)
				wsBase, err := api.wsBase()
				if err != nil {
					m.logSystem("ws base error: " + err.Error())
				} else {
					wsURL := strings.TrimRight(wsBase, "/") + "/ws/sessions/" + created.ID
					conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
					if err != nil {
						m.logSystem("attach ws failed: " + err.Error())
					} else {
						m.swapConn(conn)
						m.activeSessionID = created.ID
						m.logSystem("Connected to session " + created.ID)
						go func() {
							for {
								_, payload, err := conn.ReadMessage()
								if err != nil {
									m.extCh <- infoMsg{text: "Session disconnected: " + err.Error()}
									return
								}
								m.extCh <- wsPayloadMsg{payload: payload}
							}
						}()
					}
				}
			}
		}
	}

	p := tea.NewProgram(m, tea.WithMouseCellMotion())
	_, err = p.Run()
	return err
}

// ── wizard helpers ────────────────────────────────────────────────────────────

func (m *tuiModel) openWizard() {
	wd, _ := os.Getwd()
	if len(m.sessions) > 0 && m.selected < len(m.sessions) {
		wd = m.sessions[m.selected].WorkingDir
	}
	in := textinput.New()
	in.Placeholder = "Working directory path"
	in.CharLimit = 500
	in.SetValue(wd)
	in.CursorEnd()
	in.Focus()
	m.wizardDirInput = in
	m.wizardFocus = 0
	m.wizardBackend = 0
	m.wizardModel = 0
	m.wizardSkip = 0
	m.wizardActive = true
}

func (m *tuiModel) wizardCurrentModels() []string {
	if m.wizardBackend == 1 {
		return wizardClaudeModels
	}
	return wizardCopilotModels
}

func (m *tuiModel) updateWizard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.closeConn()
		return m, tea.Quit
	case "esc":
		m.wizardActive = false
		return m, nil
	case "tab":
		m.wizardFocus = (m.wizardFocus + 1) % 4
		if m.wizardFocus == 0 {
			m.wizardDirInput.Focus()
		} else {
			m.wizardDirInput.Blur()
		}
		return m, nil
	case "shift+tab":
		m.wizardFocus = (m.wizardFocus + 3) % 4
		if m.wizardFocus == 0 {
			m.wizardDirInput.Focus()
		} else {
			m.wizardDirInput.Blur()
		}
		return m, nil
	case "up":
		switch m.wizardFocus {
		case 1:
			if m.wizardBackend > 0 {
				m.wizardBackend--
				m.wizardModel = 0
			}
		case 2:
			if m.wizardModel > 0 {
				m.wizardModel--
			}
		case 3:
			if m.wizardSkip > 0 {
				m.wizardSkip--
			}
		}
		return m, nil
	case "down":
		switch m.wizardFocus {
		case 1:
			if m.wizardBackend < len(wizardBackends)-1 {
				m.wizardBackend++
				m.wizardModel = 0
			}
		case 2:
			models := m.wizardCurrentModels()
			if m.wizardModel < len(models)-1 {
				m.wizardModel++
			}
		case 3:
			if m.wizardSkip < len(wizardSkipOptions)-1 {
				m.wizardSkip++
			}
		}
		return m, nil
	case "enter":
		return m.wizardCreate()
	}
	if m.wizardFocus == 0 {
		var cmd tea.Cmd
		m.wizardDirInput, cmd = m.wizardDirInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *tuiModel) wizardCreate() (tea.Model, tea.Cmd) {
	wd := strings.TrimSpace(m.wizardDirInput.Value())
	if wd == "" {
		wd, _ = os.Getwd()
	}
	if strings.HasPrefix(wd, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			if wd == "~" {
				wd = home
			} else {
				wd = filepath.Join(home, strings.TrimPrefix(wd, "~/"))
			}
		}
	}
	if !filepath.IsAbs(wd) {
		if abs, err := filepath.Abs(wd); err == nil {
			wd = abs
		}
	}
	backend := wizardBackends[m.wizardBackend]
	models := m.wizardCurrentModels()
	model := models[m.wizardModel]
	if model == "(default)" {
		model = ""
	}
	skip := m.wizardSkip == 1
	m.wizardActive = false
	return m, createSessionCmd(m.api, wd, backend, model, skip)
}

func (m *tuiModel) renderWizard() string {
	const wizW = 60

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	focusedLabel := lipgloss.NewStyle().Foreground(colCyan).Bold(true)
	dimLabel := lipgloss.NewStyle().Foreground(colMuted).Bold(true)
	selOpt := lipgloss.NewStyle().Foreground(colGreen).Bold(true)
	dimOpt := lipgloss.NewStyle().Foreground(colText)

	label := func(section int, text string) string {
		if m.wizardFocus == section {
			return focusedLabel.Render(text)
		}
		return dimLabel.Render(text)
	}

	var lines []string
	lines = append(lines, titleStyle.Render("  New Session"))
	lines = append(lines, "")

	// Dir
	lines = append(lines, label(0, "  Working Directory"))
	lines = append(lines, "  "+m.wizardDirInput.View())
	lines = append(lines, "")

	// Backend
	lines = append(lines, label(1, "  Backend"))
	for i, b := range wizardBackends {
		if i == m.wizardBackend {
			lines = append(lines, "    "+selOpt.Render("◉ "+b))
		} else {
			lines = append(lines, "    "+dimOpt.Render("○ "+b))
		}
	}
	lines = append(lines, "")

	// Model
	models := m.wizardCurrentModels()
	lines = append(lines, label(2, "  Model"))
	for i, mdl := range models {
		if i == m.wizardModel {
			lines = append(lines, "    "+selOpt.Render("◉ "+mdl))
		} else {
			lines = append(lines, "    "+dimOpt.Render("○ "+mdl))
		}
	}
	lines = append(lines, "")

	// Skip Permissions
	lines = append(lines, label(3, "  Skip Permissions"))
	for i, opt := range wizardSkipOptions {
		if i == m.wizardSkip {
			lines = append(lines, "    "+selOpt.Render("◉ "+opt))
		} else {
			lines = append(lines, "    "+dimOpt.Render("○ "+opt))
		}
	}
	lines = append(lines, "")
	lines = append(lines, styleMuted.Render("  Tab/Shift+Tab=next section  ↑/↓=select  Enter=create  Esc=cancel"))

	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colCyan).
		Padding(0, 1).
		Width(wizW).
		Render(strings.Join(lines, "\n"))

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel)
}

// ── Init ──────────────────────────────────────────────────────────────────────

func (m *tuiModel) Init() tea.Cmd {
	return tea.Batch(
		refreshSessionsCmd(m.api),
		waitExternalCmd(m.extCh),
		tickCmd(),
		spinnerTickCmd(),
	)
}

// ── Update ────────────────────────────────────────────────────────────────────

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil

	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.viewport.ScrollUp(3)
		case tea.MouseButtonWheelDown:
			m.viewport.ScrollDown(3)
		}
		return m, nil

	case spinnerTickMsg:
		m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
		return m, spinnerTickCmd()

	case tickMsg:
		return m, tea.Batch(refreshSessionsCmd(m.api), tickCmd())

	case sessionsMsg:
		if msg.err != nil {
			m.logSystem("Refresh failed: " + msg.err.Error())
			return m, nil
		}
		// Track state changes to measure elapsed time per session.
		for _, s := range msg.sessions {
			state := sessionStateLabel(s)
			if m.sessionLastState[s.ID] != state {
				m.sessionLastState[s.ID] = state
				m.sessionStateStart[s.ID] = time.Now()
			}
		}
			m.sessions = msg.sessions
		m.missionTitle = msg.missionTitle
		m.missionSummary = msg.missionSummary
		if m.selected >= len(m.sessions) {
			m.selected = max(0, len(m.sessions)-1)
		}
		return m, nil

	case attachMsg:
		if msg.err != nil {
			m.logSystem("Connect failed: " + msg.err.Error())
			return m, nil
		}
		m.swapConn(msg.conn)
		m.activeSessionID = msg.sessionID
		m.wsReconnecting = false
		m.logSystem("Connected to session " + msg.sessionID)
		return m, nil

	case createSessionMsg:
		if msg.err != nil {
			m.logSystem("Create failed: " + msg.err.Error())
			return m, nil
		}
		m.logSystem("Created session " + msg.session.ID)
		return m, tea.Batch(
			refreshSessionsCmd(m.api),
			attachSessionCmd(m.api, msg.session.ID, m.extCh),
		)

	case deleteSessionMsg:
		if msg.err != nil {
			m.logSystem("Delete failed: " + msg.err.Error())
			return m, nil
		}
		if m.activeSessionID == msg.id {
			m.closeConn()
			m.activeSessionID = ""
		}
		m.logSystem("Deleted session " + msg.id)
		return m, refreshSessionsCmd(m.api)

	case updateSessionMsg:
		if msg.err != nil {
			m.logSystem("Session update failed: " + msg.err.Error())
			return m, nil
		}
		m.logSystem(fmt.Sprintf("Updated session %s (skipPermissions=%v)", msg.session.ID, msg.session.SkipPermissions))
		return m, refreshSessionsCmd(m.api)

	case wsPayloadMsg:
		m.handleIncoming(msg.payload)
		return m, waitExternalCmd(m.extCh)

	case infoMsg:
		if strings.Contains(msg.text, "disconnected") {
			m.wsReconnecting = true
			m.wsReconnectSince = time.Now()
		}
		m.logSystem(msg.text)
		return m, waitExternalCmd(m.extCh)

	case errMsg:
		if msg.err != nil {
			m.log(styleRed.Render("✗ ") + msg.err.Error())
		}
		return m, waitExternalCmd(m.extCh)

	case tea.KeyMsg:
		// While in delete-confirm mode, only y/n/esc are meaningful.
		if m.deleteConfirmID != "" {
			switch msg.String() {
			case "y", "Y":
				id := m.deleteConfirmID
				m.deleteConfirmID = ""
				return m, deleteSessionCmd(m.api, id)
			default:
				m.deleteConfirmID = ""
				m.logSystem("Delete cancelled")
				return m, nil
			}
		}

		switch msg.String() {
		case "ctrl+c":
			m.closeConn()
			return m, tea.Quit

		case "ctrl+l":
			m.logs = nil
			m.rebuildViewport()
			return m, nil

		case "ctrl+d":
			if len(m.sessions) == 0 {
				return m, nil
			}
			m.deleteConfirmID = m.sessions[m.selected].ID
			return m, nil

		case "up":
			if m.historyPos > 0 || m.input.Value() != "" {
				// Browse history backwards.
				if m.historyPos == 0 {
					m.historyLive = m.input.Value()
				}
				if m.historyPos < len(m.inputHistory) {
					m.historyPos++
					m.input.SetValue(m.inputHistory[len(m.inputHistory)-m.historyPos])
					m.input.CursorEnd()
				}
				return m, nil
			}
			if m.selected > 0 {
				m.selected--
			}
			return m, nil

		case "down":
			if m.historyPos > 0 {
				// Browse history forwards.
				m.historyPos--
				if m.historyPos == 0 {
					m.input.SetValue(m.historyLive)
				} else {
					m.input.SetValue(m.inputHistory[len(m.inputHistory)-m.historyPos])
				}
				m.input.CursorEnd()
				return m, nil
			}
			if m.selected < len(m.sessions)-1 {
				m.selected++
			}
			return m, nil

		case "k":
			if m.input.Value() == "" {
				if m.selected > 0 {
					m.selected--
				}
				return m, nil
			}

		case "j":
			if m.input.Value() == "" {
				if m.selected < len(m.sessions)-1 {
					m.selected++
				}
				return m, nil
			}

		case "pgup":
			m.viewport.HalfPageUp()
			return m, nil

		case "pgdown":
			m.viewport.HalfPageDown()
			return m, nil

		case "g":
			if m.input.Value() == "" {
				m.viewport.GotoTop()
				return m, nil
			}

		case "G":
			if m.input.Value() == "" {
				m.viewport.GotoBottom()
				return m, nil
			}

		case "ctrl+r", "f5":
			return m, refreshSessionsCmd(m.api)

		case "n":
			if m.input.Value() == "" && len(m.sessions) > 0 {
				wd := m.sessions[m.selected].WorkingDir
				m.input.SetValue("/new " + wd + " ")
				m.input.CursorEnd()
				return m, nil
			}

		case "enter":
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				if len(m.sessions) == 0 {
					m.logSystem("No sessions available")
					return m, nil
				}
				return m, attachSessionCmd(m.api, m.sessions[m.selected].ID, m.extCh)
			}
			m.input.SetValue("")
			m.historyPos = 0
			m.historyLive = ""
			if strings.HasPrefix(text, "/") {
				return m, m.handleCommand(text)
			}
			// Push to input history (deduplicate consecutive identical entries).
			if len(m.inputHistory) == 0 || m.inputHistory[len(m.inputHistory)-1] != text {
				m.inputHistory = append(m.inputHistory, text)
				if len(m.inputHistory) > 100 {
					m.inputHistory = m.inputHistory[1:]
				}
			}
			if m.activeSessionID == "" {
				m.logSystem("Connect to a session first (select and press Enter)")
				return m, nil
			}
			return m, sendWSCmd(m, map[string]any{"type": "prompt", "text": text})
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m *tuiModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Starting TUI..."
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	panelStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")).
		Padding(0, 1)
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Bold(true)
	activeStyle := lipgloss.NewStyle().Foreground(colGreen)

	leftW := max(46, m.width*42/100)
	rightW := max(50, m.width-leftW)
	if leftW+rightW > m.width {
		rightW = m.width - leftW
	}

	// ── top bar ───────────────────────────────────────────────────────────────
	topBar := panelStyle.Width(m.width - 2).Height(3).Render(
		titleStyle.Render(" Copilot Bridge  Mission Control ") +
			"\n" +
			styleMuted.Render(fmt.Sprintf(
				"sessions=%d  selected=%s  connected=%s",
				len(m.sessions),
				m.selectedSessionID(),
				defaultString(m.activeSessionID, "none"),
			)),
	)
	topBarH := lipgloss.Height(topBar)

	// ── optional banners ──────────────────────────────────────────────────────
	var banners []string

	// Permission banner — shown whenever any session needs approval.
	if pendingSessions := m.pendingPermissionSessions(); len(pendingSessions) > 0 {
		lines := []string{styleYellow.Render("⚠ Permission approval required:")}
		for _, s := range pendingSessions {
			name := defaultString(s.Title, s.ID)
			lines = append(lines, styleMuted.Render("  "+name+"  ")+styleCyan.Render("/allow <requestId> <optionId>"))
		}
		banner := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colYellow).
			Padding(0, 1).
			Width(m.width - 2).
			Render(strings.Join(lines, "\n"))
		banners = append(banners, banner)
	}

	// Delete confirmation banner.
	if m.deleteConfirmID != "" {
		banner := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colRed).
			Padding(0, 1).
			Width(m.width - 2).
			Render(styleRed.Render("⚠ Delete session "+m.deleteConfirmID+"?") +
				styleMuted.Render("  y = confirm  ·  any other key = cancel"))
		banners = append(banners, banner)
	}

	bannerH := 0
	for _, b := range banners {
		bannerH += lipgloss.Height(b)
	}

	// ── status bar ────────────────────────────────────────────────────────────
	statusBar := styleMuted.Render(fmt.Sprintf(
		"  server: %s  ·  sessions: %d  ·  n=new  ctrl+d=delete  ctrl+l=clear  ctrl+c=quit",
		m.api.baseURL, len(m.sessions),
	))
	statusBarH := 1

	// ── body layout ───────────────────────────────────────────────────────────
	bodyH := max(10, m.height-topBarH-bannerH-statusBarH)
	detailsH := 9
	if bodyH < 20 {
		detailsH = 7
	}
	inputH := 4
	feedH := max(6, bodyH-detailsH-inputH)

	// Left panel: add mission summary if available
	var missionBlock string
	if strings.TrimSpace(m.missionTitle) != "" || strings.TrimSpace(m.missionSummary) != "" {
		mb := ""
		if m.missionTitle != "" {
			mb += styleText.Render(m.missionTitle) + "\n"
		}
		if m.missionSummary != "" {
			mb += styleMuted.Render(m.missionSummary) + "\n"
		}
		missionBlock = mb + "\n"
	}
	left := panelStyle.Width(leftW - 2).Height(bodyH).Render(
		titleStyle.Render(" Sessions ") + "\n" + missionBlock + m.renderMissionControl(selectedStyle, activeStyle),
	)

	detailBox := panelStyle.Width(rightW - 2).Height(detailsH).Render(
		titleStyle.Render(" Session Details ") + "\n" + m.renderDetails(),
	)

	// Feed header with connected session, scroll indicator, and reconnect state.
	feedHeader := titleStyle.Render(" Session Feed ")
	if m.wsReconnecting {
		elapsed := time.Since(m.wsReconnectSince).Round(time.Second)
		feedHeader += styleYellow.Render("  ⟳ reconnecting… " + elapsed.String())
	} else if m.activeSessionID != "" {
		feedHeader += styleMuted.Render("  connected: " + m.activeSessionID)
	} else {
		feedHeader += styleMuted.Render("  not connected")
	}
	if !m.viewport.AtBottom() {
		feedHeader += styleMuted.Render("  ↑ scrolled · PgUp/PgDn · G=bottom")
	}
	feedBox := panelStyle.Width(rightW - 2).Height(feedH).Render(feedHeader + "\n" + m.viewport.View())

	// Input box hint line reflects current context.
	hint := "Enter=send  ·  Enter(empty)=connect  ·  ↑/↓=history/navigate  ·  Ctrl+R=refresh"
	inputBox := panelStyle.Width(rightW - 2).Height(inputH).Render(
		styleCyan.Render(" ❯ ") + m.input.View() + "\n" +
			styleMuted.Render("  "+hint),
	)

	right := lipgloss.JoinVertical(lipgloss.Left, detailBox, feedBox, inputBox)
	mainRow := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	parts := []string{topBar}
	parts = append(parts, banners...)
	parts = append(parts, mainRow, statusBar)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// pendingPermissionSessions returns sessions with pending permission requests.
func (m *tuiModel) pendingPermissionSessions() []WSSessionInfo {
	var out []WSSessionInfo
	for _, s := range m.sessions {
		if s.PendingPermission {
			out = append(out, s)
		}
	}
	return out
}

// ── renderDetails ─────────────────────────────────────────────────────────────

func (m *tuiModel) renderDetails() string {
	if len(m.sessions) == 0 {
		return styleMuted.Render("No sessions")
	}
	s := m.sessions[m.selected]
	state := sessionStateLabel(s)

	stateStr := stateStyle(state).Render(state)
	if state == "working" {
		spinner := styleOrange.Render(spinnerFrames[m.spinnerFrame])
		elapsed := formatElapsed(time.Since(m.sessionStateStart[s.ID]))
		stateStr = spinner + " " + styleOrange.Render("working") + styleMuted.Render(" "+elapsed)
	} else if state == "starting" {
		spinner := styleCyan.Render(spinnerFrames[m.spinnerFrame])
		elapsed := formatElapsed(time.Since(m.sessionStateStart[s.ID]))
		stateStr = spinner + " " + styleCyan.Render("starting") + styleMuted.Render(" "+elapsed)
	}

	backendStr := styleCyan.Render(s.Backend)
	modelStr := styleText.Render(defaultString(s.Model, "(default)"))

	runStr := styleText.Render("false")
	if s.IsRunning {
		runStr = styleOrange.Render("true")
	}
	permStr := styleText.Render("false")
	if s.PendingPermission {
		permStr = styleYellow.Render("true")
	}
	skipStr := styleText.Render("false")
	if s.SkipPermissions {
		skipStr = styleRed.Render("true")
	}
	tool := defaultString(s.CurrentTool, "-")
	toolStr := styleText.Render(tool)
	if tool != "-" {
		toolStr = styleOrange.Render(tool)
	}

	lbl := func(s string) string { return styleLabel.Render(s) }

	lines := []string{
		lbl("id:      ") + styleText.Render(s.ID),
		lbl("state:   ") + stateStr + lbl("  status: ") + styleText.Render(s.Status),
		lbl("backend: ") + backendStr + lbl("  model: ") + modelStr,
		lbl("skip:    ") + skipStr + lbl("  pending: ") + permStr + lbl("  running: ") + runStr,
		lbl("tool:    ") + toolStr,
		lbl("msg:     ") + styleText.Render(trimForLine(defaultString(s.LastMessage, "-"), 120)),
		lbl("dir:     ") + styleText.Render(s.WorkingDir),
	}
	if s.Title != "" {
		lines = append(lines, lbl("title:   ")+styleText.Render(trimForLine(s.Title, 80)))
	}
	if s.Summary != "" {
		lines = append(lines, lbl("summary: ")+styleText.Render(trimForLine(s.Summary, 80)))
	}
	return strings.Join(lines, "\n")
}

// ── handleCommand ─────────────────────────────────────────────────────────────

func (m *tuiModel) handleCommand(raw string) tea.Cmd {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return nil
	}
	switch fields[0] {
	case "/help":
		m.logSystem("Commands:")
		m.logSystem("  /refresh")
		m.logSystem("  /use <sessionId>")
		m.logSystem("  /new <workingDir> [backend] [model] [skipPermissions(true|false)]")
		m.logSystem("  /interrupt")
		m.logSystem("  /allow <requestId> <optionId>")
		m.logSystem("  /skip [true|false] [id]")
		m.logSystem("  /delete [id]")
		m.logSystem("  /quit")
		m.logSystem("Hotkeys: n=new session  ctrl+d=delete  ctrl+l=clear feed  PgUp/PgDn=scroll  g/G=top/bottom")
		return nil
	case "/refresh":
		return refreshSessionsCmd(m.api)
	case "/use":
		if len(fields) < 2 {
			m.logSystem("Usage: /use <sessionId>")
			return nil
		}
		return attachSessionCmd(m.api, fields[1], m.extCh)
	case "/new":
		if len(fields) < 2 {
			m.logSystem("Usage: /new <workingDir> [backend] [model] [skipPermissions]")
			return nil
		}
		wd := fields[1]
		if strings.HasPrefix(wd, "~") {
			if home, err := os.UserHomeDir(); err == nil {
				if wd == "~" {
					wd = home
				} else {
					wd = filepath.Join(home, strings.TrimPrefix(wd, "~/"))
				}
			}
		}
		if !filepath.IsAbs(wd) {
			if abs, err := filepath.Abs(wd); err == nil {
				wd = abs
			} else {
				m.logSystem(fmt.Sprintf("Invalid workingDir '%s': %v", wd, err))
				return nil
			}
		}
		backend := "copilot"
		model := ""
		skip := false
		if len(fields) >= 3 {
			backend = fields[2]
		}
		if len(fields) >= 4 {
			model = fields[3]
		}
		if len(fields) >= 5 {
			if v, err := strconv.ParseBool(fields[4]); err == nil {
				skip = v
			}
		}
		return createSessionCmd(m.api, wd, backend, model, skip)
	case "/interrupt":
		return sendWSCmd(m, map[string]any{"type": "interrupt"})
	case "/allow":
		if len(fields) < 3 {
			m.logSystem("Usage: /allow <requestId> <optionId>")
			return nil
		}
		return sendWSCmd(m, map[string]any{
			"type":      "permission_response",
			"requestId": fields[1],
			"optionId":  fields[2],
		})
	case "/skip":
		if len(m.sessions) == 0 {
			m.logSystem("No sessions available")
			return nil
		}
		target := m.sessions[m.selected]
		next := !target.SkipPermissions
		if len(fields) >= 2 {
			if v, err := strconv.ParseBool(fields[1]); err == nil {
				next = v
			}
		}
		if len(fields) >= 3 {
			if s, ok := m.findSessionByID(fields[2]); ok {
				target = s
			} else {
				m.logSystem("Session not found: " + fields[2])
				return nil
			}
		}
		return updateSessionCmd(m.api, target.ID, next)
	case "/delete":
		if len(m.sessions) == 0 {
			m.logSystem("No sessions available")
			return nil
		}
		targetID := m.sessions[m.selected].ID
		if len(fields) >= 2 {
			targetID = fields[1]
		}
		return deleteSessionCmd(m.api, targetID)
	case "/quit", "/exit":
		m.closeConn()
		return tea.Quit
	default:
		m.logSystem("Unknown command, use /help")
		return nil
	}
}

// ── handleIncoming ────────────────────────────────────────────────────────────

func (m *tuiModel) handleIncoming(payload []byte) {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(payload, &probe); err != nil {
		return
	}
	if probe.Type == "history" {
		var h WSHistoryMessage
		if err := json.Unmarshal(payload, &h); err != nil {
			return
		}
		for _, it := range h.Messages {
			m.renderMessage(it)
		}
		return
	}
	var msg WSMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		return
	}
	m.renderMessage(msg)
}

// ── renderMessage ─────────────────────────────────────────────────────────────

func (m *tuiModel) renderMessage(msg WSMessage) {
	ts := styleMuted.Render(time.Now().Format("15:04") + " ")

	switch msg.Type {
	case "prompt_sent":
		var d struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(msg.Data, &d) == nil {
			m.log("")
			m.log(ts + styleCyan.Render("❯ ") + styleText.Render(d.Text))
		}

	case "agent_text":
		var d WSAgentText
		if json.Unmarshal(msg.Data, &d) == nil {
			m.log(ts + styleText.Render(d.Text))
		}

	case "tool_call":
		var d WSToolCall
		if json.Unmarshal(msg.Data, &d) == nil {
			var icon string
			var titleStyle lipgloss.Style
			switch d.Status {
			case "success", "done":
				icon = "✓"
				titleStyle = styleGreen
			case "error":
				icon = "✗"
				titleStyle = styleRed
			default:
				icon = "⚙"
				titleStyle = styleOrange
			}
			line := ts + titleStyle.Render(icon+" "+d.Title) + styleMuted.Render(" ("+d.Kind+")")
			m.log(line)
			if d.Content != "" {
				// Show first line; if content has more lines hint at it.
				lines := strings.SplitN(strings.TrimSpace(d.Content), "\n", 3)
				m.log(styleMuted.Render("  → ") + trimForLine(lines[0], 120))
				if len(lines) > 1 {
					remaining := strings.Count(d.Content, "\n")
					m.log(styleMuted.Render(fmt.Sprintf("  [+%d lines]", remaining)))
				}
			}
		}

	case "tool_result":
		var d WSToolResult
		if json.Unmarshal(msg.Data, &d) == nil && d.Content != "" {
			lines := strings.SplitN(strings.TrimSpace(d.Content), "\n", 3)
			m.log(styleMuted.Render("  ↳ ") + trimForLine(lines[0], 120))
			if len(lines) > 1 {
				remaining := strings.Count(d.Content, "\n")
				m.log(styleMuted.Render(fmt.Sprintf("  [+%d lines]", remaining)))
			}
		}

	case "permission_request":
		var d WSPermissionRequest
		if json.Unmarshal(msg.Data, &d) == nil {
			m.log("")
			m.log(ts + styleYellow.Render("⚠ Permission Required: ") + styleText.Render(d.Title))
			if d.Command != "" {
				m.log(styleMuted.Render("  cmd: ") + styleText.Render(d.Command))
			}
			for _, o := range d.Options {
				m.log(styleCyan.Render("  ["+o.OptionID+"] ") + o.Name + styleMuted.Render(" ("+o.Kind+")"))
			}
			m.log(styleMuted.Render("  Use: /allow " + d.RequestID + " <optionId>"))
		}

	case "permission_resolved":
		var d struct {
			RequestID string `json:"requestId"`
			OptionID  string `json:"optionId"`
		}
		if json.Unmarshal(msg.Data, &d) == nil {
			m.log(ts + styleGreen.Render("✓ Approved ") + styleMuted.Render(d.RequestID+" → "+d.OptionID))
		}

	case "run_complete":
		var d struct {
			StopReason string `json:"stopReason"`
		}
		if json.Unmarshal(msg.Data, &d) == nil {
			m.log("")
			m.log(ts + styleGreen.Render("✓ Done") + styleMuted.Render("  "+d.StopReason))
		}

	case "status":
		var d struct {
			Status string `json:"status"`
		}
		if json.Unmarshal(msg.Data, &d) == nil && d.Status != "" {
			m.log(styleMuted.Render("  ∙ " + d.Status))
		}

	case "error":
		var d WSError
		if json.Unmarshal(msg.Data, &d) == nil {
			m.log(ts + styleRed.Render("✗ Error: ") + d.Message)
		}
	}
}

// ── layout helpers ────────────────────────────────────────────────────────────

func (m *tuiModel) resize() {
	leftW := max(46, m.width*42/100)
	rightW := max(50, m.width-leftW)
	vw := max(24, rightW-8)
	bodyH := max(10, m.height-8)
	detailsH := 9
	if bodyH < 20 {
		detailsH = 7
	}
	inputH := 4
	feedH := max(6, bodyH-detailsH-inputH)
	vh := max(4, feedH-3)
	m.viewport.Width = vw
	m.viewport.Height = vh
	m.rebuildViewport()
}

func (m *tuiModel) rebuildViewport() {
	atBottom := m.viewport.AtBottom()
	m.viewport.SetContent(strings.Join(m.logs, "\n"))
	if atBottom {
		m.viewport.GotoBottom()
	}
}

func (m *tuiModel) log(s string) {
	m.logs = append(m.logs, s)
	if len(m.logs) > 4000 {
		m.logs = m.logs[len(m.logs)-4000:]
	}
	m.rebuildViewport()
}

// logSystem renders a muted system/info line (no timestamp prefix).
func (m *tuiModel) logSystem(s string) {
	m.log(styleMuted.Render("  · " + s))
}

// ── connection helpers ────────────────────────────────────────────────────────

func (m *tuiModel) swapConn(conn *websocket.Conn) {
	m.connMu.Lock()
	prev := m.conn
	m.conn = conn
	m.connMu.Unlock()
	if prev != nil {
		_ = prev.Close()
	}
}

func (m *tuiModel) closeConn() {
	m.connMu.Lock()
	c := m.conn
	m.conn = nil
	m.connMu.Unlock()
	if c != nil {
		_ = c.Close()
	}
}

// ── commands ──────────────────────────────────────────────────────────────────

func refreshSessionsCmd(api *tuiAPIClient) tea.Cmd {
	return func() tea.Msg {
		s, err := api.listSessions()
		missionTitle := ""
		missionSummary := ""
		if err == nil {
			if m, merr := api.missionSummary(); merr == nil {
				missionTitle = m["title"]
				missionSummary = m["summary"]
			}
		}
		return sessionsMsg{sessions: s, err: err, missionTitle: missionTitle, missionSummary: missionSummary}
	}
}

func createSessionCmd(api *tuiAPIClient, wd, backend, model string, skip bool) tea.Cmd {
	return func() tea.Msg {
		created, err := api.createSession(wd, backend, model, skip)
		return createSessionMsg{session: created, err: err}
	}
}

func deleteSessionCmd(api *tuiAPIClient, id string) tea.Cmd {
	return func() tea.Msg {
		err := api.deleteSession(id)
		return deleteSessionMsg{id: id, err: err}
	}
}

func updateSessionCmd(api *tuiAPIClient, id string, skip bool) tea.Cmd {
	return func() tea.Msg {
		updated, err := api.updateSessionSkipPermissions(id, skip)
		return updateSessionMsg{session: updated, err: err}
	}
}

func attachSessionCmd(api *tuiAPIClient, sessionID string, extCh chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		wsBase, err := api.wsBase()
		if err != nil {
			return attachMsg{sessionID: sessionID, err: err}
		}
		wsURL := strings.TrimRight(wsBase, "/") + "/ws/sessions/" + sessionID
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			return attachMsg{sessionID: sessionID, err: err}
		}
		go func() {
			for {
				_, payload, err := conn.ReadMessage()
				if err != nil {
					extCh <- infoMsg{text: "Session disconnected: " + err.Error()}
					return
				}
				extCh <- wsPayloadMsg{payload: payload}
			}
		}()
		return attachMsg{sessionID: sessionID, conn: conn}
	}
}

func sendWSCmd(m *tuiModel, payload map[string]any) tea.Cmd {
	return func() tea.Msg {
		m.connMu.Lock()
		conn := m.conn
		m.connMu.Unlock()
		if conn == nil {
			return infoMsg{text: "No active session connection"}
		}
		if err := conn.WriteJSON(payload); err != nil {
			return errMsg{err: err}
		}
		return nil
	}
}

func waitExternalCmd(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(4*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func spinnerTickCmd() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(t time.Time) tea.Msg {
		return spinnerTickMsg(t)
	})
}

// ── renderMissionControl ──────────────────────────────────────────────────────

func (m *tuiModel) renderMissionControl(selectedStyle, activeStyle lipgloss.Style) string {
	if len(m.sessions) == 0 {
		return styleMuted.Render("No sessions")
	}

	linesPerSession := 3 // 2 content lines + 1 separator
	availableRows := max(3, max(10, m.height-8)-3)
	maxSessions := max(1, availableRows/linesPerSession)
	start := 0
	if m.selected >= maxSessions {
		start = m.selected - maxSessions + 1
	}
	end := min(len(m.sessions), start+maxSessions)

	var out []string
	for i := start; i < end; i++ {
		s := m.sessions[i]
		state := sessionStateLabel(s)
		isSelected := i == m.selected
		isActive := s.ID == m.activeSessionID

		prefix := "  "
		if isSelected {
			prefix = "▶ "
		} else if isActive {
			prefix = "● "
		}

		backendModel := s.Backend
		if s.Model != "" {
			backendModel += ":" + trimForLine(s.Model, 16)
		}

		subtitle := trimForLine(defaultString(s.Title, defaultString(s.LastMessage, "—")), 48)

		if isSelected {
			topLine := selectedStyle.Render(" " + prefix + s.ID + "  " + state + "  " + backendModel + " ")
			botLine := selectedStyle.Render("   " + shortPath(s.WorkingDir) + "  " + subtitle + " ")
			sep := styleSep.Render(strings.Repeat("─", 42))
			out = append(out, topLine, botLine, sep)
		} else {
			// Animated spinner for active states.
			var stateTag string
			switch state {
			case "working":
				stateTag = styleOrange.Render(spinnerFrames[m.spinnerFrame] + " working")
				if elapsed := m.sessionStateStart[s.ID]; !elapsed.IsZero() {
					stateTag += styleMuted.Render(" " + formatElapsed(time.Since(elapsed)))
				}
			case "starting":
				stateTag = styleCyan.Render(spinnerFrames[m.spinnerFrame] + " starting")
			case "waiting-input":
				stateTag = styleYellow.Render("⚠ waiting-input")
			default:
				stateTag = stateStyle(state).Render(state)
			}

			topLine := prefix + s.ID + "  " + stateTag + "  " + styleMuted.Render(backendModel)
			botLine := "   " + styleMuted.Render(shortPath(s.WorkingDir)+"  "+subtitle)
			sep := styleSep.Render(strings.Repeat("─", 42))
			if isActive {
				topLine = activeStyle.Render(topLine)
			}
			out = append(out, topLine, botLine, sep)
		}
	}

	if start > 0 {
		out = append([]string{styleMuted.Render("  ↑ more")}, out...)
	}
	if end < len(m.sessions) {
		out = append(out, styleMuted.Render("  ↓ more"))
	}
	return strings.Join(out, "\n")
}

// ── small helpers ─────────────────────────────────────────────────────────────

func shortPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	path = strings.TrimSuffix(path, "/")
	if i := strings.LastIndex(path, "/"); i >= 0 && i < len(path)-1 {
		return path[i+1:]
	}
	return path
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func trimForLine(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}

func formatElapsed(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}

func sessionStateLabel(s WSSessionInfo) string {
	if s.Status == "starting" {
		return "starting"
	}
	if s.Status == "error" {
		return "error"
	}
	if s.Status != "ready" {
		return "offline"
	}
	if s.PendingPermission {
		return "waiting-input"
	}
	if s.IsRunning {
		return "working"
	}
	return "idle"
}

func (m *tuiModel) findSessionByID(id string) (WSSessionInfo, bool) {
	for _, s := range m.sessions {
		if s.ID == id {
			return s, true
		}
	}
	return WSSessionInfo{}, false
}

func (m *tuiModel) selectedSessionID() string {
	if len(m.sessions) == 0 || m.selected >= len(m.sessions) {
		return "none"
	}
	return m.sessions[m.selected].ID
}
