package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

type tuiTheme struct {
	Name   string
	Green  lipgloss.Color
	Orange lipgloss.Color
	Yellow lipgloss.Color
	Red    lipgloss.Color
	Cyan   lipgloss.Color
	Violet lipgloss.Color
	Gray   lipgloss.Color
	Muted  lipgloss.Color
	Text   lipgloss.Color
	Sep    lipgloss.Color
	Border lipgloss.Color
	Accent lipgloss.Color
	SelBg  lipgloss.Color
	Panel  lipgloss.Color
}

var tuiThemes = []tuiTheme{
	{
		Name:   "opencode",
		Green:  lipgloss.Color("78"),
		Orange: lipgloss.Color("215"),
		Yellow: lipgloss.Color("221"),
		Red:    lipgloss.Color("203"),
		Cyan:   lipgloss.Color("81"),
		Violet: lipgloss.Color("147"),
		Gray:   lipgloss.Color("243"),
		Muted:  lipgloss.Color("246"),
		Text:   lipgloss.Color("252"),
		Sep:    lipgloss.Color("239"),
		Border: lipgloss.Color("238"),
		Accent: lipgloss.Color("117"),
		SelBg:  lipgloss.Color("239"),
		Panel:  lipgloss.Color("235"),
	},
	{
		Name:   "catppuccin",
		Green:  lipgloss.Color("#A6E3A1"),
		Orange: lipgloss.Color("#FAB387"),
		Yellow: lipgloss.Color("#F9E2AF"),
		Red:    lipgloss.Color("#F38BA8"),
		Cyan:   lipgloss.Color("#89DCEB"),
		Violet: lipgloss.Color("#CBA6F7"),
		Gray:   lipgloss.Color("#7F849C"),
		Muted:  lipgloss.Color("#BAC2DE"),
		Text:   lipgloss.Color("#CDD6F4"),
		Sep:    lipgloss.Color("#6C7086"),
		Border: lipgloss.Color("#585B70"),
		Accent: lipgloss.Color("#94E2D5"),
		SelBg:  lipgloss.Color("#45475A"),
		Panel:  lipgloss.Color("#1E1E2E"),
	},
	{
		Name:   "dracula",
		Green:  lipgloss.Color("#50FA7B"),
		Orange: lipgloss.Color("#FFB86C"),
		Yellow: lipgloss.Color("#F1FA8C"),
		Red:    lipgloss.Color("#FF5555"),
		Cyan:   lipgloss.Color("#8BE9FD"),
		Violet: lipgloss.Color("#BD93F9"),
		Gray:   lipgloss.Color("#6272A4"),
		Muted:  lipgloss.Color("#94A3C5"),
		Text:   lipgloss.Color("#F8F8F2"),
		Sep:    lipgloss.Color("#44475A"),
		Border: lipgloss.Color("#3B3E52"),
		Accent: lipgloss.Color("#BD93F9"),
		SelBg:  lipgloss.Color("#343746"),
		Panel:  lipgloss.Color("#282A36"),
	},
	{
		Name:   "tokyonight",
		Green:  lipgloss.Color("#9ECE6A"),
		Orange: lipgloss.Color("#FF9E64"),
		Yellow: lipgloss.Color("#E0AF68"),
		Red:    lipgloss.Color("#F7768E"),
		Cyan:   lipgloss.Color("#7DCFFF"),
		Violet: lipgloss.Color("#BB9AF7"),
		Gray:   lipgloss.Color("#7AA2F7"),
		Muted:  lipgloss.Color("#A9B1D6"),
		Text:   lipgloss.Color("#C0CAF5"),
		Sep:    lipgloss.Color("#3B4261"),
		Border: lipgloss.Color("#292E42"),
		Accent: lipgloss.Color("#7AA2F7"),
		SelBg:  lipgloss.Color("#2F3549"),
		Panel:  lipgloss.Color("#1A1B26"),
	},
}

var (
	colGreen  lipgloss.Color
	colOrange lipgloss.Color
	colYellow lipgloss.Color
	colRed    lipgloss.Color
	colCyan   lipgloss.Color
	colViolet lipgloss.Color
	colGray   lipgloss.Color
	colMuted  lipgloss.Color
	colText   lipgloss.Color
	colSep    lipgloss.Color
	colBorder lipgloss.Color
	colAccent lipgloss.Color
	colSelBg  lipgloss.Color
	colPanel  lipgloss.Color

	styleGreen  lipgloss.Style
	styleOrange lipgloss.Style
	styleYellow lipgloss.Style
	styleRed    lipgloss.Style
	styleCyan   lipgloss.Style
	styleViolet lipgloss.Style
	styleGray   lipgloss.Style
	styleMuted  lipgloss.Style
	styleText   lipgloss.Style
	styleSep    lipgloss.Style
	styleLabel  lipgloss.Style
	styleAccent lipgloss.Style
)

func init() {
	applyTheme(tuiThemes[0])
}

const (
	tuiStateDirName  = ".orbitor"
	tuiStateFileName = "tui_state.json"
	tuiHistoryLimit  = 500
)

type tuiStateFile struct {
	Theme string `json:"theme"`
}

func applyTheme(th tuiTheme) {
	colGreen = th.Green
	colOrange = th.Orange
	colYellow = th.Yellow
	colRed = th.Red
	colCyan = th.Cyan
	colViolet = th.Violet
	colGray = th.Gray
	colMuted = th.Muted
	colText = th.Text
	colSep = th.Sep
	colBorder = th.Border
	colAccent = th.Accent
	colSelBg = th.SelBg
	colPanel = th.Panel

	styleGreen = lipgloss.NewStyle().Foreground(colGreen).Bold(true)
	styleOrange = lipgloss.NewStyle().Foreground(colOrange).Bold(true)
	styleYellow = lipgloss.NewStyle().Foreground(colYellow).Bold(true)
	styleRed = lipgloss.NewStyle().Foreground(colRed).Bold(true)
	styleCyan = lipgloss.NewStyle().Foreground(colCyan).Bold(true)
	styleViolet = lipgloss.NewStyle().Foreground(colViolet).Bold(true)
	styleGray = lipgloss.NewStyle().Foreground(colGray)
	styleMuted = lipgloss.NewStyle().Foreground(colMuted)
	styleText = lipgloss.NewStyle().Foreground(colText)
	styleSep = lipgloss.NewStyle().Foreground(colSep)
	styleLabel = lipgloss.NewStyle().Foreground(colMuted)
	styleAccent = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
}

func themeIndexByName(name string) int {
	for i, th := range tuiThemes {
		if strings.EqualFold(th.Name, name) {
			return i
		}
	}
	return -1
}

func readThemePreference() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(home, tuiStateDirName, tuiStateFileName)
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var st tuiStateFile
	if err := json.Unmarshal(b, &st); err != nil {
		return "", err
	}
	return strings.TrimSpace(st.Theme), nil
}

func writeThemePreference(theme string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, tuiStateDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, tuiStateFileName)
	b, err := json.MarshalIndent(tuiStateFile{Theme: theme}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func currentThemeName(idx int) string {
	if idx < 0 || idx >= len(tuiThemes) {
		return "unknown"
	}
	return tuiThemes[idx].Name
}

// ── spinner ───────────────────────────────────────────────────────────────────

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

var (
	inlineCodeRe = regexp.MustCompile("`([^`]+)`")
	boldRe       = regexp.MustCompile(`\*\*([^\*]+)\*\*`)
)

// ── wizard options ────────────────────────────────────────────────────────────

var (
	wizardBackends      = []string{"copilot", "claude"}
	wizardCopilotModels = []string{
		"(default)",
		"gpt-5",
		"gpt-5-mini",
		"gpt-5.1",
		"gpt-5.1-codex-mini",
		"gpt-5.3-codex",
		"gpt-5.4",
		"gpt-5.4-mini",
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
	wizardModeOptions = []string{
		"agent  –  standard execution mode",
		"plan   –  plan only, no tool execution",
	}
)

func modelsForBackend(backend string) []string {
	if backend == "claude" {
		return wizardClaudeModels[1:]
	}
	return wizardCopilotModels[1:]
}

func canonicalModelForBackend(backend, input string) (string, bool) {
	candidate := strings.TrimSpace(input)
	if candidate == "" {
		return "", true
	}
	for _, model := range modelsForBackend(backend) {
		if strings.EqualFold(model, candidate) {
			return model, true
		}
	}
	return "", false
}

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

func (c *tuiAPIClient) createSession(workingDir, backend, model string, skipPermissions, planMode bool) (WSSessionInfo, error) {
	payload := map[string]any{
		"workingDir":      workingDir,
		"backend":         backend,
		"skipPermissions": skipPermissions,
		"planMode":        planMode,
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

func (c *tuiAPIClient) cloneSessionAndPrompt(sourceID, text string) (WSSessionInfo, error) {
	body, _ := json.Marshal(map[string]any{"text": text})
	resp, err := c.http.Post(c.baseURL+"/api/sessions/"+sourceID+"/clone-prompt", "application/json", bytes.NewReader(body))
	if err != nil {
		return WSSessionInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		msg, _ := io.ReadAll(resp.Body)
		return WSSessionInfo{}, fmt.Errorf("clone prompt failed: %s", strings.TrimSpace(string(msg)))
	}
	var out WSSessionInfo
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return WSSessionInfo{}, err
	}
	return out, nil
}

func (c *tuiAPIClient) selfUpdate(flutter bool) error {
	body, _ := json.Marshal(map[string]any{"flutter": flutter})
	resp, err := c.http.Post(c.baseURL+"/api/self-update", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("self-update failed: %s", strings.TrimSpace(string(msg)))
	}
	return nil
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

func (c *tuiAPIClient) updateSession(id string, skip, plan bool, model *string) (WSSessionInfo, error) {
	payload := map[string]any{"skipPermissions": skip, "planMode": plan}
	if model != nil {
		payload["model"] = *model
	}
	body, _ := json.Marshal(payload)
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
type clonePromptMsg struct {
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
type selfUpdateMsg struct{ err error }
type wsPayloadMsg struct{ payload []byte }
type infoMsg struct{ text string }
type errMsg struct{ err error }
type sttStartedMsg struct {
	proc      *exec.Cmd
	audioPath string
	err       error
}
type sttResultMsg struct {
	text string
	err  error
}
type whisperCLI struct {
	path   string
	flavor string
}
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

	// agent text coalescing — consecutive agent_text messages are merged into
	// a single growing log entry so the feed reads as flowing prose, not fragments.
	agentBlockIdx  int       // index into m.logs of the current block; -1 = none
	agentBlockTime time.Time // timestamp captured at block start
	agentBlockText string    // raw accumulated text (unstyled) for the block

	// replayingHistory suppresses notifications while replaying stored history
	// so that old events don't trigger a burst of notifications on open.
	replayingHistory bool

	// reconnect state
	wsReconnecting   bool
	wsReconnectSince time.Time

	// help overlay
	showHelp bool

	// zoo view
	showZoo bool
	zooBots []zooBot

	// new-session wizard
	wizardActive   bool
	wizardFocus    int // 0=dir, 1=backend, 2=model, 3=skip, 4=mode
	wizardBackend  int
	wizardModel    int
	wizardSkip     int
	wizardMode     int
	wizardDirInput textinput.Model

	// feed render options
	renderMarkdown bool
	compactBlocks  bool
	themeIdx       int

	// non-scrolling thinking pane
	thinkingLines []string
	isThinking    bool

	// push-to-talk speech-to-text (space hold)
	pttLastSpace time.Time
	pttActive    bool
	pttBusy      bool
	pttAudioPath string
	pttProc      *exec.Cmd

	// /model completion state
	modelCompLast       string
	modelCompCandidates []string
	modelCompIndex      int
	modelCompSessionID  string
}

func RunTUI(serverURL string, createNew bool, backend, model string, skip, plan bool) error {
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
		agentBlockIdx:     -1,
		renderMarkdown:    true,
		compactBlocks:     true,
		thinkingLines:     []string{"idle"},
	}
	if pref, err := readThemePreference(); err == nil && pref != "" {
		if idx := themeIndexByName(pref); idx >= 0 {
			m.themeIdx = idx
			applyTheme(tuiThemes[idx])
		}
	}
	m.logSystem("Connected to " + api.baseURL)
	m.logSystem("Tab/Shift+Tab cycle sessions  ·  ↑/↓ scroll chat  ·  Enter connect/send  ·  n new session  ·  Ctrl+D delete")
	m.logSystem("PgUp/PgDn scroll feed  ·  g/G top/bottom  ·  Ctrl+L clear  ·  Ctrl+R refresh  ·  Ctrl+C quit")
	m.logSystem("/help for all commands")

	if createNew {
		wd, err := os.Getwd()
		if err != nil {
			m.logSystem("failed to get cwd: " + err.Error())
		} else {
			created, err := api.createSession(wd, backend, model, skip, plan)
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

	p := tea.NewProgram(m, tea.WithAltScreen())
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
	m.wizardMode = 0
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
		m.wizardFocus = (m.wizardFocus + 1) % 5
		if m.wizardFocus == 0 {
			m.wizardDirInput.Focus()
		} else {
			m.wizardDirInput.Blur()
		}
		return m, nil
	case "shift+tab":
		m.wizardFocus = (m.wizardFocus + 4) % 5
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
		case 4:
			if m.wizardMode > 0 {
				m.wizardMode--
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
		case 4:
			if m.wizardMode < len(wizardModeOptions)-1 {
				m.wizardMode++
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
	plan := m.wizardMode == 1
	m.wizardActive = false
	return m, createSessionCmd(m.api, wd, backend, model, skip, plan)
}

func (m *tuiModel) renderHelp() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	head := lipgloss.NewStyle().Foreground(colCyan).Bold(true)
	key := lipgloss.NewStyle().Foreground(colText).Bold(true)
	desc := lipgloss.NewStyle().Foreground(colMuted)

	row := func(k, d string) string {
		return "  " + key.Render(fmt.Sprintf("%-18s", k)) + desc.Render(d)
	}

	lines := []string{
		titleStyle.Render("  Keyboard Shortcuts"),
		"",
		head.Render("  Navigation"),
		row("↑/↓", "scroll chat feed"),
		row("Tab/Shift+Tab", "cycle sessions"),
		row("Enter (empty input)", "connect to selected session"),
		row("n", "new session wizard"),
		row("z", "toggle agent zoo view"),
		row("Ctrl+D", "delete selected session"),
		"",
		head.Render("  Feed"),
		row("PgUp/PgDn", "scroll feed"),
		row("g / G", "scroll to top / bottom"),
		row("Ctrl+L", "clear feed"),
		row("Mouse drag", "select/highlight chat text"),
		row("Ctrl+M", "toggle markdown rendering"),
		row("Ctrl+B", "toggle compact/full blocks"),
		row("Ctrl+T", "cycle theme"),
		row("Ctrl+. / Ctrl+\\", "abort running session"),
		row("Ctrl+← / Ctrl+→", "move by word"),
		"",
		head.Render("  Session"),
		row("Enter (with text)", "send prompt to session"),
		row("Shift+Enter", "insert newline in prompt"),
		row("Alt+Enter", "send prompt to cloned session"),
		row("Hold Space", "push-to-talk dictation"),
		row("Ctrl+↑/↓", "cycle prompt history"),
		row("Ctrl+\\", "interrupt running session"),
		row("Ctrl+R / F5", "refresh sessions"),
		"",
		head.Render("  Commands"),
		row("/help", "show commands in feed"),
		row("/use <id>", "connect to session by ID"),
		row("/new <dir> [backend]", "create session"),
		row("/fork <prompt>", "clone current session and send"),
		row("/interrupt", "interrupt current session"),
		row("/allow <req> <opt>", "approve permission request"),
		row("/skip [true|false]", "toggle skip-permissions"),
		row("/plan [true|false]", "toggle plan mode"),
		row("/model <name> [id]", "set model for session"),
		row("/markdown [on|off]", "toggle markdown rendering"),
		row("/blocks [compact|full]", "toggle block density"),
		row("/theme [name]", "switch tui theme"),
		row("/delete [id]", "delete a session"),
		row("/quit", "exit"),
		"",
		desc.Render("  Press ? to close"),
	}

	panel := lipgloss.NewStyle().
		Background(colPanel).
		Border(lipgloss.NormalBorder()).
		BorderForeground(colBorder).
		Padding(0, 1).
		Width(56).
		Render(strings.Join(lines, "\n"))

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel)
}

func (m *tuiModel) renderWizard() string {
	const wizW = 60

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(colAccent)
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

	// Mode
	lines = append(lines, label(4, "  Mode"))
	for i, opt := range wizardModeOptions {
		if i == m.wizardMode {
			lines = append(lines, "    "+selOpt.Render("◉ "+opt))
		} else {
			lines = append(lines, "    "+dimOpt.Render("○ "+opt))
		}
	}
	lines = append(lines, "")
	lines = append(lines, styleMuted.Render("  Tab/Shift+Tab=next section  ↑/↓=select  Enter=create  Esc=cancel"))

	panel := lipgloss.NewStyle().
		Background(colPanel).
		Border(lipgloss.NormalBorder()).
		BorderForeground(colBorder).
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
		zooTickCmd(),
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
		if m.pttActive && time.Since(m.pttLastSpace) > 900*time.Millisecond {
			return m, tea.Batch(m.stopPTTCapture(), spinnerTickCmd())
		}
		return m, spinnerTickCmd()

	case zooTickMsg:
		if m.showZoo {
			canvasW := max(20, m.width)
			canvasH := max(8, m.height-6)
			m.zooBots = updateZooBots(m.zooBots, m.sessions, canvasW, canvasH)
		}
		return m, zooTickCmd()

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
		// Preserve selection by ID so the cursor doesn't jump when sessions
		// are added, removed, or reordered during a refresh.
		prevID := m.selectedSessionID()
		m.sessions = msg.sessions
		m.missionTitle = msg.missionTitle
		m.missionSummary = msg.missionSummary
		// Find the new index for the previously selected session.
		found := false
		for i, s := range m.sessions {
			if s.ID == prevID {
				m.selected = i
				found = true
				break
			}
		}
		if !found {
			if m.selected >= len(m.sessions) {
				m.selected = max(0, len(m.sessions)-1)
			}
		}
		m.updatePlaceholder()
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
		m.updatePlaceholder()
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

	case clonePromptMsg:
		if msg.err != nil {
			m.logSystem("Clone prompt failed: " + msg.err.Error())
			return m, nil
		}
		m.logSystem("Spawned clone session " + msg.session.ID)
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
		m.logSystem(fmt.Sprintf("Updated session %s (model=%s, skipPermissions=%v, planMode=%v)", msg.session.ID, defaultString(msg.session.Model, "(default)"), msg.session.SkipPermissions, msg.session.PlanMode))
		return m, refreshSessionsCmd(m.api)

	case sttResultMsg:
		m.pttBusy = false
		if m.pttAudioPath != "" {
			_ = os.Remove(m.pttAudioPath)
			m.pttAudioPath = ""
		}
		if msg.err != nil {
			m.logSystem("Dictation failed: " + msg.err.Error())
			return m, nil
		}
		if strings.TrimSpace(msg.text) != "" {
			if m.input.Value() != "" && !strings.HasSuffix(m.input.Value(), " ") {
				m.insertAtCursor(" ")
			}
			m.insertAtCursor(strings.TrimSpace(msg.text))
			m.logSystem("Dictation inserted")
		}
		return m, nil

	case sttStartedMsg:
		if msg.err != nil {
			m.pttBusy = false
			m.pttActive = false
			m.logSystem("Dictation start failed: " + msg.err.Error())
			return m, nil
		}
		m.pttProc = msg.proc
		m.pttAudioPath = msg.audioPath
		m.pttActive = true
		m.pttBusy = true
		m.logSystem("🎙 dictation listening (hold space), release to transcribe")
		return m, nil

	case selfUpdateMsg:
		if msg.err != nil {
			m.logSystem("Restart failed: " + msg.err.Error())
			return m, nil
		}
		m.logSystem("Server restart initiated (graceful self-update)")
		return m, nil

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
		if msg.String() != "tab" && msg.String() != "shift+tab" {
			m.resetModelCompletion()
		}
		// When wizard is open, route all key events to it.
		if m.wizardActive {
			return m.updateWizard(msg)
		}
		// Robust fallback for terminals that encode word-nav as Alt+arrow.
		if msg.Type == tea.KeyLeft && msg.Alt {
			m.moveCursorWordLeft()
			return m, nil
		}
		if msg.Type == tea.KeyRight && msg.Alt {
			m.moveCursorWordRight()
			return m, nil
		}

		// Toggle help overlay with ? only when input is empty.
		if msg.String() == "?" && m.input.Value() == "" {
			m.showHelp = !m.showHelp
			return m, nil
		}
		// Any key closes the help overlay.
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}

		// Zoo view intercepts all keys when active.
		if m.showZoo {
			return m.updateZoo(msg)
		}

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

		case " ":
			now := time.Now()
			m.pttLastSpace = now
			if !m.pttBusy && m.input.Value() == "" {
				if !m.pttActive {
					return m, m.startPTTCapture()
				}
				return m, nil
			}
			if m.pttBusy {
				return m, nil
			}

		case "up":
			m.viewport.ScrollUp(3)
			return m, nil

		case "down":
			m.viewport.ScrollDown(3)
			return m, nil

		case "ctrl+up":
			if len(m.inputHistory) == 0 {
				return m, nil
			}
			if m.historyPos == 0 {
				m.historyLive = m.input.Value()
			}
			if m.historyPos < len(m.inputHistory) {
				m.historyPos++
				m.input.SetValue(m.inputHistory[len(m.inputHistory)-m.historyPos])
				m.input.CursorEnd()
			}
			return m, nil

		case "ctrl+down":
			if m.historyPos == 0 {
				return m, nil
			}
			m.historyPos--
			if m.historyPos == 0 {
				m.input.SetValue(m.historyLive)
			} else {
				m.input.SetValue(m.inputHistory[len(m.inputHistory)-m.historyPos])
			}
			m.input.CursorEnd()
			return m, nil

		case "tab":
			if m.tryCompleteModel(false) {
				return m, nil
			}
			// Tab cycles sessions even when input has content.
			if len(m.sessions) > 0 {
				m.selected = (m.selected + 1) % len(m.sessions)
			}
			return m, nil

		case "shift+tab":
			if m.tryCompleteModel(true) {
				return m, nil
			}
			// Shift+Tab cycles sessions backwards even when input has content.
			if len(m.sessions) > 0 {
				m.selected = (m.selected + len(m.sessions) - 1) % len(m.sessions)
			}
			return m, nil

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

		case "ctrl+.", "ctrl+\\":
			if m.activeSessionID != "" {
				return m, sendWSCmd(m, map[string]any{"type": "interrupt"})
			}
			return m, nil

		case "ctrl+left", "alt+b":
			m.moveCursorWordLeft()
			return m, nil

		case "ctrl+right", "alt+f":
			m.moveCursorWordRight()
			return m, nil

		case "alt+left":
			m.moveCursorWordLeft()
			return m, nil

		case "alt+right":
			m.moveCursorWordRight()
			return m, nil

		case "ctrl+a":
			m.input.CursorStart()
			return m, nil

		case "ctrl+e":
			m.input.CursorEnd()
			return m, nil

		case "ctrl+w":
			m.deleteWordBackward()
			return m, nil

		case "alt+d":
			m.deleteWordForward()
			return m, nil

		case "ctrl+u":
			m.deleteToLineStart()
			return m, nil

		case "ctrl+k":
			m.deleteToLineEnd()
			return m, nil

		case "ctrl+r", "f5":
			return m, refreshSessionsCmd(m.api)

		case "ctrl+m":
			m.renderMarkdown = !m.renderMarkdown
			m.logSystem("Markdown rendering: " + boolLabel(m.renderMarkdown))
			m.rebuildViewport()
			return m, nil

		case "ctrl+b":
			m.compactBlocks = !m.compactBlocks
			if m.compactBlocks {
				m.logSystem("Block mode: compact")
			} else {
				m.logSystem("Block mode: full")
			}
			return m, nil

		case "ctrl+t":
			m.themeIdx = (m.themeIdx + 1) % len(tuiThemes)
			applyTheme(tuiThemes[m.themeIdx])
			if err := writeThemePreference(tuiThemes[m.themeIdx].Name); err != nil {
				m.logSystem("theme persistence warning: " + err.Error())
			}
			m.logSystem("Theme: " + tuiThemes[m.themeIdx].Name)
			m.rebuildViewport()
			return m, nil

		case "n":
			if m.input.Value() == "" {
				m.openWizard()
				return m, nil
			}

		case "z":
			if m.input.Value() == "" {
				m.showZoo = true
				return m, nil
			}

		case "shift+enter":
			m.insertAtCursor("\n")
			return m, nil

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

		case "alt+enter":
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, nil
			}
			m.input.SetValue("")
			m.historyPos = 0
			m.historyLive = ""
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
			return m, clonePromptCmd(m.api, m.activeSessionID, text)
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

	if m.wizardActive {
		return m.renderWizard()
	}

	if m.showHelp {
		return m.renderHelp()
	}

	if m.showZoo {
		return m.renderZoo()
	}

	// opencode-inspired: flat, low-contrast chrome with subtle panel tint.
	panelStyle := lipgloss.NewStyle().
		Background(colPanel).
		Border(lipgloss.NormalBorder()).
		BorderForeground(colBorder).
		Padding(0, 1)
	// Selected row: dark bg, white text.
	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Background(colSelBg).
		Bold(true)
	activeStyle := lipgloss.NewStyle().Foreground(colAccent)

	// Sessions panel is ~19% of terminal width (≈ 2/3 of previous 28%).
	leftW := max(22, m.width*19/100)
	rightW := m.width - leftW
	if rightW < 50 {
		rightW = 50
		leftW = m.width - rightW
	}

	// ── top bar (flat: one or two text lines + full-width separator) ──────────
	connLabel := styleMuted.Render(defaultString(m.activeSessionID, "–"))
	if m.activeSessionID != "" {
		connLabel = styleAccent.Render(m.activeSessionID)
	}
	topRight := connLabel + styleMuted.Render("  ·  "+fmt.Sprintf("%d sessions", len(m.sessions)))
	topLeft := styleAccent.Render("orbitor") + styleMuted.Render("  mission control")
	topGap := m.width - lipgloss.Width(topLeft) - lipgloss.Width(topRight) - 2
	if topGap < 1 {
		topGap = 1
	}
	topLine := " " + topLeft + strings.Repeat(" ", topGap) + topRight
	if strings.TrimSpace(m.missionTitle) != "" {
		topLine += "\n" + styleMuted.Render(" "+m.missionTitle)
	}
	topBar := topLine + "\n" + styleSep.Render(strings.Repeat("─", m.width))
	topBarH := lipgloss.Height(topBar)

	// ── optional banners ──────────────────────────────────────────────────────
	var banners []string

	if pendingSessions := m.pendingPermissionSessions(); len(pendingSessions) > 0 {
		lines := []string{styleYellow.Render(" ⚠ permission required")}
		for _, s := range pendingSessions {
			name := defaultString(s.Title, s.ID)
			lines = append(lines, styleMuted.Render("  "+name+"  ")+styleCyan.Render("/allow <requestId> <optionId>"))
		}
		banner := lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(colYellow).
			Padding(0, 1).
			Width(m.width - 2).
			Render(strings.Join(lines, "\n"))
		banners = append(banners, banner)
	}

	if m.deleteConfirmID != "" {
		banner := lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(colRed).
			Padding(0, 1).
			Width(m.width - 2).
			Render(styleRed.Render(" ⚠ delete "+m.deleteConfirmID+"?") +
				styleMuted.Render("  y=confirm  ·  any other key=cancel"))
		banners = append(banners, banner)
	}

	bannerH := 0
	for _, b := range banners {
		bannerH += lipgloss.Height(b)
	}

	// ── status bar ────────────────────────────────────────────────────────────
	statusLeft := fmt.Sprintf("  %s  ·  n=new  z=zoo  tab=cycle  ctrl+d=del  ?=help", m.api.baseURL)
	statusRight := time.Now().Format("15:04:05  ")
	statusPad := m.width - lipgloss.Width(statusLeft) - lipgloss.Width(statusRight)
	if statusPad < 0 {
		statusPad = 0
	}
	statusBar := styleSep.Render(strings.Repeat("─", m.width)) + "\n" +
		styleMuted.Render(statusLeft+strings.Repeat(" ", statusPad)+statusRight)
	statusBarH := 2

	// ── body layout ───────────────────────────────────────────────────────────
	bodyH := max(10, m.height-topBarH-bannerH-statusBarH-6)
	detailsH := 8
	if bodyH < 20 {
		detailsH = 6
	}
	thinkingH := 4
	if bodyH < 18 {
		thinkingH = 3
	}
	inputH := 3
	feedH := max(6, bodyH-detailsH-thinkingH-inputH)

	// ── left panel (sessions) ─────────────────────────────────────────────────
	sessionsAvailH := max(3, bodyH-1)
	listW := max(20, leftW-4) // inner content width for separator lines
	sessionsHeader := styleAccent.Render(" sessions")
	if m.missionSummary != "" {
		sessionsHeader += styleMuted.Render("  " + trimForLine(m.missionSummary, leftW-16))
	}
	left := panelStyle.Width(leftW - 2).Height(bodyH).Render(
		clampLines(sessionsHeader+"\n"+m.renderMissionControl(selectedStyle, activeStyle, sessionsAvailH, listW), bodyH),
	)

	// ── right panels ──────────────────────────────────────────────────────────
	detailContentW := max(20, rightW-4)
	detailBox := panelStyle.Width(rightW - 2).Height(detailsH).Render(
		clampLines(styleMuted.Render(" details")+"\n"+m.renderDetails(detailContentW), detailsH),
	)

	m.viewport.Height = max(4, feedH-3)

	feedHeader := styleMuted.Render(" feed")
	feedHeader += styleMuted.Render("  theme:" + currentThemeName(m.themeIdx))
	feedHeader += styleMuted.Render("  md:" + boolLabel(m.renderMarkdown))
	if m.compactBlocks {
		feedHeader += styleMuted.Render("  blocks:compact")
	} else {
		feedHeader += styleMuted.Render("  blocks:full")
	}
	if m.wsReconnecting {
		elapsed := time.Since(m.wsReconnectSince).Round(time.Second)
		feedHeader += styleYellow.Render("  ⟳ reconnecting… " + elapsed.String())
	} else if m.activeSessionID != "" {
		feedHeader += styleMuted.Render("  " + m.activeSessionID)
	} else {
		feedHeader += styleMuted.Render("  –")
	}
	if m.viewport.AtBottom() {
		feedHeader += styleAccent.Render("  ●")
	} else {
		feedHeader += styleMuted.Render("  ↑ PgUp/PgDn  G=bottom")
	}
	feedBox := panelStyle.Width(rightW - 2).Height(feedH).Render(feedHeader + "\n" + m.viewport.View())

	thinkingLabel := " idle"
	if m.isThinking {
		thinkingLabel = " thinking " + spinnerFrames[m.spinnerFrame]
	}
	thinkingHeader := styleMuted.Render(" thinking") + styleCyan.Render(thinkingLabel)
	thinkingBody := strings.Join(clampSliceTail(m.thinkingLines, max(1, thinkingH-1)), "\n")
	thinkingBox := panelStyle.Width(rightW - 2).Height(thinkingH).Render(
		thinkingHeader + "\n" + clampLines(thinkingBody, max(1, thinkingH-1)),
	)

	var promptPrefix string
	var hint string
	if m.activeSessionID != "" {
		promptPrefix = styleAccent.Render(" ❯ ")
		hint = "Enter=send(queue)  ·  Alt+Enter=fork send  ·  hold Space=dictate  ·  Ctrl+./Ctrl+\\=abort  ·  ↑/↓ scroll"
	} else {
		promptPrefix = styleMuted.Render(" ❯ ")
		hint = "Enter=connect  ·  hold Space=dictate  ·  Tab cycle sessions  ·  ↑/↓ scroll"
	}
	if m.isThinking {
		hint += "  ·  agent running"
	}
	inputBox := panelStyle.Width(rightW - 2).Height(inputH).Render(
		promptPrefix + m.input.View() + "\n" + styleMuted.Render("  "+hint),
	)

	right := lipgloss.JoinVertical(lipgloss.Left, detailBox, feedBox, thinkingBox, inputBox)
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

func (m *tuiModel) renderDetails(contentWidth int) string {
	if len(m.sessions) == 0 {
		return styleMuted.Render("No session selected")
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
	planStr := styleText.Render("false")
	if s.PlanMode {
		planStr = styleViolet.Render("true")
	}
	tool := defaultString(s.CurrentTool, "-")
	toolStr := styleText.Render(tool)
	if tool != "-" {
		toolStr = styleOrange.Render(tool)
	}

	lbl := func(s string) string { return styleLabel.Render(s) }
	// Each detail line has a 9-char label prefix; the value gets the rest.
	valW := max(10, contentWidth-9)

	lines := []string{
		lbl("id:      ") + styleText.Render(trimForLine(s.ID, valW)),
		lbl("state:   ") + stateStr + lbl("  status: ") + styleText.Render(trimForLine(s.Status, valW/2)),
		lbl("backend: ") + backendStr + lbl("  model: ") + styleText.Render(trimForLine(defaultString(s.Model, "(default)"), valW/2)),
		lbl("skip:    ") + skipStr + lbl("  plan: ") + planStr + lbl("  pending: ") + permStr + lbl("  running: ") + runStr + func() string {
			if s.QueueDepth > 0 {
				return lbl("  queued: ") + styleYellow.Render(strconv.Itoa(s.QueueDepth))
			}
			return ""
		}(),
		lbl("tool:    ") + toolStr,
		lbl("dir:     ") + styleText.Render(trimForLine(s.WorkingDir, valW)),
		lbl("msg:     ") + styleText.Render(trimForLine(defaultString(s.LastMessage, "-"), valW)),
	}
	if s.CurrentPrompt != "" {
		lines = append(lines, lbl("task:    ")+styleText.Render(trimForLine(s.CurrentPrompt, valW)))
	}
	if s.Title != "" {
		lines = append(lines, lbl("title:   ")+styleText.Render(trimForLine(s.Title, valW)))
	}
	if s.Summary != "" {
		lines = append(lines, lbl("summary: ")+styleText.Render(trimForLine(s.Summary, valW)))
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
		m.logSystem("  /fork <prompt>")
		m.logSystem("  /interrupt")
		m.logSystem("  /abort")
		m.logSystem("  /allow <requestId> <optionId>")
		m.logSystem("  /skip [true|false] [id]")
		m.logSystem("  /plan [true|false] [id]")
		m.logSystem("  /model <model|default> [id]")
		m.logSystem("  /markdown [on|off]")
		m.logSystem("  /blocks [compact|full]")
		m.logSystem("  /theme [name]")
		m.logSystem("  /themes")
		m.logSystem("  /restart")
		m.logSystem("  /delete [id]")
		m.logSystem("  /quit")
		m.logSystem("Hotkeys: n=new session  tab/shift+tab=cycle sessions  up/down=scroll chat  ctrl+up/down=prompt history  alt+enter=fork prompt  ctrl+d=delete  ctrl+l=clear  ctrl+m=markdown  ctrl+b=blocks  ctrl+t=theme  ctrl+./ctrl+\\=abort  ctrl/alt+left/right=word move  PgUp/PgDn=scroll  g/G=top/bottom")
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
		plan := false
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
		if len(fields) >= 6 {
			if v, err := strconv.ParseBool(fields[5]); err == nil {
				plan = v
			}
		}
		return createSessionCmd(m.api, wd, backend, model, skip, plan)
	case "/fork":
		if m.activeSessionID == "" {
			m.logSystem("Connect to a session first")
			return nil
		}
		prompt := strings.TrimSpace(strings.TrimPrefix(raw, "/fork"))
		if prompt == "" {
			m.logSystem("Usage: /fork <prompt>")
			return nil
		}
		return clonePromptCmd(m.api, m.activeSessionID, prompt)
	case "/interrupt":
		return sendWSCmd(m, map[string]any{"type": "interrupt"})
	case "/abort":
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
		return updateSessionCmd(m.api, target.ID, next, target.PlanMode, nil)
	case "/plan":
		if len(m.sessions) == 0 {
			m.logSystem("No sessions available")
			return nil
		}
		target := m.sessions[m.selected]
		nextPlan := !target.PlanMode
		if len(fields) >= 2 {
			if v, err := strconv.ParseBool(fields[1]); err == nil {
				nextPlan = v
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
		return updateSessionCmd(m.api, target.ID, target.SkipPermissions, nextPlan, nil)
	case "/model":
		if len(m.sessions) == 0 {
			m.logSystem("No sessions available")
			return nil
		}
		target := m.sessions[m.selected]
		if len(fields) >= 3 {
			if s, ok := m.findSessionByID(fields[2]); ok {
				target = s
			} else {
				m.logSystem("Session not found: " + fields[2])
				return nil
			}
		}
		if len(fields) < 2 {
			m.logSystem("Usage: /model <model|default> [sessionId]")
			m.logSystem("Available models (" + target.Backend + "): " + strings.Join(modelsForBackend(target.Backend), ", "))
			return nil
		}
		rawModel := strings.TrimSpace(fields[1])
		nextModel := ""
		if !strings.EqualFold(rawModel, "default") && rawModel != "(default)" {
			canon, ok := canonicalModelForBackend(target.Backend, rawModel)
			if !ok {
				m.logSystem("Unknown model for " + target.Backend + ": " + rawModel)
				m.logSystem("Available models (" + target.Backend + "): " + strings.Join(modelsForBackend(target.Backend), ", "))
				return nil
			}
			nextModel = canon
		}
		return updateSessionCmd(m.api, target.ID, target.SkipPermissions, target.PlanMode, &nextModel)
	case "/markdown":
		if len(fields) < 2 {
			m.logSystem("Usage: /markdown [on|off]")
			return nil
		}
		switch strings.ToLower(fields[1]) {
		case "on", "true", "1":
			m.renderMarkdown = true
		case "off", "false", "0":
			m.renderMarkdown = false
		default:
			m.logSystem("Usage: /markdown [on|off]")
			return nil
		}
		m.logSystem("Markdown rendering: " + boolLabel(m.renderMarkdown))
		m.rebuildViewport()
		return nil
	case "/blocks":
		if len(fields) < 2 {
			m.logSystem("Usage: /blocks [compact|full]")
			return nil
		}
		switch strings.ToLower(fields[1]) {
		case "compact":
			m.compactBlocks = true
		case "full":
			m.compactBlocks = false
		default:
			m.logSystem("Usage: /blocks [compact|full]")
			return nil
		}
		if m.compactBlocks {
			m.logSystem("Block mode: compact")
		} else {
			m.logSystem("Block mode: full")
		}
		return nil
	case "/theme":
		if len(fields) < 2 {
			var names []string
			for _, th := range tuiThemes {
				names = append(names, th.Name)
			}
			m.logSystem("Themes: " + strings.Join(names, ", "))
			return nil
		}
		want := strings.ToLower(fields[1])
		for i, th := range tuiThemes {
			if strings.EqualFold(th.Name, want) {
				m.themeIdx = i
				applyTheme(th)
				if err := writeThemePreference(th.Name); err != nil {
					m.logSystem("theme persistence warning: " + err.Error())
				}
				m.logSystem("Theme: " + th.Name)
				m.rebuildViewport()
				return nil
			}
		}
		m.logSystem("Unknown theme: " + fields[1])
		return nil
	case "/themes":
		var names []string
		for _, th := range tuiThemes {
			names = append(names, th.Name)
		}
		m.logSystem("Themes: " + strings.Join(names, ", "))
		return nil
	case "/restart":
		return selfUpdateCmd(m.api, false)
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
		if len(h.Messages) > tuiHistoryLimit {
			h.Messages = h.Messages[len(h.Messages)-tuiHistoryLimit:]
			m.logSystem(fmt.Sprintf("Loaded recent %d messages (history truncated)", tuiHistoryLimit))
		}
		// Clear the feed and rebuild from the authoritative DB history.
		// This handles both initial connect and reconnect cleanly.
		m.logs = nil
		m.agentBlockIdx = -1
		m.replayingHistory = true
		for _, it := range h.Messages {
			m.renderMessage(it)
		}
		m.replayingHistory = false
		m.rebuildViewport()
		return
	}
	var msg WSMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		return
	}
	m.renderMessage(msg)
}

// ── chat display helpers ───────────────────────────────────────────────────────

func (m *tuiModel) chatWidth() int {
	if m.viewport.Width > 4 {
		return m.viewport.Width
	}
	return 80
}

// turnHeader renders a visual separator line for a conversation turn:
//
//	── role  ·  14:23 ──────────────────────────────
func (m *tuiModel) turnHeader(role string, roleStyle lipgloss.Style, ts string) string {
	w := m.chatWidth()
	left := " " + role + " "
	leftW := max(12, w-len(ts)-3)
	if leftW < len(left) {
		leftW = len(left)
	}
	bar := left
	if lipgloss.Width(bar) < leftW {
		bar += strings.Repeat("─", leftW-lipgloss.Width(bar))
	}
	return roleStyle.Render(bar) + styleMuted.Render(" "+ts)
}

// ── renderMessage ─────────────────────────────────────────────────────────────

func (m *tuiModel) renderMessage(msg WSMessage) {
	tsStr := time.Now().Format("15:04")
	w := m.chatWidth()

	// Agent text is coalesced into a single growing log entry so the feed reads
	// as flowing prose rather than one line per network chunk.
	if msg.Type == "agent_text" {
		var d WSAgentText
		if json.Unmarshal(msg.Data, &d) != nil || d.Text == "" {
			return
		}
		if m.agentBlockIdx >= 0 && m.agentBlockIdx < len(m.logs) {
			m.agentBlockText += d.Text
			hdr := m.turnHeader("assistant", styleAccent, m.agentBlockTime.Format("15:04"))
			body := m.renderRichTextBlock(m.agentBlockText, w, false)
			m.logs[m.agentBlockIdx] = hdr + "\n" + body
		} else {
			m.agentBlockIdx = len(m.logs)
			m.agentBlockTime = time.Now()
			m.agentBlockText = d.Text
			hdr := m.turnHeader("assistant", styleAccent, tsStr)
			body := m.renderRichTextBlock(d.Text, w, false)
			m.logs = append(m.logs, hdr+"\n"+body)
			if len(m.logs) > 4000 {
				m.logs = m.logs[len(m.logs)-4000:]
				m.agentBlockIdx = -1 // index lost after trim; start fresh next chunk
			}
		}
		if !m.replayingHistory {
			m.rebuildViewport()
		}
		return
	}

	// Internal protocol messages with no visible representation — don't break
	// an active agent text block (e.g. acp_update can interleave with text chunks).
	if msg.Type == "acp_update" {
		m.ingestACPThinking(msg.Data)
		return
	}
	if msg.Type == "session_ended" {
		return
	}

	// Every other message type ends the current agent text block.
	m.agentBlockIdx = -1

	switch msg.Type {
	case "prompt_sent":
		var d struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(msg.Data, &d) == nil {
			hdr := m.turnHeader("you", styleCyan, tsStr)
			body := styleCyan.Render(wrapWords(d.Text, w, ""))
			m.log(hdr + "\n" + body)
			m.isThinking = true
			m.pushThinking("prompt sent")
		}

	case "tool_call":
		var d WSToolCall
		if json.Unmarshal(msg.Data, &d) == nil {
			m.log(styleMuted.Render("  --- tool call ---"))
			icon, iconCol := toolKindIcon(d.Kind)
			iconSty := lipgloss.NewStyle().Foreground(iconCol)

			// Status determines the leading sigil and colour.
			var sigil string
			var sigilSty lipgloss.Style
			switch d.Status {
			case "success", "done":
				sigil, sigilSty = "✓", styleGreen
			case "error":
				sigil, sigilSty = "✗", styleRed
			default: // pending / running
				sigil, sigilSty = icon, iconSty
			}

			titleStr := trimForLine(d.Title, w-14)
			kindLabel := styleMuted.Render("  " + d.Kind)
			titleLine := "  " + sigilSty.Render(sigil) + " " + styleText.Render(titleStr) + kindLabel + styleMuted.Render("  ["+d.Status+"]")

			if d.Content == "" {
				// Inline tool — single compact line.
				m.log(titleLine)
			} else {
				// Block tool — title then content behind a coloured │ rail.
				m.log(titleLine)
				m.log(m.renderRichTextBlock(d.Content, w, true))
			}
			m.isThinking = true
			m.pushThinking("tool: " + trimForLine(defaultString(d.Title, d.Kind), 70))
		}

	case "tool_result":
		var d WSToolResult
		if json.Unmarshal(msg.Data, &d) == nil && d.Content != "" {
			m.log(styleMuted.Render("  --- tool result ---"))
			m.log(m.renderRichTextBlock(d.Content, w, true))
			m.pushThinking("tool result")
		}

	case "permission_request":
		var d WSPermissionRequest
		if json.Unmarshal(msg.Data, &d) == nil {
			m.log("")
			m.log("  " + styleYellow.Render("⏸ permission required") + styleMuted.Render("  "+d.Title))
			if d.Command != "" {
				m.log(styleMuted.Render("    $ ") + styleText.Render(d.Command))
			}
			for _, o := range d.Options {
				m.log("    " + styleCyan.Render("["+o.OptionID+"]") + "  " + styleText.Render(o.Name) + styleMuted.Render("  "+o.Kind))
			}
			m.log(styleMuted.Render("    /allow " + d.RequestID + " <optionId>"))
			if !m.replayingHistory {
				go sendNotification("Permission needed", m.sessionDisplayName()+" is waiting for approval")
			}
			m.isThinking = true
		}

	case "permission_resolved":
		var d struct {
			RequestID string `json:"requestId"`
			OptionID  string `json:"optionId"`
		}
		if json.Unmarshal(msg.Data, &d) == nil {
			m.log("  " + styleGreen.Render("✓ approved") + styleMuted.Render("  "+d.OptionID))
			m.isThinking = false
		}

	case "run_complete":
		var d struct {
			StopReason string `json:"stopReason"`
			PRURL      string `json:"prUrl"`
		}
		if json.Unmarshal(msg.Data, &d) == nil {
			hdr := m.turnHeader("done", styleGreen, tsStr)
			entry := "\n" + hdr
			if d.StopReason != "" && d.StopReason != "end_turn" {
				entry += "\n" + styleMuted.Render("   "+d.StopReason)
			}
			m.log(entry)
			if d.PRURL != "" {
				m.log("   " + styleCyan.Render("PR: ") + d.PRURL)
			}
			notifBody := m.sessionDisplayName()
			if d.StopReason != "" {
				notifBody += " — " + d.StopReason
			}
			if !m.replayingHistory {
				go sendNotification("Agent finished", notifBody)
			}
			m.isThinking = false
			m.pushThinking("run complete")
		}

	case "status":
		var d struct {
			Status string `json:"status"`
		}
		if json.Unmarshal(msg.Data, &d) == nil && d.Status != "" {
			m.log(styleMuted.Render("  ◦ status: " + d.Status))
			m.pushThinking("status: " + d.Status)
			switch strings.ToLower(d.Status) {
			case "working", "running", "thinking", "starting", "respawning":
				m.isThinking = true
			case "ready", "idle", "killed":
				m.isThinking = false
			}
		}

	case "error":
		var d WSError
		if json.Unmarshal(msg.Data, &d) == nil {
			m.log("  " + styleRed.Render("error") + styleMuted.Render("  "+d.Message))
			m.isThinking = false
			m.pushThinking("error: " + trimForLine(d.Message, 70))
		}
	case "interrupted":
		m.isThinking = false
		m.log(styleMuted.Render("  · interrupted"))
	}
}

// ── message formatting helpers ─────────────────────────────────────────────────

// msgLeftBar prefixes each line with a coloured ┃ bar — used for "you" turns.
func msgLeftBar(text string, barColor lipgloss.Color) string {
	bar := lipgloss.NewStyle().Foreground(barColor).Render("┃") + " "
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = " " + bar + styleText.Render(l)
	}
	return strings.Join(lines, "\n")
}

// toolKindIcon maps a tool kind string to an opencode-style display icon and accent colour.
func toolKindIcon(kind string) (string, lipgloss.Color) {
	k := strings.ToLower(kind)
	switch {
	case strings.Contains(k, "bash") || strings.Contains(k, "exec") || k == "run_command" || k == "execute":
		return "$", colOrange
	case strings.Contains(k, "write") || strings.Contains(k, "create"):
		return "←", colGreen
	case strings.Contains(k, "edit") || strings.Contains(k, "patch") || strings.Contains(k, "modify"):
		return "≈", colCyan
	case strings.Contains(k, "read") || strings.Contains(k, "view") || strings.Contains(k, "cat"):
		return "→", colMuted
	case strings.Contains(k, "glob") || strings.Contains(k, "find") || strings.Contains(k, "list"):
		return "✱", colMuted
	case strings.Contains(k, "grep") || strings.Contains(k, "search"):
		return "✱", colCyan
	case strings.Contains(k, "web") || strings.Contains(k, "fetch") || strings.Contains(k, "browser") || strings.Contains(k, "url"):
		return "%", colCyan
	case strings.Contains(k, "task") || strings.Contains(k, "agent"):
		return "│", colCyan
	default:
		return "⚙", colMuted
	}
}

func boolLabel(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func (m *tuiModel) renderRichTextBlock(text string, width int, toolOutput bool) string {
	t := strings.TrimSpace(text)
	if t == "" {
		return ""
	}
	if isLikelyDiff(t) {
		return m.renderDiffBlock(t, width)
	}
	return m.renderMarkdownBlock(t, width, toolOutput)
}

func (m *tuiModel) renderMarkdownBlock(text string, width int, toolOutput bool) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	maxLines := len(lines)
	if toolOutput && m.compactBlocks {
		maxLines = min(maxLines, 14)
	}

	var out []string
	inCode := false
	codeLang := ""
	codeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("251")).Background(lipgloss.Color("236"))
	for i := 0; i < maxLines; i++ {
		line := lines[i]
		trim := strings.TrimSpace(line)

		if strings.HasPrefix(trim, "```") {
			inCode = !inCode
			codeLang = strings.TrimSpace(strings.TrimPrefix(trim, "```"))
			if inCode && codeLang != "" {
				out = append(out, styleMuted.Render("code: "+codeLang))
			}
			continue
		}

		if inCode {
			wrapped := wrapWords(line, max(12, width), "")
			for _, wl := range strings.Split(wrapped, "\n") {
				out = append(out, codeStyle.Render(" "+wl))
			}
			continue
		}

		if strings.HasPrefix(trim, "#") {
			level := 0
			for level < len(trim) && trim[level] == '#' {
				level++
			}
			head := strings.TrimSpace(trim[level:])
			if head == "" {
				continue
			}
			sty := styleAccent
			if level >= 3 {
				sty = styleCyan
			}
			out = append(out, sty.Render(strings.ToUpper(trimForLine(head, max(12, width)))))
			continue
		}

		if strings.HasPrefix(trim, "- ") || strings.HasPrefix(trim, "* ") {
			item := strings.TrimSpace(trim[2:])
			item = m.applyInlineMarkdown(item)
			out = append(out, styleMuted.Render("  • ")+styleText.Render(wrapWords(item, max(12, width-4), "")))
			continue
		}

		if strings.HasPrefix(trim, "1. ") || strings.HasPrefix(trim, "2. ") || strings.HasPrefix(trim, "3. ") || strings.HasPrefix(trim, "4. ") || strings.HasPrefix(trim, "5. ") {
			out = append(out, styleMuted.Render("  "+trim[:2])+styleText.Render(wrapWords(strings.TrimSpace(trim[2:]), max(12, width-4), "")))
			continue
		}

		rendered := line
		if m.renderMarkdown {
			rendered = m.applyInlineMarkdown(line)
		}
		if strings.TrimSpace(rendered) == "" {
			out = append(out, "")
			continue
		}
		out = append(out, styleText.Render("  "+wrapWords(rendered, max(12, width-2), "")))
	}

	if maxLines < len(lines) {
		out = append(out, styleMuted.Render(fmt.Sprintf("[+%d lines]", len(lines)-maxLines)))
	}
	return strings.Join(out, "\n")
}

func (m *tuiModel) applyInlineMarkdown(line string) string {
	if !m.renderMarkdown {
		return line
	}
	line = boldRe.ReplaceAllStringFunc(line, func(match string) string {
		sub := boldRe.FindStringSubmatch(match)
		if len(sub) != 2 {
			return match
		}
		return lipgloss.NewStyle().Bold(true).Render(sub[1])
	})
	line = inlineCodeRe.ReplaceAllStringFunc(line, func(match string) string {
		sub := inlineCodeRe.FindStringSubmatch(match)
		if len(sub) != 2 {
			return match
		}
		return lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("238")).Render(" " + sub[1] + " ")
	})
	return line
}

func (m *tuiModel) renderDiffBlock(diff string, width int) string {
	lines := strings.Split(strings.ReplaceAll(diff, "\r\n", "\n"), "\n")
	maxLines := len(lines)
	if m.compactBlocks {
		maxLines = min(maxLines, 24)
	}
	var out []string
	for i := 0; i < maxLines; i++ {
		l := lines[i]
		switch {
		case strings.HasPrefix(l, "diff --git"):
			out = append(out, styleAccent.Render("▌ file "+trimForLine(strings.TrimPrefix(l, "diff --git "), max(12, width-7))))
		case strings.HasPrefix(l, "index "):
			out = append(out, styleMuted.Render("  "+trimForLine(l, max(12, width-2))))
		case strings.HasPrefix(l, "@@"):
			out = append(out, styleCyan.Render("┃ "+trimForLine(l, max(12, width-2))))
		case strings.HasPrefix(l, "+++"), strings.HasPrefix(l, "---"):
			out = append(out, styleViolet.Render("│ "+trimForLine(l, max(12, width-2))))
		case strings.HasPrefix(l, "+"):
			out = append(out, styleGreen.Render("┃ + "+trimForLine(strings.TrimPrefix(l, "+"), max(12, width-4))))
		case strings.HasPrefix(l, "-"):
			out = append(out, styleRed.Render("┃ - "+trimForLine(strings.TrimPrefix(l, "-"), max(12, width-4))))
		default:
			out = append(out, styleText.Render("┃   "+trimForLine(l, max(12, width-4))))
		}
	}
	if maxLines < len(lines) {
		out = append(out, styleMuted.Render(fmt.Sprintf("[+%d diff lines]", len(lines)-maxLines)))
	}
	return strings.Join(out, "\n")
}

func isLikelyDiff(text string) bool {
	s := strings.TrimSpace(text)
	if strings.Contains(s, "\ndiff --git ") || strings.HasPrefix(s, "diff --git ") {
		return true
	}
	if strings.Contains(s, "\n@@ ") || strings.HasPrefix(s, "@@ ") {
		return true
	}
	if strings.Contains(s, "\n+++ ") && strings.Contains(s, "\n--- ") {
		return true
	}
	return false
}

// ── macOS notifications ───────────────────────────────────────────────────────

// sendNotification fires a macOS notification with sound via osascript.
// Runs in the background so it never blocks the TUI render loop.
func sendNotification(title, body string) {
	script := fmt.Sprintf(`display notification %q with title %q sound name "Default"`, body, title)
	_ = exec.Command("osascript", "-e", script).Start()
}

// sessionDisplayName returns the human-readable name for the currently
// active session — its Title if set, otherwise the base of its working dir.
func (m *tuiModel) sessionDisplayName() string {
	for _, s := range m.sessions {
		if s.ID == m.activeSessionID {
			if s.Title != "" {
				return s.Title
			}
			return filepath.Base(s.WorkingDir)
		}
	}
	return "session"
}

// ── layout helpers ────────────────────────────────────────────────────────────

func (m *tuiModel) resize() {
	leftW := max(22, m.width*19/100)
	rightW := m.width - leftW
	if rightW < 50 {
		rightW = 50
		leftW = m.width - rightW
	}
	vw := max(24, rightW-8)
	m.viewport.Width = vw
	// Viewport height is set in View() where the actual banner height is known.
	m.rebuildViewport()
}

func (m *tuiModel) rebuildViewport() {
	atBottom := m.viewport.AtBottom()
	m.viewport.SetContent(m.renderViewportContent())
	if atBottom {
		m.viewport.GotoBottom()
	}
}

func (m *tuiModel) renderViewportContent() string {
	w := m.chatWidth()
	bg := lipgloss.NewStyle().Background(colPanel).Foreground(colText)
	var out []string
	for _, entry := range m.logs {
		lines := strings.Split(entry, "\n")
		for _, line := range lines {
			line = stripANSI(line)
			out = append(out, bg.Render(padToWidth(line, w)))
		}
	}
	return strings.Join(out, "\n")
}

func padToWidth(s string, w int) string {
	if w <= 0 {
		return s
	}
	lw := lipgloss.Width(s)
	if lw >= w {
		return s
	}
	return s + strings.Repeat(" ", w-lw)
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

func clampSliceTail(in []string, n int) []string {
	if n <= 0 || len(in) == 0 {
		return []string{}
	}
	if len(in) <= n {
		return in
	}
	return in[len(in)-n:]
}

func (m *tuiModel) log(s string) {
	m.logs = append(m.logs, s)
	if len(m.logs) > 4000 {
		m.logs = m.logs[len(m.logs)-4000:]
	}
	if !m.replayingHistory {
		m.rebuildViewport()
	}
}

func (m *tuiModel) insertAtCursor(s string) {
	if s == "" {
		return
	}
	v := []rune(m.input.Value())
	pos := m.input.Position()
	if pos < 0 {
		pos = 0
	}
	if pos > len(v) {
		pos = len(v)
	}
	ins := []rune(s)
	newVal := string(v[:pos]) + string(ins) + string(v[pos:])
	m.input.SetValue(newVal)
	m.input.SetCursor(pos + len(ins))
}

func (m *tuiModel) pushThinking(s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	m.thinkingLines = append(m.thinkingLines, styleMuted.Render("• ")+styleText.Render(trimForLine(s, 90)))
	if len(m.thinkingLines) > 80 {
		m.thinkingLines = m.thinkingLines[len(m.thinkingLines)-80:]
	}
}

func (m *tuiModel) ingestACPThinking(raw json.RawMessage) {
	var env struct {
		UpdateType string          `json:"updateType"`
		Data       json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return
	}
	updateType := strings.ToLower(strings.TrimSpace(env.UpdateType))
	force := strings.Contains(updateType, "think") ||
		strings.Contains(updateType, "reason") ||
		strings.Contains(updateType, "analysis") ||
		strings.Contains(updateType, "plan")

	var data any
	if len(env.Data) > 0 && json.Unmarshal(env.Data, &data) == nil {
		lines := extractThinkingLines(data, force)
		for _, line := range lines {
			m.pushThinking("thought: " + line)
		}
	}
}

func extractThinkingLines(v any, force bool) []string {
	seen := map[string]struct{}{}
	var out []string

	reasonKey := func(k string) bool {
		k = strings.ToLower(strings.TrimSpace(k))
		return strings.Contains(k, "thought") ||
			strings.Contains(k, "think") ||
			strings.Contains(k, "reason") ||
			strings.Contains(k, "analysis") ||
			strings.Contains(k, "plan") ||
			strings.Contains(k, "rationale")
	}
	reasonType := func(t string) bool {
		t = strings.ToLower(strings.TrimSpace(t))
		return strings.Contains(t, "thought") ||
			strings.Contains(t, "think") ||
			strings.Contains(t, "reason") ||
			strings.Contains(t, "analysis") ||
			strings.Contains(t, "plan")
	}
	appendLine := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		s = strings.Join(strings.Fields(s), " ")
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}

	var walk func(node any, ctx bool)
	walk = func(node any, ctx bool) {
		switch n := node.(type) {
		case string:
			if ctx || force {
				appendLine(n)
			}
		case []any:
			for _, item := range n {
				walk(item, ctx)
			}
		case map[string]any:
			localCtx := ctx
			if typ, ok := n["type"].(string); ok && reasonType(typ) {
				localCtx = true
			}
			for k, val := range n {
				keyCtx := localCtx || reasonKey(k)
				switch vv := val.(type) {
				case string:
					lk := strings.ToLower(strings.TrimSpace(k))
					if keyCtx || (lk == "text" && localCtx) || force {
						appendLine(vv)
					}
				default:
					walk(vv, keyCtx)
				}
			}
		}
	}

	walk(v, false)
	if len(out) > 6 {
		out = out[len(out)-6:]
	}
	return out
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

func createSessionCmd(api *tuiAPIClient, wd, backend, model string, skip, plan bool) tea.Cmd {
	return func() tea.Msg {
		created, err := api.createSession(wd, backend, model, skip, plan)
		return createSessionMsg{session: created, err: err}
	}
}

func clonePromptCmd(api *tuiAPIClient, sourceID, text string) tea.Cmd {
	return func() tea.Msg {
		created, err := api.cloneSessionAndPrompt(sourceID, text)
		return clonePromptMsg{session: created, err: err}
	}
}

func deleteSessionCmd(api *tuiAPIClient, id string) tea.Cmd {
	return func() tea.Msg {
		err := api.deleteSession(id)
		return deleteSessionMsg{id: id, err: err}
	}
}

func updateSessionCmd(api *tuiAPIClient, id string, skip, plan bool, model *string) tea.Cmd {
	return func() tea.Msg {
		updated, err := api.updateSession(id, skip, plan, model)
		return updateSessionMsg{session: updated, err: err}
	}
}

func selfUpdateCmd(api *tuiAPIClient, flutter bool) tea.Cmd {
	return func() tea.Msg {
		err := api.selfUpdate(flutter)
		return selfUpdateMsg{err: err}
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

func (m *tuiModel) renderMissionControl(selectedStyle, activeStyle lipgloss.Style, availH int, listW int) string {
	if len(m.sessions) == 0 {
		return styleMuted.Render("  No sessions yet\n\n") +
			styleText.Render("  Press ") + styleCyan.Render("n") + styleText.Render(" to start a new session\n") +
			styleText.Render("  Press ") + styleCyan.Render("?") + styleText.Render(" for help")
	}

	linesPerSession := 4 // 3 content lines + 1 separator
	maxSessions := max(1, availH/linesPerSession)
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
		titleText := defaultString(s.Title, defaultString(s.CurrentPrompt, defaultString(s.LastMessage, "—")))
		fullPath := homeTildePath(s.WorkingDir)

		// Build inline badges for plan/skip modes.
		var badges string
		if s.PlanMode {
			badges += " " + styleViolet.Render("[P]")
		}
		if s.SkipPermissions {
			badges += " " + styleOrange.Render("[S]")
		}

		stateLabel := state
		switch state {
		case "working":
			stateLabel = "working"
			if elapsed := m.sessionStateStart[s.ID]; !elapsed.IsZero() {
				stateLabel += " " + formatElapsed(time.Since(elapsed))
			}
		case "waiting-input":
			stateLabel = "waiting"
		}
		topLine := trimForLine(prefix+s.ID+"  "+stateLabel+"  "+backendModel, max(10, listW))
		botLine := trimForLine("  "+fullPath, max(10, listW))
		sep := styleSep.Render(strings.Repeat("─", listW))
		if isSelected {
			titleLine := trimForLine("  "+titleText, max(10, listW))
			out = append(out, selectedStyle.Render(topLine), selectedStyle.Render(titleLine), selectedStyle.Render(botLine), sep)
		} else {
			var stateTag string
			switch state {
			case "working":
				stateTag = styleOrange.Render(spinnerFrames[m.spinnerFrame] + " working")
				if elapsed := m.sessionStateStart[s.ID]; !elapsed.IsZero() {
					stateTag += styleMuted.Render(" " + formatElapsed(time.Since(elapsed)))
				}
				if s.QueueDepth > 0 {
					stateTag += styleYellow.Render(fmt.Sprintf(" +%d queued", s.QueueDepth))
				}
			case "starting":
				stateTag = styleCyan.Render(spinnerFrames[m.spinnerFrame] + " starting")
			case "waiting-input":
				stateTag = styleYellow.Render("waiting")
			default:
				stateTag = stateStyle(state).Render(state)
			}
			topLine := prefix + s.ID + "  " + stateTag + "  " + styleCyan.Render(trimForLine(backendModel, max(8, listW/3)))
			titleLine := styleText.Render(trimForLine("  "+titleText, max(10, listW)))
			botLine := styleMuted.Render(trimForLine("  "+fullPath, max(10, listW)))
			if isActive {
				topLine = activeStyle.Render(prefix+s.ID) + "  " + stateTag + "  " + styleCyan.Render(trimForLine(backendModel, max(8, listW/3)))
			}
			out = append(out, trimForLine(topLine, max(10, listW))+badges, titleLine, botLine, sep)
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

// clampLines truncates s to at most n lines (splitting on \n).
// Each line produced by lipgloss Render() is self-contained (ANSI reset at end),
// so splitting and rejoining is safe.
func clampLines(s string, n int) string {
	if n <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n")
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

func (m *tuiModel) moveCursorWordLeft() {
	runes := []rune(m.input.Value())
	pos := m.input.Position()
	if pos <= 0 {
		m.input.SetCursor(0)
		return
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	for pos > 0 && isWordBoundaryRune(runes[pos-1]) {
		pos--
	}
	for pos > 0 && !isWordBoundaryRune(runes[pos-1]) {
		pos--
	}
	m.input.SetCursor(pos)
}

func (m *tuiModel) moveCursorWordRight() {
	runes := []rune(m.input.Value())
	pos := m.input.Position()
	n := len(runes)
	if pos >= n {
		m.input.SetCursor(n)
		return
	}
	for pos < n && isWordBoundaryRune(runes[pos]) {
		pos++
	}
	for pos < n && !isWordBoundaryRune(runes[pos]) {
		pos++
	}
	m.input.SetCursor(pos)
}

func (m *tuiModel) deleteWordBackward() {
	v := []rune(m.input.Value())
	pos := m.input.Position()
	if pos <= 0 || len(v) == 0 {
		return
	}
	start := pos
	for start > 0 && isWordBoundaryRune(v[start-1]) {
		start--
	}
	for start > 0 && !isWordBoundaryRune(v[start-1]) {
		start--
	}
	newVal := string(append(v[:start], v[pos:]...))
	m.input.SetValue(newVal)
	m.input.SetCursor(start)
}

func (m *tuiModel) deleteWordForward() {
	v := []rune(m.input.Value())
	pos := m.input.Position()
	if pos >= len(v) {
		return
	}
	end := pos
	for end < len(v) && isWordBoundaryRune(v[end]) {
		end++
	}
	for end < len(v) && !isWordBoundaryRune(v[end]) {
		end++
	}
	newVal := string(append(v[:pos], v[end:]...))
	m.input.SetValue(newVal)
	m.input.SetCursor(pos)
}

func (m *tuiModel) deleteToLineStart() {
	v := []rune(m.input.Value())
	pos := m.input.Position()
	if pos <= 0 {
		return
	}
	m.input.SetValue(string(v[pos:]))
	m.input.SetCursor(0)
}

func (m *tuiModel) deleteToLineEnd() {
	v := []rune(m.input.Value())
	pos := m.input.Position()
	if pos >= len(v) {
		return
	}
	m.input.SetValue(string(v[:pos]))
	m.input.SetCursor(pos)
}

func isWordBoundaryRune(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '/' || r == '\\' || r == ':' || r == '.' || r == '-' || r == '_' || r == ',' || r == ';' || r == '(' || r == ')' || r == '[' || r == ']'
}

func homeTildePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	home = strings.TrimSuffix(home, "/")
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+"/") {
		return "~/" + strings.TrimPrefix(path, home+"/")
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

// wrapWords word-wraps text to maxWidth, prefixing every output line with indent.
// Blank lines in the source are preserved as empty lines in the output.
func wrapWords(text string, maxWidth int, indent string) string {
	lineW := maxWidth - len(indent)
	if lineW < 10 {
		lineW = 10
	}
	var result []string
	for _, para := range strings.Split(text, "\n") {
		if strings.TrimSpace(para) == "" {
			result = append(result, "")
			continue
		}
		words := strings.Fields(para)
		var line strings.Builder
		lineLen := 0
		for _, word := range words {
			wl := len(word)
			if lineLen == 0 {
				line.WriteString(word)
				lineLen = wl
			} else if lineLen+1+wl <= lineW {
				line.WriteByte(' ')
				line.WriteString(word)
				lineLen += 1 + wl
			} else {
				result = append(result, indent+line.String())
				line.Reset()
				line.WriteString(word)
				lineLen = wl
			}
		}
		if line.Len() > 0 {
			result = append(result, indent+line.String())
		}
	}
	return strings.Join(result, "\n")
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

func (m *tuiModel) startPTTCapture() tea.Cmd {
	return func() tea.Msg {
		if m.pttBusy || m.pttActive {
			return sttStartedMsg{err: fmt.Errorf("dictation already active")}
		}
		if _, err := exec.LookPath("ffmpeg"); err != nil {
			return sttStartedMsg{err: fmt.Errorf("ffmpeg not found")}
		}
		audioPath := filepath.Join(os.TempDir(), fmt.Sprintf("orbitor-stt-%d.wav", time.Now().UnixNano()))
		cmd := exec.Command("ffmpeg",
			"-hide_banner", "-loglevel", "error", "-nostdin",
			"-f", "avfoundation", "-i", ":0",
			"-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le",
			audioPath,
		)
		if err := cmd.Start(); err != nil {
			return sttStartedMsg{err: err}
		}
		return sttStartedMsg{proc: cmd, audioPath: audioPath}
	}
}

func (m *tuiModel) stopPTTCapture() tea.Cmd {
	return func() tea.Msg {
		if !m.pttActive {
			return nil
		}
		m.pttActive = false
		proc := m.pttProc
		audioPath := m.pttAudioPath
		m.pttProc = nil
		if proc != nil && proc.Process != nil {
			_ = proc.Process.Signal(os.Interrupt)
			done := make(chan error, 1)
			go func() { done <- proc.Wait() }()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				_ = proc.Process.Kill()
			}
		}
		if audioPath == "" {
			return sttResultMsg{err: fmt.Errorf("no audio captured")}
		}
		text, err := transcribeAudioSwift(audioPath)
		return sttResultMsg{text: text, err: err}
	}
}

func transcribeAudioSwift(path string) (string, error) {
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("capture missing: %w", err)
	}
	cli, err := findWhisperCLI()
	if err != nil {
		return "", err
	}
	outBase := strings.TrimSuffix(path, filepath.Ext(path))
	modelRef, err := whisperModelRef(cli.flavor)
	if err != nil {
		return "", err
	}
	args := whisperTranscriptionArgs(cli.flavor, modelRef, path, outBase)
	cmd := exec.Command(cli.path, args...)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s", msg)
	}
	txtPath := outBase + ".txt"
	defer os.Remove(txtPath)
	b, err := os.ReadFile(txtPath)
	if err != nil {
		return "", fmt.Errorf("dictation output missing: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}

func whisperTranscriptionArgs(flavor, modelPath, audioPath, outBase string) []string {
	switch flavor {
	case "python":
		return []string{
			audioPath,
			"--model", modelPath,
			"--language", "en",
			"--output_format", "txt",
			"--output_dir", filepath.Dir(outBase),
			"--verbose", "False",
		}
	default:
		return []string{
			"-m", modelPath,
			"-f", audioPath,
			"-l", "en",
			"-otxt",
			"-of", outBase,
			"-np",
		}
	}
}

func whisperModelRef(flavor string) (string, error) {
	if flavor == "python" {
		return "tiny.en", nil
	}
	return ensureWhisperTinyModel()
}

func findWhisperCLI() (whisperCLI, error) {
	candidates := []whisperCLI{
		{path: "whisper-cli", flavor: "cpp"},
		{path: "whisper-cpp", flavor: "cpp"},
	}
	for _, candidate := range candidates {
		if p, err := exec.LookPath(candidate.path); err == nil {
			candidate.path = p
			return candidate, nil
		}
	}
	if p, err := exec.LookPath("whisper"); err == nil {
		flavor, detectErr := detectWhisperFlavor(p)
		if detectErr != nil {
			return whisperCLI{}, detectErr
		}
		return whisperCLI{path: p, flavor: flavor}, nil
	}
	return whisperCLI{}, fmt.Errorf("local STT not installed: run `brew install whisper-cpp` or install the Python `whisper` CLI")
}

func detectWhisperFlavor(path string) (string, error) {
	cmd := exec.Command(path, "--help")
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	helpText := strings.ToLower(out.String() + "\n" + stderr.String())
	switch {
	case strings.Contains(helpText, "--output_format") || strings.Contains(helpText, "--model_dir"):
		return "python", nil
	case strings.Contains(helpText, "-otxt") || strings.Contains(helpText, "whisper.cpp"):
		return "cpp", nil
	case err == nil:
		return "", fmt.Errorf("unsupported whisper CLI at %s", path)
	default:
		return "", fmt.Errorf("detect whisper CLI failed: %w", err)
	}
}

func ensureWhisperTinyModel() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".orbitor", "models")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	modelPath := filepath.Join(dir, "ggml-tiny.en.bin")
	if _, err := os.Stat(modelPath); err == nil {
		return modelPath, nil
	}
	url := "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-tiny.en.bin"
	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("download tiny model failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download tiny model failed: %s", resp.Status)
	}
	tmp := modelPath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, modelPath); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return modelPath, nil
}

func (m *tuiModel) resetModelCompletion() {
	m.modelCompLast = ""
	m.modelCompCandidates = nil
	m.modelCompIndex = 0
	m.modelCompSessionID = ""
}

func (m *tuiModel) resolveModelCompletionTarget(fields []string) (WSSessionInfo, bool) {
	if len(m.sessions) == 0 {
		return WSSessionInfo{}, false
	}
	target := m.sessions[m.selected]
	if len(fields) >= 3 {
		s, ok := m.findSessionByID(fields[2])
		if !ok {
			return WSSessionInfo{}, false
		}
		target = s
	}
	return target, true
}

func (m *tuiModel) tryCompleteModel(reverse bool) bool {
	raw := strings.TrimSpace(m.input.Value())
	if !strings.HasPrefix(raw, "/model") {
		return false
	}
	fields := strings.Fields(raw)
	target, ok := m.resolveModelCompletionTarget(fields)
	if !ok {
		return true
	}
	typedModel := ""
	if len(fields) >= 2 {
		typedModel = fields[1]
	}

	snapshot := target.ID + "::" + typedModel
	if m.modelCompLast != snapshot {
		var candidates []string
		for _, mdl := range modelsForBackend(target.Backend) {
			if typedModel == "" || strings.HasPrefix(strings.ToLower(mdl), strings.ToLower(typedModel)) {
				candidates = append(candidates, mdl)
			}
		}
		if strings.HasPrefix(strings.ToLower("default"), strings.ToLower(typedModel)) {
			candidates = append(candidates, "default")
		}
		if len(candidates) == 0 {
			return true
		}
		m.modelCompLast = snapshot
		m.modelCompCandidates = candidates
		m.modelCompIndex = 0
		m.modelCompSessionID = target.ID
	} else if len(m.modelCompCandidates) > 0 {
		if reverse {
			m.modelCompIndex = (m.modelCompIndex + len(m.modelCompCandidates) - 1) % len(m.modelCompCandidates)
		} else {
			m.modelCompIndex = (m.modelCompIndex + 1) % len(m.modelCompCandidates)
		}
	}
	if len(m.modelCompCandidates) == 0 {
		return true
	}
	completed := m.modelCompCandidates[m.modelCompIndex]
	parts := []string{"/model", completed}
	if len(fields) >= 3 {
		parts = append(parts, fields[2])
	}
	m.input.SetValue(strings.Join(parts, " "))
	m.input.CursorEnd()
	return true
}

func (m *tuiModel) updatePlaceholder() {
	if len(m.sessions) == 0 {
		m.input.Placeholder = "Press n to create a new session..."
		return
	}
	if m.activeSessionID == "" {
		m.input.Placeholder = "Press Enter to connect, or type a command..."
		return
	}
	if s, ok := m.findSessionByID(m.activeSessionID); ok {
		state := sessionStateLabel(s)
		switch state {
		case "working":
			m.input.Placeholder = "Session is busy — Ctrl+\\ to interrupt..."
		case "waiting-input":
			m.input.Placeholder = "Session needs permission — /allow <requestId> <optionId>"
		default:
			m.input.Placeholder = "Type a prompt and press Enter..."
		}
		return
	}
	m.input.Placeholder = "Type a prompt and press Enter..."
}
