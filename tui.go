package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
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
	tuiStateDirName          = ".orbitor"
	tuiStateFileName         = "tui_state.json"
	tuiHistoryLimit          = 500
	pttHoldThreshold         = 4
	pttReleaseDebounce       = 175 * time.Millisecond
	spinnerTickInterval      = 100 * time.Millisecond
	pttShutdownLogFloor      = 75 * time.Millisecond
	pttTranscriptionLogFloor = 250 * time.Millisecond
	// shiftEnterPrivate is a Unicode private-use character (U+E001) used as a
	// sentinel to signal "Shift+Enter → insert newline". Bubbletea's standard
	// terminal parser cannot distinguish Shift+Enter from Enter in most
	// terminals, so shiftEnterFilter converts known Kitty/xterm escape
	// sequences into a KeyRunes message carrying this rune before the model
	// sees it.
	shiftEnterPrivate = '\ue001'
	// altEnterPrivate is a Unicode private-use character (U+E002) used as a
	// sentinel to signal "Alt/Option+Enter → fork send". With modifyOtherKeys=2
	// active, Alt+Enter sends \x1b[27;3;13~ or \x1b[13;3u rather than the
	// traditional \x1b\r that bubbletea would parse natively as "alt+enter".
	altEnterPrivate = '\ue002'
)

// keyFilter is a bubbletea WithFilter function that converts unrecognised CSI
// sequences for Shift+Enter and Alt+Enter into synthetic KeyRunes messages
// carrying private-use sentinels the model can act on.
//
// Sequences handled:
//
//	\x1b[13u     – Kitty keyboard protocol: bare Enter (no modifier)
//	\x1b[13;1u   – Kitty keyboard protocol: bare Enter (explicit no-modifier)
//	\x1b[13;2u   – Kitty keyboard protocol: Shift+Enter
//	\x1b[27;2;13~ – xterm modifyOtherKeys=2: Shift+Enter
//	\x1b[13;3u   – Kitty keyboard protocol: Alt+Enter
//	\x1b[27;3;13~ – xterm modifyOtherKeys=2: Alt+Enter
//
// The bare-Enter cases are needed because enabling the Kitty keyboard protocol
// (disambiguate flag) remaps plain Enter from \r to \x1b[13u.
func shiftEnterFilter(_ tea.Model, msg tea.Msg) tea.Msg {
	rv := reflect.ValueOf(msg)
	// bubbletea's unknownCSISequenceMsg is an unexported []byte type.
	// Detect it by checking the package path + slice-of-bytes shape.
	if rv.Kind() != reflect.Slice {
		return msg
	}
	rt := rv.Type()
	if rt.Elem().Kind() != reflect.Uint8 {
		return msg
	}
	if rt.PkgPath() != "github.com/charmbracelet/bubbletea" {
		return msg
	}
	data := rv.Bytes()
	// Kitty bare Enter (sent when the Kitty disambiguate flag is active).
	if bytes.Equal(data, []byte("\x1b[13u")) || bytes.Equal(data, []byte("\x1b[13;1u")) {
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	if bytes.Equal(data, []byte("\x1b[13;2u")) || bytes.Equal(data, []byte("\x1b[27;2;13~")) {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{shiftEnterPrivate}}
	}
	if bytes.Equal(data, []byte("\x1b[13;3u")) || bytes.Equal(data, []byte("\x1b[27;3;13~")) {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{altEnterPrivate}}
	}
	return msg
}

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

// ── undo/redo types ──────────────────────────────────────────────────────────

// fileChangeSnapshot stores a git diff patch that can be reversed (undo) or
// reapplied (redo). Each snapshot represents the file-level changes made
// during a single agent run.
type fileChangeSnapshot struct {
	patch     string    // unified diff output from git diff HEAD
	files     []string  // affected file paths
	timestamp time.Time // when the snapshot was captured
	message   string    // human-readable description
}

// undoStack tracks file change snapshots per session for undo/redo support.
type undoStack struct {
	undoable []fileChangeSnapshot
	redoable []fileChangeSnapshot
}

// ── pending notification ─────────────────────────────────────────────────────

// pendingNotification tracks a run_complete event waiting for the AI summary
// to become available before firing the desktop notification.
type pendingNotification struct {
	sessionID  string
	stopReason string
	prURL      string
	createdAt  time.Time
}

// ── sub-agent feed blocks ────────────────────────────────────────────────────

// subAgentBlock tracks the log indices belonging to a single sub-agent
// invocation so they can be collapsed/expanded in the feed.
type subAgentBlock struct {
	toolCallID   string
	title        string    // human-readable title from the tool_call
	status       string    // "running", "completed", "failed"
	summaryIdx   int       // index in m.logs where the collapsible summary lives
	childIndices []int     // indices in m.logs of all child messages within this sub-agent
	startedAt    time.Time
	completedAt  time.Time
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
type wsDisconnectedMsg struct {
	conn *websocket.Conn
	err  error
}
type infoMsg struct{ text string }
type errMsg struct{ err error }
type sttStartedMsg struct {
	proc          *exec.Cmd
	stdin         io.WriteCloser
	audioPath     string
	streaming     bool
	disableNative bool
	disableNote   string
	err           error
	localSession  *localSTTSession // non-nil when using local whisper STT
}
type sttPartialMsg struct {
	text     string
	external bool
}
type sttResultMsg struct {
	text               string
	err                error
	captureStopDelay   time.Duration
	transcribeDelay    time.Duration
	releaseToTextDelay time.Duration
	disableNative      bool
	disableNote        string
	external           bool
}
type clipboardPasteMsg struct {
	insert string
	note   string
	err    error
}
type whisperCLI struct {
	path   string
	flavor string
}
type tickMsg time.Time
type spinnerTickMsg time.Time

// ── file picker ───────────────────────────────────────────────────────────────

type filePicker struct {
	active     bool
	query      string   // text typed after @
	atPosition int      // rune position of the @ in the input value
	files      []string // all files from git ls-files (relative paths)
	filtered   []string // fuzzy-matched subset
	selected   int      // index in filtered list
	maxVisible int      // max items to show (10)
	sessionID  string   // session ID the files were loaded for
	workingDir string   // working directory the files were loaded for
	loading    bool     // true while async file load is in progress
}

type filePickerFilesMsg struct {
	files      []string
	sessionID  string
	workingDir string
}

// filePickerLoadCmd runs git ls-files in the session's working directory.
func filePickerLoadCmd(sessionID, workingDir string) tea.Cmd {
	return func() tea.Msg {
		files := filePickerListFiles(workingDir)
		return filePickerFilesMsg{files: files, sessionID: sessionID, workingDir: workingDir}
	}
}

// filePickerListFiles returns relative file paths from the working directory.
// It tries git ls-files first, then falls back to a bounded directory walk.
func filePickerListFiles(dir string) []string {
	if dir == "" {
		return nil
	}
	// Try git ls-files first (fast, respects .gitignore).
	cmd := exec.Command("git", "ls-files")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err == nil {
		var files []string
		sc := bufio.NewScanner(bytes.NewReader(out))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line != "" {
				files = append(files, line)
			}
		}
		if len(files) > 0 {
			return files
		}
	}
	// Fallback: bounded directory walk (max 5000 files, max depth 8).
	const maxFiles = 5000
	const maxDepth = 8
	var files []string
	baseLen := len(strings.Split(filepath.Clean(dir), string(os.PathSeparator)))
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == ".next" || name == "__pycache__" || name == "vendor" {
				return filepath.SkipDir
			}
			depth := len(strings.Split(filepath.Clean(path), string(os.PathSeparator))) - baseLen
			if depth > maxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return nil
		}
		files = append(files, rel)
		if len(files) >= maxFiles {
			return filepath.SkipAll
		}
		return nil
	})
	return files
}

// filePickerFuzzyMatch returns files that match the query as a subsequence,
// sorted by score (exact prefix > path basename match > subsequence).
func filePickerFuzzyMatch(files []string, query string, limit int) []string {
	if query == "" {
		// Show all files up to limit, preferring shorter paths.
		if len(files) <= limit {
			return files
		}
		return files[:limit]
	}
	queryLower := strings.ToLower(query)
	type scored struct {
		path  string
		score int
	}
	var matches []scored
	for _, f := range files {
		fLower := strings.ToLower(f)
		base := strings.ToLower(filepath.Base(f))
		// Check subsequence match.
		if !isSubsequence(queryLower, fLower) {
			continue
		}
		score := 0
		// Exact prefix of full path.
		if strings.HasPrefix(fLower, queryLower) {
			score += 100
		}
		// Exact prefix of basename.
		if strings.HasPrefix(base, queryLower) {
			score += 80
		}
		// Contains the query as a substring.
		if strings.Contains(fLower, queryLower) {
			score += 50
		}
		// Basename contains the query as a substring.
		if strings.Contains(base, queryLower) {
			score += 40
		}
		// Prefer shorter paths.
		score -= len(f) / 10
		matches = append(matches, scored{path: f, score: score})
	}
	// Sort by score descending, then alphabetically.
	for i := 0; i < len(matches); i++ {
		for j := i + 1; j < len(matches); j++ {
			if matches[j].score > matches[i].score ||
				(matches[j].score == matches[i].score && matches[j].path < matches[i].path) {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}
	result := make([]string, 0, min(limit, len(matches)))
	for i := 0; i < len(matches) && i < limit; i++ {
		result = append(result, matches[i].path)
	}
	return result
}

func isSubsequence(needle, haystack string) bool {
	ni := 0
	for hi := 0; hi < len(haystack) && ni < len(needle); hi++ {
		if haystack[hi] == needle[ni] {
			ni++
		}
	}
	return ni == len(needle)
}

// filePickerDetect checks the textarea value and cursor position and returns
// (active, query, atPosition). It only activates when @ is preceded by
// whitespace or is at the start of the line, and the cursor is within the
// @ token (no whitespace between @ and cursor).
func filePickerDetect(value string, cursorPos int) (bool, string, int) {
	runes := []rune(value)
	if cursorPos < 0 || cursorPos > len(runes) {
		return false, "", 0
	}
	// Walk backwards from cursor to find the @ character.
	// Stop if we hit whitespace (no active @ mention at cursor).
	pos := cursorPos - 1
	for pos >= 0 {
		r := runes[pos]
		if r == '@' {
			// Found @. Check that it's at start or preceded by whitespace.
			if pos > 0 && !unicode.IsSpace(runes[pos-1]) {
				return false, "", 0
			}
			query := string(runes[pos+1 : cursorPos])
			return true, query, pos
		}
		if unicode.IsSpace(r) {
			// Hit whitespace before finding @.
			return false, "", 0
		}
		pos--
	}
	return false, "", 0
}

// ── model ─────────────────────────────────────────────────────────────────────

type tuiModel struct {
	api *tuiAPIClient

	width  int
	height int

	sessions        []WSSessionInfo
	selected        int
	activeSessionID string

	logs []string

	input    textarea.Model
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

	// unread indicators — set when a non-active session's LastMessage changes
	sessionUnread       map[string]bool
	sessionLastMessage  map[string]string

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

	// sub-agent hierarchy expansion (session ID → expanded)
	expandedSessions map[string]bool

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

	// toolCallCache tracks kind/title by toolCallID for history replay merging.
	toolCallCache map[string]WSToolCall

	// sub-agent collapsible feed blocks
	activeSubAgentStack []string                 // stack of sub-agent toolCallIDs (supports nesting)
	subAgentBlocks      map[string]*subAgentBlock // toolCallID → block metadata
	subAgentExpanded    map[string]bool           // toolCallID → expanded in feed

	// push-to-talk speech-to-text (space hold)
	pttLastSpace          time.Time
	pttActive             bool
	pttStarting           bool
	pttBusy               bool
	pttAudioPath          string
	pttProc               *exec.Cmd
	pttProcInput          io.WriteCloser
	pttStreaming          bool
	pttReleaseAt          time.Time
	pttSpaceRun           int
	pttTriggerValue       string
	pttTriggerCursor      int
	pttTriggerCaptured    bool
	pttInsertCursor       int
	pttInsertValueVersion string
	pttLiveText           string
	pttDisableNativeLive  bool

	// /model completion state
	modelCompLast       string
	modelCompCandidates []string
	modelCompIndex      int
	modelCompSessionID  string

	// sidebar visibility
	hideSidebar bool

	// cached max rows available for the input editor; set each View() cycle
	// so Update() can eagerly resize the textarea and keep scroll correct.
	inputMaxH int

	// active local whisper STT session (non-nil while PTT is active on the local path)
	pttLocalSession *localSTTSession

	// interactive permission-request overlay
	permRequest  *WSPermissionRequest
	permSelected int

	// pending desktop notifications awaiting AI summary from session refresh
	pendingNotifs map[string]pendingNotification // sessionID → notification

	// command palette
	palette commandPalette

	// undo/redo — per-session undo stacks and pre-run baseline tracking
	undoStacks   map[string]*undoStack // session ID → undo stack
	preRunDiff   map[string]string     // session ID → git diff HEAD output before a run
	preRunCommit map[string]string     // session ID → HEAD commit hash before a run

	// @ file picker
	picker filePicker

	// mouse hover state (-1 = none)
	hoverSession int

	// layout geometry cached each View() for mouse hit-testing
	layoutTopBarH    int
	layoutBannerH    int
	layoutLeftW      int
	layoutBodyH      int
	layoutSessionStart int // first visible session index
	layoutSessionEnd   int // one past last visible session index
}

func RunTUI(serverURL string, createNew bool, backend, model string, skip, plan bool) error {
	api, err := newTUIAPIClient(serverURL)
	if err != nil {
		return err
	}

	in := textarea.New()
	in.Placeholder = "Type prompt, @/path, or Ctrl+V"
	in.CharLimit = 32000
	in.ShowLineNumbers = false
	in.Prompt = "❯ "
	in.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("shift+enter", string([]rune{shiftEnterPrivate}), "ctrl+j"), key.WithHelp("shift+enter/ctrl+j", "newline"))
	in.KeyMap.WordBackward = key.NewBinding(key.WithKeys("alt+left", "ctrl+left", "alt+b"), key.WithHelp("ctrl+left", "word backward"))
	in.KeyMap.WordForward = key.NewBinding(key.WithKeys("alt+right", "ctrl+right", "alt+f"), key.WithHelp("ctrl+right", "word forward"))
	in.KeyMap.LineStart = key.NewBinding(key.WithKeys("home", "ctrl+a"), key.WithHelp("home", "line start"))
	in.KeyMap.LineEnd = key.NewBinding(key.WithKeys("end", "ctrl+e"), key.WithHelp("end", "line end"))
	focusedStyle, blurredStyle := textarea.DefaultStyles()
	focusedStyle.CursorLine = lipgloss.NewStyle()
	blurredStyle.CursorLine = lipgloss.NewStyle()
	focusedStyle.Prompt = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	blurredStyle.Prompt = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	focusedStyle.Text = lipgloss.NewStyle().Foreground(colText)
	blurredStyle.Text = lipgloss.NewStyle().Foreground(colText)
	focusedStyle.Placeholder = lipgloss.NewStyle().Foreground(colMuted)
	blurredStyle.Placeholder = lipgloss.NewStyle().Foreground(colMuted)
	in.FocusedStyle = focusedStyle
	in.BlurredStyle = blurredStyle
	in.Focus()

	m := &tuiModel{
		api:               api,
		input:             in,
		viewport:          viewport.New(60, 20),
		extCh:             make(chan tea.Msg, 256),
		sessionStateStart:  make(map[string]time.Time),
		sessionLastState:   make(map[string]string),
		sessionUnread:      make(map[string]bool),
		sessionLastMessage: make(map[string]string),
		toolCallCache:      make(map[string]WSToolCall),
		subAgentBlocks:     make(map[string]*subAgentBlock),
		subAgentExpanded:   make(map[string]bool),
		pendingNotifs:      make(map[string]pendingNotification),
		expandedSessions:  make(map[string]bool),
		palette:           newCommandPalette(),
		undoStacks:        make(map[string]*undoStack),
		preRunDiff:        make(map[string]string),
		preRunCommit:      make(map[string]string),
		hoverSession:      -1,
		historyPos:        0,
		agentBlockIdx:     -1,
		renderMarkdown:    true,
		compactBlocks:     true,
		thinkingLines:     []string{"idle"},
		picker:            filePicker{maxVisible: 10},
	}
	if pref, err := readThemePreference(); err == nil && pref != "" {
		if idx := themeIndexByName(pref); idx >= 0 {
			m.themeIdx = idx
			applyTheme(tuiThemes[idx])
		}
	}
	m.logSystem("Connected to " + api.baseURL)
	m.logSystem("Tab/Shift+Tab cycle sessions  ·  ↑/↓ scroll chat  ·  Enter connect/send  ·  Shift+Enter/Ctrl+J newline  ·  Ctrl+V attach/paste  ·  Ctrl+N new session  ·  Ctrl+D delete")
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
									m.extCh <- wsDisconnectedMsg{conn: conn, err: err}
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

	// Reset keyboard protocol state left over from a prior crash (the
	// disable sequences are no-ops if the protocol is not active).
	fmt.Print("\x1b[>4;0m\x1b[=0u")

	// Keyboard protocol enable sequences are sent from Init() after the
	// alt screen is active (so they apply to the alt-screen keyboard stack).
	// Disable sequences are sent here on exit to restore the terminal.
	defer fmt.Print("\x1b[>4;0m\x1b[=0u")

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithFilter(shiftEnterFilter))
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

// ── permission-request overlay ────────────────────────────────────────────────

func (m *tuiModel) updatePermRequest(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	d := m.permRequest
	switch msg.String() {
	case "ctrl+c":
		m.closeConn()
		return m, tea.Quit
	case "esc":
		m.permRequest = nil
		return m, nil
	case "up", "k":
		if m.permSelected > 0 {
			m.permSelected--
		}
		return m, nil
	case "down", "j":
		if m.permSelected < len(d.Options)-1 {
			m.permSelected++
		}
		return m, nil
	case "enter":
		opt := d.Options[m.permSelected]
		m.permRequest = nil
		return m, sendWSCmd(m, map[string]any{
			"type":      "permission_response",
			"requestId": d.RequestID,
			"optionId":  opt.OptionID,
		})
	}
	// number keys 1–9 for quick selection
	if len(msg.String()) == 1 && msg.String() >= "1" && msg.String() <= "9" {
		idx := int(msg.String()[0]-'1')
		if idx < len(d.Options) {
			opt := d.Options[idx]
			m.permRequest = nil
			return m, sendWSCmd(m, map[string]any{
				"type":      "permission_response",
				"requestId": d.RequestID,
				"optionId":  opt.OptionID,
			})
		}
	}
	return m, nil
}

func (m *tuiModel) renderPermRequest() string {
	d := m.permRequest
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(colYellow)
	selOpt := lipgloss.NewStyle().Foreground(colGreen).Bold(true)
	dimOpt := lipgloss.NewStyle().Foreground(colText)
	kindStyle := lipgloss.NewStyle().Foreground(colMuted)

	var lines []string
	lines = append(lines, titleStyle.Render("  ⏸ Permission Required"))
	lines = append(lines, "")
	lines = append(lines, "  "+styleText.Render(d.Title))
	if d.Command != "" {
		lines = append(lines, "")
		lines = append(lines, styleMuted.Render("  $ ")+styleText.Render(d.Command))
	}
	lines = append(lines, "")
	for i, o := range d.Options {
		prefix := "  "
		if len(d.Options) <= 9 {
			prefix = fmt.Sprintf("  %d. ", i+1)
		}
		name := o.Name
		kind := ""
		if o.Kind != "" {
			kind = "  " + kindStyle.Render(o.Kind)
		}
		if i == m.permSelected {
			lines = append(lines, selOpt.Render(prefix+"▶ "+name)+kind)
		} else {
			lines = append(lines, dimOpt.Render(prefix+"  "+name)+kind)
		}
	}
	lines = append(lines, "")
	lines = append(lines, styleMuted.Render("  ↑/↓=select  Enter=confirm  Esc=dismiss"))

	panel := lipgloss.NewStyle().
		Background(colPanel).
		Border(lipgloss.NormalBorder()).
		BorderForeground(colYellow).
		Padding(0, 1).
		Width(60).
		Render(strings.Join(lines, "\n"))

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel)
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
		row("Ctrl+N", "new session wizard"),
		row("z", "toggle agent zoo view"),
		row("e", "expand/collapse sub-agents"),
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
		row("Ctrl+P", "command palette"),
		row("Ctrl+S", "toggle sidebar"),
		row("Ctrl+Z", "undo file changes"),
		row("Ctrl+Y", "redo file changes"),
		row("Ctrl+G", "expand/collapse sub-agent blocks"),
		row("Ctrl+. / Ctrl+\\", "abort running session"),
		row("Ctrl+← / Ctrl+→", "move by word"),
		"",
		head.Render("  Mouse"),
		row("Click session", "select session"),
		row("Double-click session", "connect to session"),
		row("Hover session", "highlight session"),
		row("Click wizard/perm option", "select option"),
		row("Click sub-agent header", "expand/collapse sub-agent"),
		row("Scroll wheel", "scroll feed or sessions"),
		"",
		head.Render("  Sub-Agents"),
		row("Ctrl+G", "expand/collapse all sub-agent blocks"),
		row("Click ▸/▾ in feed", "toggle individual sub-agent"),
		row("e", "expand sub-agents in sidebar"),
		"",
		head.Render("  Session"),
		row("Enter (with text)", "send prompt to session"),
		row("Shift+Enter / Ctrl+J", "insert newline in prompt"),
		row("Alt+Enter", "send prompt to cloned session"),
		row("Ctrl+V", "paste clipboard image or file path"),
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
		row("/undo", "undo last file changes"),
		row("/redo", "redo undone changes"),
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

// ── command palette ──────────────────────────────────────────────────────────

type paletteAction struct {
	name string
	hint string
}

type commandPalette struct {
	open            bool
	filterInput     textinput.Model
	selected        int
	actions         []paletteAction
	filteredActions []paletteAction
}

func newCommandPalette() commandPalette {
	ti := textinput.New()
	ti.Placeholder = "Type to filter..."
	ti.CharLimit = 100
	ti.Focus()
	return commandPalette{
		filterInput: ti,
		actions: []paletteAction{
			{name: "New Session", hint: "Ctrl+N"},
			{name: "Delete Session", hint: "Ctrl+D"},
			{name: "Toggle Sidebar", hint: "Ctrl+S"},
			{name: "Toggle Markdown", hint: "Ctrl+M"},
			{name: "Toggle Blocks", hint: "Ctrl+B"},
			{name: "Cycle Theme", hint: "Ctrl+T"},
			{name: "Toggle Help", hint: "?"},
			{name: "Toggle AI Zoo", hint: "z"},
			{name: "Expand Sub-agents", hint: "e"},
			{name: "Interrupt Session", hint: "Ctrl+\\"},
			{name: "Refresh Sessions", hint: "Ctrl+R"},
			{name: "Clear Feed", hint: "Ctrl+L"},
			{name: "Fork Session", hint: "Alt+Enter"},
			{name: "Toggle Plan Mode", hint: ""},
			{name: "Toggle Skip Permissions", hint: ""},
			{name: "Undo File Changes", hint: "Ctrl+Z"},
			{name: "Redo File Changes", hint: "Ctrl+Y"},
		},
	}
}

func (p *commandPalette) activate() {
	p.open = true
	p.filterInput.SetValue("")
	p.filterInput.Focus()
	p.selected = 0
	p.refilter()
}

func (p *commandPalette) close() {
	p.open = false
	p.filterInput.Blur()
}

func (p *commandPalette) refilter() {
	query := strings.ToLower(strings.TrimSpace(p.filterInput.Value()))
	if query == "" {
		p.filteredActions = make([]paletteAction, len(p.actions))
		copy(p.filteredActions, p.actions)
	} else {
		p.filteredActions = nil
		for _, a := range p.actions {
			if strings.Contains(strings.ToLower(a.name), query) {
				p.filteredActions = append(p.filteredActions, a)
			}
		}
	}
	if p.selected >= len(p.filteredActions) {
		p.selected = max(0, len(p.filteredActions)-1)
	}
}

func (m *tuiModel) openPalette() {
	m.palette.activate()
}

func (m *tuiModel) closePalette() {
	m.palette.close()
}

func (m *tuiModel) updatePalette(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.closeConn()
		return m, tea.Quit
	case "esc", "ctrl+p":
		m.closePalette()
		return m, nil
	case "up":
		if m.palette.selected > 0 {
			m.palette.selected--
		}
		return m, nil
	case "down":
		if m.palette.selected < len(m.palette.filteredActions)-1 {
			m.palette.selected++
		}
		return m, nil
	case "enter":
		if len(m.palette.filteredActions) == 0 {
			m.closePalette()
			return m, nil
		}
		action := m.palette.filteredActions[m.palette.selected]
		m.closePalette()
		return m.executePaletteAction(action.name)
	}
	// Forward to the text input for filtering.
	var cmd tea.Cmd
	m.palette.filterInput, cmd = m.palette.filterInput.Update(msg)
	m.palette.refilter()
	return m, cmd
}

func (m *tuiModel) executePaletteAction(name string) (tea.Model, tea.Cmd) {
	switch name {
	case "New Session":
		if m.input.Value() == "" {
			m.openWizard()
		}
		return m, nil
	case "Delete Session":
		if len(m.sessions) == 0 {
			return m, nil
		}
		m.deleteConfirmID = m.sessions[m.selected].ID
		return m, nil
	case "Toggle Sidebar":
		m.hideSidebar = !m.hideSidebar
		m.resize()
		return m, nil
	case "Toggle Markdown":
		m.renderMarkdown = !m.renderMarkdown
		m.logSystem("Markdown rendering: " + boolLabel(m.renderMarkdown))
		m.rebuildViewport()
		return m, nil
	case "Toggle Blocks":
		m.compactBlocks = !m.compactBlocks
		if m.compactBlocks {
			m.logSystem("Block mode: compact")
		} else {
			m.logSystem("Block mode: full")
		}
		return m, nil
	case "Cycle Theme":
		m.themeIdx = (m.themeIdx + 1) % len(tuiThemes)
		applyTheme(tuiThemes[m.themeIdx])
		if err := writeThemePreference(tuiThemes[m.themeIdx].Name); err != nil {
			m.logSystem("theme persistence warning: " + err.Error())
		}
		m.logSystem("Theme: " + tuiThemes[m.themeIdx].Name)
		m.rebuildViewport()
		return m, nil
	case "Toggle Help":
		m.showHelp = !m.showHelp
		return m, nil
	case "Toggle AI Zoo":
		m.showZoo = true
		return m, nil
	case "Expand Sub-agents":
		if len(m.sessions) > 0 {
			id := m.sessions[m.selected].ID
			m.expandedSessions[id] = !m.expandedSessions[id]
		}
		return m, nil
	case "Interrupt Session":
		if m.activeSessionID != "" {
			return m, sendWSCmd(m, map[string]any{"type": "interrupt"})
		}
		return m, nil
	case "Refresh Sessions":
		return m, refreshSessionsCmd(m.api)
	case "Clear Feed":
		m.logs = nil
		m.clearSubAgentState()
		m.rebuildViewport()
		return m, nil
	case "Fork Session":
		raw := m.input.Value()
		if strings.TrimSpace(raw) == "" || m.activeSessionID == "" {
			return m, nil
		}
		m.input.SetValue("")
		m.syncInputChrome()
		m.historyPos = 0
		m.historyLive = ""
		if len(m.inputHistory) == 0 || m.inputHistory[len(m.inputHistory)-1] != raw {
			m.inputHistory = append(m.inputHistory, raw)
			if len(m.inputHistory) > 100 {
				m.inputHistory = m.inputHistory[1:]
			}
		}
		return m, clonePromptCmd(m.api, m.activeSessionID, raw)
	case "Toggle Plan Mode":
		if len(m.sessions) == 0 {
			m.logSystem("No sessions available")
			return m, nil
		}
		target := m.sessions[m.selected]
		return m, updateSessionCmd(m.api, target.ID, target.SkipPermissions, !target.PlanMode, nil)
	case "Toggle Skip Permissions":
		if len(m.sessions) == 0 {
			m.logSystem("No sessions available")
			return m, nil
		}
		target := m.sessions[m.selected]
		return m, updateSessionCmd(m.api, target.ID, !target.SkipPermissions, target.PlanMode, nil)
	case "Undo File Changes":
		if cmd := m.performUndo(); cmd != nil {
			return m, cmd
		}
		return m, nil
	case "Redo File Changes":
		if cmd := m.performRedo(); cmd != nil {
			return m, cmd
		}
		return m, nil
	}
	return m, nil
}

func (m *tuiModel) renderPalette() string {
	const paletteW = 52
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	hintStyle := lipgloss.NewStyle().Foreground(colMuted)
	normalStyle := lipgloss.NewStyle().Foreground(colText)
	selStyle := lipgloss.NewStyle().Foreground(colAccent).Bold(true)

	var lines []string
	lines = append(lines, titleStyle.Render("  Command Palette"))
	lines = append(lines, "")
	lines = append(lines, "  "+m.palette.filterInput.View())
	lines = append(lines, "")

	if len(m.palette.filteredActions) == 0 {
		lines = append(lines, hintStyle.Render("  No matching commands"))
	} else {
		maxVisible := min(len(m.palette.filteredActions), max(5, m.height-10))
		// Scroll window around selected item.
		start := 0
		if m.palette.selected >= maxVisible {
			start = m.palette.selected - maxVisible + 1
		}
		end := min(start+maxVisible, len(m.palette.filteredActions))

		for i := start; i < end; i++ {
			a := m.palette.filteredActions[i]
			nameStr := a.name
			hintStr := ""
			if a.hint != "" {
				hintStr = "  " + hintStyle.Render(a.hint)
			}
			if i == m.palette.selected {
				lines = append(lines, "  "+selStyle.Render("> "+nameStr)+hintStr)
			} else {
				lines = append(lines, "    "+normalStyle.Render(nameStr)+hintStr)
			}
		}
	}

	lines = append(lines, "")
	lines = append(lines, hintStyle.Render("  Up/Down=navigate  Enter=select  Esc=close"))

	panel := lipgloss.NewStyle().
		Background(colPanel).
		Border(lipgloss.NormalBorder()).
		BorderForeground(colBorder).
		Padding(0, 1).
		Width(paletteW).
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
		// Enable keyboard enhancement protocols so that Shift+Enter and
		// Alt+Enter send distinct escape sequences. Written directly to stdout
		// here (after alt-screen entry) so the sequences apply to the
		// alt-screen keyboard stack, not the main screen.
		// \x1b[>4;2m – xterm modifyOtherKeys level 2
		// \x1b[=1u   – Kitty keyboard protocol disambiguate flag
		enableKittyKeyboardCmd(),
		refreshSessionsCmd(m.api),
		waitExternalCmd(m.extCh),
		tickCmd(),
		spinnerTickCmd(),
		zooTickCmd(),
		prewarmDarwinSpeechHelperCmd(),
		prewarmLocalSTTModelCmd(),
	)
}

// prewarmDarwinSpeechHelperCmd compiles the native dictation helper in the
// background at startup so the first dictation attempt is instant. Errors are
// silently ignored — the helper will try again (and surface the error) when
// dictation is actually invoked.
func prewarmDarwinSpeechHelperCmd() tea.Cmd {
	if runtime.GOOS != "darwin" {
		return nil
	}
	return func() tea.Msg {
		_, _ = ensureDarwinSpeechHelperBinary()
		return nil
	}
}

func enableKittyKeyboardCmd() tea.Cmd {
	return func() tea.Msg {
		os.Stdout.WriteString("\x1b[>4;2m\x1b[=1u")
		return nil
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

// handleMouse processes mouse events for click and hover interactions.
func (m *tuiModel) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	x, y := msg.X, msg.Y

	// ── scroll wheel (always active) ─────────────────────────────────────
	if msg.Button == tea.MouseButtonWheelUp {
		if !m.hideSidebar && x < m.layoutLeftW {
			// scroll sessions: select previous
			if m.selected > 0 {
				m.selected--
			}
		} else {
			m.viewport.ScrollUp(3)
		}
		return m, nil
	}
	if msg.Button == tea.MouseButtonWheelDown {
		if !m.hideSidebar && x < m.layoutLeftW {
			if m.selected < len(m.sessions)-1 {
				m.selected++
			}
		} else {
			m.viewport.ScrollDown(3)
		}
		return m, nil
	}

	// ── wizard overlay ───────────────────────────────────────────────────
	if m.wizardActive {
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			return m.handleWizardClick(x, y)
		}
		return m, nil
	}

	// ── permission overlay ───────────────────────────────────────────────
	if m.permRequest != nil {
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			return m.handlePermClick(x, y)
		}
		return m, nil
	}

	// ── zoo view ─────────────────────────────────────────────────────────
	if m.showZoo {
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			return m.handleZooClick(x, y)
		}
		return m, nil
	}

	// ── main view: session sidebar click / hover ─────────────────────────
	if !m.hideSidebar && x < m.layoutLeftW {
		bodyTop := m.layoutTopBarH + m.layoutBannerH
		// +2 accounts for the panel border (1) + "sessions" header (1)
		sessionAreaTop := bodyTop + 2
		localY := y - sessionAreaTop

		if localY >= 0 && len(m.sessions) > 0 {
			// Walk visible sessions to find which one was hit
			row := 0
			hitIdx := -1
			for i := m.layoutSessionStart; i < m.layoutSessionEnd; i++ {
				lines := m.sessionVisualLines(i)
				if localY >= row && localY < row+lines {
					hitIdx = i
					break
				}
				row += lines
			}

			if msg.Action == tea.MouseActionMotion {
				if hitIdx >= 0 {
					m.hoverSession = hitIdx
				} else {
					m.hoverSession = -1
				}
				return m, nil
			}

			if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && hitIdx >= 0 {
				m.hoverSession = -1
				if hitIdx == m.selected {
					// Click on already-selected session → connect to it
					return m, attachSessionCmd(m.api, m.sessions[hitIdx].ID, m.extCh)
				}
				m.selected = hitIdx
				return m, nil
			}
		}
	} else {
		// Clear hover when mouse leaves sidebar
		if m.hoverSession != -1 {
			m.hoverSession = -1
		}

		// Click in the feed area — check if it hit a sub-agent summary line.
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			if tcID := m.hitTestSubAgentSummary(y); tcID != "" {
				m.toggleSubAgentBlock(tcID)
				return m, nil
			}
		}
	}

	return m, nil
}

// hitTestSubAgentSummary checks if the given screen Y coordinate corresponds
// to a sub-agent summary line in the viewport and returns its toolCallID.
func (m *tuiModel) hitTestSubAgentSummary(screenY int) string {
	// The feed viewport starts after: topBar + banners + details panel header area.
	// We need the viewport's screen-relative Y offset. The feed box is in the
	// right column, below the details panel.
	// Rather than compute the exact pixel offset, we use the viewport's own
	// scroll offset: viewport line = screenY - feedScreenTop.
	// For simplicity, we build a mapping from viewport line → log index.

	if len(m.subAgentBlocks) == 0 {
		return ""
	}

	// Build hidden set for collapsed children.
	hidden := make(map[int]bool)
	for tcID, block := range m.subAgentBlocks {
		if !m.subAgentExpanded[tcID] {
			for _, idx := range block.childIndices {
				hidden[idx] = true
			}
		}
	}

	// Build summaryIndices set for quick lookup.
	summaryToTC := make(map[int]string)
	for tcID, block := range m.subAgentBlocks {
		summaryToTC[block.summaryIdx] = tcID
	}

	// Walk visible logs, counting rendered lines, to find which log index
	// the clicked viewport line corresponds to.
	viewportLine := m.viewport.YOffset + (screenY - m.viewportScreenTop())
	if viewportLine < 0 {
		return ""
	}

	line := 0
	for i, entry := range m.logs {
		if hidden[i] {
			continue
		}
		entryLines := strings.Count(entry, "\n") + 1 + 1 // +1 for content, +1 for blank line after
		if viewportLine >= line && viewportLine < line+entryLines {
			if tcID, ok := summaryToTC[i]; ok {
				return tcID
			}
			return ""
		}
		line += entryLines
	}
	return ""
}

// viewportScreenTop returns the screen Y coordinate where the feed viewport starts.
func (m *tuiModel) viewportScreenTop() int {
	// topBar + banners + details panel (with border)
	detailsH := 8
	if m.layoutBodyH < 20 {
		detailsH = 6
	}
	// +2 for the detail panel border/chrome, +1 for feed header
	return m.layoutTopBarH + m.layoutBannerH + detailsH + 2 + 1
}

// handleWizardClick maps a click in the wizard overlay to a field/option selection.
func (m *tuiModel) handleWizardClick(x, y int) (tea.Model, tea.Cmd) {
	const wizW = 60
	// The wizard is centered; compute its bounding box.
	panelLeft := (m.width - wizW) / 2
	panelRight := panelLeft + wizW
	if x < panelLeft || x > panelRight {
		return m, nil
	}

	// Lines inside the panel (0-indexed from panel top):
	// 0: border
	// 1: "  New Session"
	// 2: ""
	// 3: "  Working Directory"
	// 4: input
	// 5: ""
	// 6: "  Backend"
	// 7+: backend options ...
	// Then model, skip, mode sections follow.
	//
	// We compute the panel vertical center and offset from there.
	models := m.wizardCurrentModels()

	// Determine total content lines to find panel top
	contentH := 2 + 2 + (1 + len(wizardBackends)) + 1 + (1 + len(models)) + 1 + (1 + len(wizardSkipOptions)) + 1 + (1 + len(wizardModeOptions)) + 1 + 1
	panelTop := (m.height - contentH) / 2
	localY := y - panelTop - 1 // -1 for border

	if localY < 0 {
		return m, nil
	}

	line := 0

	// Title + blank
	line += 2

	// Working Directory section (label + input)
	if localY >= line && localY < line+2 {
		m.wizardFocus = 0
		m.wizardDirInput.Focus()
		return m, nil
	}
	line += 3 // label + input + blank

	// Backend section
	if localY == line {
		m.wizardFocus = 1
		m.wizardDirInput.Blur()
		return m, nil
	}
	line++ // label
	for i := range wizardBackends {
		if localY == line+i {
			m.wizardFocus = 1
			m.wizardDirInput.Blur()
			m.wizardBackend = i
			m.wizardModel = 0
			return m, nil
		}
	}
	line += len(wizardBackends) + 1 // options + blank

	// Model section
	if localY == line {
		m.wizardFocus = 2
		m.wizardDirInput.Blur()
		return m, nil
	}
	line++
	for i := range models {
		if localY == line+i {
			m.wizardFocus = 2
			m.wizardDirInput.Blur()
			m.wizardModel = i
			return m, nil
		}
	}
	line += len(models) + 1

	// Skip Permissions section
	if localY == line {
		m.wizardFocus = 3
		m.wizardDirInput.Blur()
		return m, nil
	}
	line++
	for i := range wizardSkipOptions {
		if localY == line+i {
			m.wizardFocus = 3
			m.wizardDirInput.Blur()
			m.wizardSkip = i
			return m, nil
		}
	}
	line += len(wizardSkipOptions) + 1

	// Mode section
	if localY == line {
		m.wizardFocus = 4
		m.wizardDirInput.Blur()
		return m, nil
	}
	line++
	for i := range wizardModeOptions {
		if localY == line+i {
			m.wizardFocus = 4
			m.wizardDirInput.Blur()
			m.wizardMode = i
			return m, nil
		}
	}

	return m, nil
}

// handlePermClick maps a click in the permission overlay to an option.
func (m *tuiModel) handlePermClick(x, y int) (tea.Model, tea.Cmd) {
	d := m.permRequest
	const panelW = 60
	panelLeft := (m.width - panelW) / 2
	panelRight := panelLeft + panelW
	if x < panelLeft || x > panelRight {
		return m, nil
	}

	// Content lines:
	// 0: border
	// 1: "  ⏸ Permission Required"
	// 2: ""
	// 3: title
	// 4: "" (if command present)
	// 5: command (if present)
	// 6: ""
	// 7+: options
	headerLines := 4 // title + blank + title text + blank
	if d.Command != "" {
		headerLines += 2 // blank + command
	}

	contentH := headerLines + len(d.Options) + 2 // + blank + hint
	panelTop := (m.height - contentH) / 2
	localY := y - panelTop - 1 // -1 for border

	optStart := headerLines
	optIdx := localY - optStart
	if optIdx >= 0 && optIdx < len(d.Options) {
		opt := d.Options[optIdx]
		m.permRequest = nil
		return m, sendWSCmd(m, map[string]any{
			"type":      "permission_response",
			"requestId": d.RequestID,
			"optionId":  opt.OptionID,
		})
	}

	return m, nil
}

// handleZooClick selects or connects to an agent in the zoo view based on click position.
func (m *tuiModel) handleZooClick(x, y int) (tea.Model, tea.Cmd) {
	if len(m.sessions) == 0 {
		return m, nil
	}

	// Zoo bots are rendered at specific canvas positions. Find the closest bot.
	bestIdx := -1
	bestDist := 999999
	for i, bot := range m.zooBots {
		dx := x - int(bot.x)
		dy := y - int(bot.y)
		dist := dx*dx + dy*dy
		if dist < bestDist {
			bestDist = dist
			bestIdx = i
		}
	}

	// Only select if click is reasonably close to a bot (within ~10 cells)
	if bestIdx >= 0 && bestDist <= 100 {
		if m.selected == bestIdx {
			// Double-click effect: connect to the session
			m.showZoo = false
			return m, attachSessionCmd(m.api, m.sessions[bestIdx].ID, m.extCh)
		}
		m.selected = bestIdx
	}

	return m, nil
}

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case spinnerTickMsg:
		m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
		// Update running sub-agent summaries so the spinner animates.
		for _, block := range m.subAgentBlocks {
			if block.status == "running" && block.summaryIdx >= 0 && block.summaryIdx < len(m.logs) {
				m.logs[block.summaryIdx] = m.renderSubAgentSummary(block)
			}
		}
		if m.pttActive && time.Since(m.pttLastSpace) > pttReleaseDebounce {
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
		cmds := []tea.Cmd{refreshSessionsCmd(m.api), tickCmd()}
		if m.wsReconnecting && m.activeSessionID != "" {
			cmds = append(cmds, attachSessionCmd(m.api, m.activeSessionID, m.extCh))
		}
		return m, tea.Batch(cmds...)

	case sessionsMsg:
		if msg.err != nil {
			m.logSystem("Refresh failed: " + msg.err.Error())
			return m, nil
		}
		// Track state changes to measure elapsed time per session.
		// Also detect new LastMessage on non-active sessions → mark unread.
		for _, s := range msg.sessions {
			state := sessionStateLabel(s)
			if m.sessionLastState[s.ID] != state {
				m.sessionLastState[s.ID] = state
				m.sessionStateStart[s.ID] = time.Now()
			}
			if s.ID != m.activeSessionID {
				prev, seen := m.sessionLastMessage[s.ID]
				if seen && s.LastMessage != "" && s.LastMessage != prev {
					m.sessionUnread[s.ID] = true
				}
			}
			m.sessionLastMessage[s.ID] = s.LastMessage
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
		m.flushPendingNotifications()
		return m, nil

	case attachMsg:
		if msg.err != nil {
			m.logSystem("Connect failed: " + msg.err.Error())
			return m, nil
		}
		m.swapConn(msg.conn)
		m.activeSessionID = msg.sessionID
		m.wsReconnecting = false
		delete(m.sessionUnread, msg.sessionID)
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
		m.pttStarting = false
		m.pttBusy = false
		m.pttActive = false
		m.pttStreaming = false
		m.pttProcInput = nil
		if msg.disableNative && !m.pttDisableNativeLive {
			m.pttDisableNativeLive = true
			if msg.disableNote != "" {
				m.logSystem(msg.disableNote)
			}
		}
		if m.pttAudioPath != "" {
			_ = os.Remove(m.pttAudioPath)
			m.pttAudioPath = ""
		}
		if msg.err != nil {
			m.resetPTTTrigger()
			// disableNative errors are already described by the disableNote logged above.
			if !msg.disableNative {
				m.logSystem("Dictation failed: " + msg.err.Error())
			}
			if msg.external {
				return m, waitExternalCmd(m.extCh)
			}
			return m, nil
		}
		if strings.TrimSpace(msg.text) != "" {
			m.applyDictationText(strings.TrimSpace(msg.text))
			switch {
			case msg.releaseToTextDelay > 0:
				m.logSystem(fmt.Sprintf("Dictation inserted (%s live)", formatDurationRounded(msg.releaseToTextDelay)))
			case msg.captureStopDelay >= pttShutdownLogFloor || msg.transcribeDelay >= pttTranscriptionLogFloor:
				m.logSystem(fmt.Sprintf("Dictation inserted (%s stop, %s transcribe)", formatDurationRounded(msg.captureStopDelay), formatDurationRounded(msg.transcribeDelay)))
			default:
				m.logSystem("Dictation inserted")
			}
		} else if msg.external {
			// Native streaming path returned empty — audio engine ran but nothing was
			// transcribed. Most commonly a TCC permission issue after a binary update.
			m.logSystem("No speech detected — if this persists, check Microphone and Speech Recognition permissions for your terminal in System Settings > Privacy & Security, then run: rm -rf ~/.orbitor/bin")
		}
		m.resetPTTTrigger()
		if msg.external {
			return m, waitExternalCmd(m.extCh)
		}
		return m, nil

	case sttPartialMsg:
		if strings.TrimSpace(msg.text) == "" {
			if msg.external {
				return m, waitExternalCmd(m.extCh)
			}
			return m, nil
		}
		m.applyDictationText(msg.text)
		if msg.external {
			return m, waitExternalCmd(m.extCh)
		}
		return m, nil

	case clipboardPasteMsg:
		if msg.err != nil {
			m.logSystem("Paste failed: " + msg.err.Error())
			return m, nil
		}
		if strings.TrimSpace(msg.insert) == "" {
			return m, nil
		}
		if strings.HasPrefix(strings.TrimSpace(msg.insert), "@") {
			m.insertPromptTokenAtCursor(strings.TrimSpace(msg.insert))
		} else {
			m.insertAtCursor(msg.insert)
		}
		if msg.note != "" {
			m.logSystem(msg.note)
		}
		return m, nil

	case sttStartedMsg:
		m.pttStarting = false
		if msg.disableNative && !m.pttDisableNativeLive {
			m.pttDisableNativeLive = true
			if msg.disableNote != "" {
				m.logSystem(msg.disableNote)
			}
		}
		if msg.err != nil {
			m.pttBusy = false
			m.pttActive = false
			m.pttProcInput = nil
			m.pttStreaming = false
			m.resetPTTTrigger()
			m.logSystem("Dictation start failed: " + msg.err.Error())
			return m, nil
		}
		m.pttProc = msg.proc
		m.pttProcInput = msg.stdin
		m.pttAudioPath = msg.audioPath
		m.pttStreaming = msg.streaming
		m.pttLocalSession = msg.localSession
		m.pttReleaseAt = time.Time{}
		m.pttActive = true
		m.pttBusy = true
		m.pttSpaceRun = pttHoldThreshold
		m.pttInsertCursor = m.inputCursorPosition()
		m.pttInsertValueVersion = m.input.Value()
		if msg.streaming {
			m.logSystem("🎙 live dictation listening (hold space), release to finish")
		} else {
			m.logSystem("🎙 dictation listening (hold space), release to transcribe")
		}
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

	case wsDisconnectedMsg:
		m.connMu.Lock()
		isCurrent := m.conn == msg.conn
		m.connMu.Unlock()
		if isCurrent {
			m.wsReconnecting = true
			m.wsReconnectSince = time.Now()
		}
		m.logSystem("Session disconnected: " + msg.err.Error())
		return m, waitExternalCmd(m.extCh)

	case infoMsg:
		m.logSystem(msg.text)
		return m, waitExternalCmd(m.extCh)

	case errMsg:
		if msg.err != nil {
			m.log(styleRed.Render("✗ ") + msg.err.Error())
		}
		return m, waitExternalCmd(m.extCh)

	case filePickerFilesMsg:
		m.picker.loading = false
		if msg.sessionID == m.picker.sessionID && msg.workingDir == m.picker.workingDir {
			m.picker.files = msg.files
			m.picker.filtered = filePickerFuzzyMatch(m.picker.files, m.picker.query, m.picker.maxVisible)
			m.picker.selected = 0
		}
		return m, nil

	case undoResultMsg:
		if msg.err != nil {
			// On failure, restore the snapshot to the stack it came from.
			stack := m.sessionUndoStack(msg.sessionID)
			if msg.wasUndo {
				stack.undoable = append(stack.undoable, msg.snapshot)
			} else {
				stack.redoable = append(stack.redoable, msg.snapshot)
			}
			m.logSystem(msg.err.Error())
		} else {
			// On success, move the snapshot to the opposite stack.
			stack := m.sessionUndoStack(msg.sessionID)
			if msg.wasUndo {
				stack.redoable = append(stack.redoable, msg.snapshot)
			} else {
				stack.undoable = append(stack.undoable, msg.snapshot)
			}
			m.logSystem(msg.text)
		}
		return m, nil

	case tea.KeyMsg:
		// Discard raw SGR mouse escape sequences that leak through when the
		// terminal sends scroll events Bubble Tea doesn't parse (e.g. scrolling
		// past the bottom of the viewport). These look like [<65;109;35M.
		if s := msg.String(); len(s) > 2 && strings.HasPrefix(s, "[<") {
			return m, nil
		}
		if msg.String() != "tab" && msg.String() != "shift+tab" {
			m.resetModelCompletion()
		}
		if msg.String() != " " {
			m.resetPTTSpaceRun()
		}
		// When wizard is open, route all key events to it.
		if m.wizardActive {
			return m.updateWizard(msg)
		}
		// When a permission request is pending, route all key events to it.
		if m.permRequest != nil {
			return m.updatePermRequest(msg)
		}
		// When command palette is open, route all key events to it.
		if m.palette.open {
			return m.updatePalette(msg)
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

		// When file picker is active, intercept navigation keys.
		if m.picker.active {
			switch msg.String() {
			case "up":
				if m.picker.selected > 0 {
					m.picker.selected--
				}
				return m, nil
			case "down":
				if m.picker.selected < len(m.picker.filtered)-1 {
					m.picker.selected++
				}
				return m, nil
			case "tab", "enter":
				if len(m.picker.filtered) > 0 && m.picker.selected < len(m.picker.filtered) {
					m.filePickerComplete()
				}
				return m, nil
			case "esc":
				m.picker.active = false
				return m, nil
			}
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
			m.clearSubAgentState()
			m.rebuildViewport()
			return m, nil

		case "ctrl+d":
			if len(m.sessions) == 0 {
				return m, nil
			}
			m.deleteConfirmID = m.sessions[m.selected].ID
			return m, nil

		case " ":
			m.pttLastSpace = time.Now()
			if m.pttActive || m.pttBusy || m.pttStarting {
				m.pttSpaceRun = pttHoldThreshold
				return m, nil
			}
			m.capturePTTTriggerSnapshot()
			m.pttSpaceRun++
			if m.pttSpaceRun >= pttHoldThreshold {
				m.restorePTTTriggerSnapshot()
				m.pttStarting = true
				return m, m.startPTTCapture()
			}

		case "up":
			li := m.input.LineInfo()
			if m.input.Line() > 0 || li.RowOffset > 0 {
				break // let textarea move cursor up (logical or visual line)
			}
			m.viewport.ScrollUp(3)
			return m, nil

		case "down":
			li := m.input.LineInfo()
			lastLogLine := strings.Count(m.input.Value(), "\n")
			if m.input.Line() < lastLogLine || li.RowOffset < li.Height-1 {
				break // let textarea move cursor down (logical or visual line)
			}
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
				m.setInputValueAndCursor(m.input.Value(), len([]rune(m.input.Value())))
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
			m.setInputValueAndCursor(m.input.Value(), len([]rune(m.input.Value())))
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

		case "ctrl+v":
			return m, pasteClipboardCmd()

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

		case "ctrl+g":
			m.toggleSubAgentExpansion()
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

		case "ctrl+p":
			m.openPalette()
			return m, nil

		case "ctrl+s":
			m.hideSidebar = !m.hideSidebar
			m.resize()
			return m, nil

		case "ctrl+z":
			if cmd := m.performUndo(); cmd != nil {
				return m, cmd
			}
			return m, nil

		case "ctrl+y":
			if cmd := m.performRedo(); cmd != nil {
				return m, cmd
			}
			return m, nil

		case "ctrl+n":
			if m.input.Value() == "" {
				m.openWizard()
				return m, nil
			}

		case "z":
			if m.input.Value() == "" {
				m.showZoo = true
				return m, nil
			}

		case "e":
			if m.input.Value() == "" && len(m.sessions) > 0 {
				id := m.sessions[m.selected].ID
				m.expandedSessions[id] = !m.expandedSessions[id]
				return m, nil
			}

		case "enter":
			raw := m.input.Value()
			if strings.TrimSpace(raw) == "" {
				if len(m.sessions) == 0 {
					m.logSystem("No sessions available")
					return m, nil
				}
				return m, attachSessionCmd(m.api, m.sessions[m.selected].ID, m.extCh)
			}
			m.input.SetValue("")
			m.syncInputChrome()
			m.historyPos = 0
			m.historyLive = ""
			commandText := strings.TrimSpace(raw)
			if !strings.Contains(raw, "\n") && strings.HasPrefix(commandText, "/") {
				return m, m.handleCommand(commandText)
			}
			// Push to input history (deduplicate consecutive identical entries).
			if len(m.inputHistory) == 0 || m.inputHistory[len(m.inputHistory)-1] != raw {
				m.inputHistory = append(m.inputHistory, raw)
				if len(m.inputHistory) > 100 {
					m.inputHistory = m.inputHistory[1:]
				}
			}
			if m.activeSessionID == "" {
				m.logSystem("Connect to a session first (select and press Enter)")
				return m, nil
			}
			return m, sendWSCmd(m, map[string]any{"type": "prompt", "text": raw})

		case "alt+enter", string([]rune{altEnterPrivate}):
			raw := m.input.Value()
			if strings.TrimSpace(raw) == "" {
				return m, nil
			}
			m.input.SetValue("")
			m.syncInputChrome()
			m.historyPos = 0
			m.historyLive = ""
			if len(m.inputHistory) == 0 || m.inputHistory[len(m.inputHistory)-1] != raw {
				m.inputHistory = append(m.inputHistory, raw)
				if len(m.inputHistory) > 100 {
					m.inputHistory = m.inputHistory[1:]
				}
			}
			if m.activeSessionID == "" {
				m.logSystem("Connect to a session first (select and press Enter)")
				return m, nil
			}
			return m, clonePromptCmd(m.api, m.activeSessionID, raw)
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	// Eagerly resize the textarea so its internal scroll offset is
	// recalculated with the correct height before the next View() call.
	// Without this, inserting a newline when height=1 causes the textarea to
	// scroll the first line out of view before View() can widen the box.
	if m.inputMaxH > 0 {
		m.input.SetHeight(m.promptEditorHeight(m.inputMaxH - 1))
	}
	// Detect @ mentions for the file picker after every textarea change.
	pickerCmd := m.filePickerSync()
	if pickerCmd != nil {
		cmd = tea.Batch(cmd, pickerCmd)
	}
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

	if m.permRequest != nil {
		return m.renderPermRequest()
	}

	if m.showHelp {
		return m.renderHelp()
	}

	if m.palette.open {
		return m.renderPalette()
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
	var leftW, rightW int
	if m.hideSidebar {
		leftW = 0
		rightW = m.width
	} else {
		leftW = max(22, m.width*19/100)
		rightW = m.width - leftW
		if rightW < 50 {
			rightW = 50
			leftW = m.width - rightW
		}
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
	m.layoutTopBarH = topBarH

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
	m.layoutBannerH = bannerH

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
	// Reserve space for file picker popup when active.
	pickerH := 0
	if m.picker.active && (len(m.picker.filtered) > 0 || m.picker.loading) {
		pickerH = min(len(m.picker.filtered)+2, m.picker.maxVisible+2) // +2 for header+border
		if m.picker.loading && len(m.picker.filtered) == 0 {
			pickerH = 3
		}
		if len(m.picker.files) > len(m.picker.filtered) && m.picker.query != "" {
			pickerH++ // "N more" line
		}
		pickerH += 2 // top and bottom border
	}
	inputMaxH := max(3, bodyH-detailsH-thinkingH-pickerH-6)
	m.inputMaxH = inputMaxH
	inputEditorH := m.promptEditorHeight(inputMaxH - 1)
	m.input.SetHeight(inputEditorH)
	inputH := inputEditorH + 1
	feedH := max(6, bodyH-detailsH-thinkingH-inputH-pickerH)

	m.layoutLeftW = leftW
	m.layoutBodyH = bodyH

	// ── left panel (sessions) ─────────────────────────────────────────────────
	var left string
	if !m.hideSidebar {
		sessionsAvailH := max(3, bodyH-1)
		listW := max(20, leftW-4) // inner content width for separator lines
		sessionsHeader := styleAccent.Render(" sessions")
		if m.missionSummary != "" {
			sessionsHeader += styleMuted.Render("  " + trimForLine(m.missionSummary, leftW-16))
		}
		left = panelStyle.Width(leftW - 2).Height(bodyH).Render(
			clampLines(sessionsHeader+"\n"+m.renderMissionControl(selectedStyle, activeStyle, sessionsAvailH, listW), bodyH),
		)
	}

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

	var hint string
	if m.activeSessionID != "" {
		hint = "Enter=send(queue)  ·  Shift+Enter/Ctrl+J=new line  ·  Alt+Enter=fork send  ·  Ctrl+V=paste image/path  ·  @/path=file mention  ·  hold Space=dictate  ·  Ctrl+./Ctrl+\\=abort  ·  ↑/↓ scroll"
	} else {
		hint = "Enter=connect  ·  Shift+Enter=new line  ·  Ctrl+V=paste image/path  ·  @/path=file mention  ·  hold Space=dictate  ·  Tab cycle sessions  ·  ↑/↓ scroll"
	}
	if m.isThinking {
		hint += "  ·  agent running"
	}
	m.syncInputChrome()
	inputBox := panelStyle.Width(rightW - 2).Height(inputH).Render(
		m.input.View() + "\n" + styleMuted.Render("  "+hint),
	)

	pickerPopup := m.renderFilePicker(rightW)
	var rightParts []string
	rightParts = append(rightParts, detailBox, feedBox, thinkingBox)
	if pickerPopup != "" {
		rightParts = append(rightParts, pickerPopup)
	}
	rightParts = append(rightParts, inputBox)
	right := lipgloss.JoinVertical(lipgloss.Left, rightParts...)
	var mainRow string
	if m.hideSidebar {
		mainRow = right
	} else {
		mainRow = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}

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
		m.logSystem("  /undo")
		m.logSystem("  /redo")
		m.logSystem("  /restart")
		m.logSystem("  /delete [id]")
		m.logSystem("  /quit")
		m.logSystem("Hotkeys: ctrl+n=new session  tab/shift+tab=cycle sessions  e=expand sub-agents  up/down=scroll chat  ctrl+up/down=prompt history  ctrl+v=paste image/path  @/path=file mention  alt+enter=fork prompt  ctrl+d=delete  ctrl+l=clear  ctrl+m=markdown  ctrl+b=blocks  ctrl+t=theme  ctrl+p=command palette  ctrl+s=sidebar  ctrl+z=undo  ctrl+y=redo  ctrl+./ctrl+\\=abort  ctrl/alt+left/right=word move  PgUp/PgDn=scroll")
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
	case "/undo":
		return m.performUndo()
	case "/redo":
		return m.performRedo()
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
		m.clearSubAgentState()
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

func (m *tuiModel) renderChatBubble(role, ts, text string, border lipgloss.Color, alignRight bool) string {
	maxBubbleWidth := max(28, min(m.chatWidth()-6, m.chatWidth()*4/5))
	header := lipgloss.NewStyle().
		Foreground(colMuted).
		Bold(true).
		Render(role) + styleMuted.Render("  "+ts)
	body := m.renderRichTextBlock(text, maxBubbleWidth-4, false)
	bubble := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1).
		MaxWidth(maxBubbleWidth).
		Render(header + "\n" + body)
	if alignRight {
		return lipgloss.NewStyle().Width(m.chatWidth()).Align(lipgloss.Right).Render(bubble)
	}
	return bubble
}

func (m *tuiModel) renderToolChatBubble(role, ts, meta, content string, border lipgloss.Color, alignRight bool) string {
	maxBubbleWidth := max(28, min(m.chatWidth()-6, m.chatWidth()*4/5))
	header := lipgloss.NewStyle().
		Foreground(colMuted).
		Bold(true).
		Render(role) + styleMuted.Render("  "+ts)
	body := m.renderMarkdownBlock(meta, maxBubbleWidth-4, false)
	if strings.TrimSpace(content) != "" {
		body += "\n\n" + m.renderRichTextBlock(content, maxBubbleWidth-4, true)
	}
	bubble := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1).
		MaxWidth(maxBubbleWidth).
		Render(header + "\n" + body)
	if alignRight {
		return lipgloss.NewStyle().Width(m.chatWidth()).Align(lipgloss.Right).Render(bubble)
	}
	return bubble
}

func (m *tuiModel) renderFeedSection(title string, titleStyle lipgloss.Style, body string) string {
	titleLine := titleStyle.Render(" " + title + " ")
	block := body
	if strings.TrimSpace(block) == "" {
		block = styleMuted.Render("  (empty)")
	}
	return titleLine + "\n" + block
}

// ── sub-agent scope helpers ───────────────────────────────────────────────────

// activeSubAgent returns the toolCallID of the innermost active sub-agent, or "".
func (m *tuiModel) activeSubAgent() string {
	if len(m.activeSubAgentStack) == 0 {
		return ""
	}
	return m.activeSubAgentStack[len(m.activeSubAgentStack)-1]
}

// pushSubAgent enters a new sub-agent scope.
func (m *tuiModel) pushSubAgent(toolCallID string) {
	m.activeSubAgentStack = append(m.activeSubAgentStack, toolCallID)
}

// popSubAgent exits the sub-agent scope for the given toolCallID.
// Handles out-of-order completion by removing from anywhere in the stack.
func (m *tuiModel) popSubAgent(toolCallID string) {
	for i := len(m.activeSubAgentStack) - 1; i >= 0; i-- {
		if m.activeSubAgentStack[i] == toolCallID {
			m.activeSubAgentStack = append(m.activeSubAgentStack[:i], m.activeSubAgentStack[i+1:]...)
			return
		}
	}
}

// trackChildLog records a log index as belonging to the current active sub-agent.
func (m *tuiModel) trackChildLog(idx int) {
	if sa := m.activeSubAgent(); sa != "" {
		if block, ok := m.subAgentBlocks[sa]; ok {
			block.childIndices = append(block.childIndices, idx)
		}
	}
}

// logInSubAgent appends to m.logs and tracks the index as a sub-agent child if applicable.
func (m *tuiModel) logInSubAgent(s string) {
	idx := len(m.logs)
	m.logs = append(m.logs, s)
	if len(m.logs) > 4000 {
		m.trimLogsWithSubAgents()
	}
	m.trackChildLog(idx)
	if !m.replayingHistory {
		m.rebuildViewport()
	}
}

// clearSubAgentState resets all sub-agent tracking (for feed clear / session switch).
func (m *tuiModel) clearSubAgentState() {
	m.activeSubAgentStack = nil
	m.subAgentBlocks = make(map[string]*subAgentBlock)
	// Keep subAgentExpanded so user's toggle preference persists.
}

// toggleSubAgentExpansion toggles expansion of all sub-agent blocks in the feed.
func (m *tuiModel) toggleSubAgentExpansion() {
	if len(m.subAgentBlocks) == 0 {
		return
	}
	// Determine if we should expand all or collapse all.
	// If any are expanded, collapse all; otherwise expand all.
	anyExpanded := false
	for tcID := range m.subAgentBlocks {
		if m.subAgentExpanded[tcID] {
			anyExpanded = true
			break
		}
	}
	for tcID, block := range m.subAgentBlocks {
		m.subAgentExpanded[tcID] = !anyExpanded
		if block.summaryIdx >= 0 && block.summaryIdx < len(m.logs) {
			m.logs[block.summaryIdx] = m.renderSubAgentSummary(block)
		}
	}
	m.rebuildViewport()
}

// toggleSubAgentBlock toggles expansion of a single sub-agent block by toolCallID.
func (m *tuiModel) toggleSubAgentBlock(toolCallID string) {
	block, ok := m.subAgentBlocks[toolCallID]
	if !ok {
		return
	}
	m.subAgentExpanded[toolCallID] = !m.subAgentExpanded[toolCallID]
	if block.summaryIdx >= 0 && block.summaryIdx < len(m.logs) {
		m.logs[block.summaryIdx] = m.renderSubAgentSummary(block)
	}
	m.rebuildViewport()
}

// renderSubAgentSummary renders the collapsible one-line summary for a sub-agent block.
func (m *tuiModel) renderSubAgentSummary(block *subAgentBlock) string {
	expanded := m.subAgentExpanded[block.toolCallID]

	var icon string
	var iconStyle lipgloss.Style
	switch block.status {
	case "running":
		icon = spinnerFrames[m.spinnerFrame]
		iconStyle = styleOrange
	case "completed":
		icon = "✓"
		iconStyle = styleGreen
	case "failed":
		icon = "✗"
		iconStyle = styleRed
	default:
		icon = "●"
		iconStyle = styleMuted
	}

	chevron := "▸" // collapsed
	if expanded {
		chevron = "▾"
	}

	steps := len(block.childIndices)
	elapsed := ""
	if !block.startedAt.IsZero() {
		if block.status == "running" {
			elapsed = formatElapsed(time.Since(block.startedAt))
		} else if !block.completedAt.IsZero() {
			elapsed = formatElapsed(block.completedAt.Sub(block.startedAt))
		}
	}

	w := m.chatWidth()
	summary := fmt.Sprintf(" %s %s sub-agent: %s", chevron, icon, block.title)
	meta := fmt.Sprintf("  %d steps", steps)
	if elapsed != "" {
		meta += "  " + elapsed
	}

	line := iconStyle.Render(summary) + styleMuted.Render(meta)
	sep := styleSep.Render(strings.Repeat("─", max(1, w)))

	if expanded {
		return sep + "\n" + line
	}
	return sep + "\n" + line + "\n" + sep
}

// trimLogsWithSubAgents trims m.logs to 4000 entries while adjusting all
// sub-agent block indices to account for the removed prefix.
func (m *tuiModel) trimLogsWithSubAgents() {
	if len(m.logs) <= 4000 {
		return
	}
	trimCount := len(m.logs) - 4000
	m.logs = m.logs[trimCount:]

	// Adjust all sub-agent block indices.
	for tcID, block := range m.subAgentBlocks {
		block.summaryIdx -= trimCount
		if block.summaryIdx < 0 {
			// Summary was trimmed away; remove the entire block.
			delete(m.subAgentBlocks, tcID)
			continue
		}
		newChildren := block.childIndices[:0]
		for _, idx := range block.childIndices {
			adjusted := idx - trimCount
			if adjusted >= 0 {
				newChildren = append(newChildren, adjusted)
			}
		}
		block.childIndices = newChildren
	}
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
			m.logs[m.agentBlockIdx] = m.renderChatBubble("assistant", m.agentBlockTime.Format("15:04"), m.agentBlockText, colAccent, false)
		} else {
			m.agentBlockIdx = len(m.logs)
			m.agentBlockTime = time.Now()
			m.agentBlockText = d.Text
			m.logs = append(m.logs, m.renderChatBubble("assistant", tsStr, d.Text, colAccent, false))
			// Track this new log entry as a sub-agent child if applicable.
			m.trackChildLog(m.agentBlockIdx)
			if len(m.logs) > 4000 {
				m.trimLogsWithSubAgents()
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
			m.log(m.renderChatBubble("you", tsStr, d.Text, colCyan, true))
			m.isThinking = true
			m.pushThinking("prompt sent")
			// Capture pre-run git baseline for undo tracking.
			if !m.replayingHistory && m.activeSessionID != "" {
				m.capturePreRunBaseline(m.activeSessionID)
			}
		}

	case "tool_call":
		var d WSToolCall
		if json.Unmarshal(msg.Data, &d) == nil {
			// Client-side cache: merge kind/title from previous events for the
			// same tool call ID so history replay also shows correct names.
			if d.ToolCallID != "" {
				if cached, ok := m.toolCallCache[d.ToolCallID]; ok {
					if d.Kind == "" {
						d.Kind = cached.Kind
					}
					if d.Title == "" {
						d.Title = cached.Title
					}
				}
				entry := m.toolCallCache[d.ToolCallID]
				if d.Kind != "" {
					entry.Kind = d.Kind
				}
				if d.Title != "" {
					entry.Title = d.Title
				}
				m.toolCallCache[d.ToolCallID] = entry
			}
			toolName := defaultString(d.Kind, "tool")
			callTitle := defaultString(d.Title, toolName)
			status := defaultString(d.Status, "running")

			// Detect sub-agent spawn: insert a collapsible summary and enter scope.
			if isSubAgentKind(toolName) {
				if _, exists := m.subAgentBlocks[d.ToolCallID]; !exists {
					block := &subAgentBlock{
						toolCallID: d.ToolCallID,
						title:      callTitle,
						status:     "running",
						summaryIdx: len(m.logs),
						startedAt:  time.Now(),
					}
					m.subAgentBlocks[d.ToolCallID] = block
					m.pushSubAgent(d.ToolCallID)
					m.logs = append(m.logs, m.renderSubAgentSummary(block))
					if !m.replayingHistory {
						m.rebuildViewport()
					}
					m.isThinking = true
					break
				}
			}

			icon, _ := toolKindIcon(toolName)
			metaParts := []string{
				fmt.Sprintf("**Tool:** `%s`", toolName),
				fmt.Sprintf("**Call:** %s %s", icon, callTitle),
				fmt.Sprintf("**Status:** `%s`", status),
			}
			m.logInSubAgent(m.renderToolChatBubble("assistant · tool", tsStr, strings.Join(metaParts, "\n"), d.Content, colOrange, false))
			m.isThinking = true
		}

	case "tool_result":
		var d WSToolResult
		if json.Unmarshal(msg.Data, &d) == nil {
			// Check if this result completes a sub-agent.
			if block, ok := m.subAgentBlocks[d.ToolCallID]; ok {
				block.status = "completed"
				block.completedAt = time.Now()
				m.popSubAgent(d.ToolCallID)
				// Add the result content as a child if non-empty.
				if d.Content != "" {
					idx := len(m.logs)
					m.logs = append(m.logs, m.renderFeedSection("tool result", styleGreen, m.renderRichTextBlock(d.Content, w, true)))
					block.childIndices = append(block.childIndices, idx)
				}
				// Update the summary line to reflect completion.
				if block.summaryIdx >= 0 && block.summaryIdx < len(m.logs) {
					m.logs[block.summaryIdx] = m.renderSubAgentSummary(block)
				}
				if !m.replayingHistory {
					m.rebuildViewport()
				}
				break
			}
			// Regular (non-sub-agent) tool result.
			if d.Content != "" {
				m.logInSubAgent(m.renderFeedSection("tool result", styleGreen, m.renderRichTextBlock(d.Content, w, true)))
			}
		}

	case "permission_request":
		var d WSPermissionRequest
		if json.Unmarshal(msg.Data, &d) == nil {
			if !m.replayingHistory {
				m.permRequest = &d
				m.permSelected = 0
				go sendNotification("Permission needed", m.sessionDisplayName()+" is waiting for approval")
			} else {
				// During history replay just log it inline; no interactive overlay.
				m.log("")
				m.log("  " + styleYellow.Render("⏸ permission required") + styleMuted.Render("  "+d.Title))
				if d.Command != "" {
					m.log(styleMuted.Render("    $ ") + styleText.Render(d.Command))
				}
				for _, o := range d.Options {
					m.log("    " + styleCyan.Render("["+o.OptionID+"]") + "  " + styleText.Render(o.Name) + styleMuted.Render("  "+o.Kind))
				}
			}
			m.isThinking = true
		}

	case "permission_resolved":
		var d struct {
			RequestID string `json:"requestId"`
			OptionID  string `json:"optionId"`
		}
		if json.Unmarshal(msg.Data, &d) == nil {
			// Clear the overlay if it's still showing (e.g. resolved from another client).
			if m.permRequest != nil && m.permRequest.RequestID == d.RequestID {
				m.permRequest = nil
			}
			m.log("  " + styleGreen.Render("✓ approved") + styleMuted.Render("  "+d.OptionID))
			m.isThinking = false
		}

	case "run_complete":
		var d struct {
			StopReason string `json:"stopReason"`
			PRURL      string `json:"prUrl"`
		}
		if json.Unmarshal(msg.Data, &d) == nil {
			entry := m.renderFeedSection("run complete", styleGreen, styleGreen.Render("  ✓ finished"))
			if d.StopReason != "" && d.StopReason != "end_turn" {
				entry += "\n" + styleMuted.Render("  reason: "+d.StopReason)
			}
			m.log(entry)
			if d.PRURL != "" {
				m.log(styleCyan.Render("  PR: ") + d.PRURL)
			}
			if !m.replayingHistory {
				m.pendingNotifs[m.activeSessionID] = pendingNotification{
					sessionID:  m.activeSessionID,
					stopReason: d.StopReason,
					prURL:      d.PRURL,
					createdAt:  time.Now(),
				}
			}
			// Capture post-run git snapshot for undo tracking.
			if !m.replayingHistory && m.activeSessionID != "" {
				m.capturePostRunSnapshot(m.activeSessionID)
			}
			m.isThinking = false
			m.pushThinking("run complete")
		}

	case "status":
		var d struct {
			Status string `json:"status"`
		}
		if json.Unmarshal(msg.Data, &d) == nil && d.Status != "" {
			m.log(m.renderFeedSection("status", styleMuted, styleMuted.Render("  ◦ "+d.Status)))
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
			m.log(m.renderFeedSection("error", styleRed, styleRed.Render("  ✗ ")+styleText.Render(d.Message)))
			m.isThinking = false
			m.pushThinking("error: " + trimForLine(d.Message, 70))
		}
	case "interrupted":
		m.isThinking = false
		m.log(m.renderFeedSection("interrupt", styleMuted, styleMuted.Render("  · interrupted")))
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
	codeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("251")).
		Background(lipgloss.Color("236")).
		Padding(0, 1)
	for i := 0; i < maxLines; i++ {
		line := lines[i]
		trim := strings.TrimSpace(line)

		if strings.HasPrefix(trim, "```") {
			inCode = !inCode
			codeLang = strings.TrimSpace(strings.TrimPrefix(trim, "```"))
			if inCode && codeLang != "" {
				out = append(out, styleMuted.Render("  ┌ code: "+codeLang))
			}
			if !inCode {
				out = append(out, styleMuted.Render("  └"))
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
	// Expand tabs to 4 spaces so that lipgloss.Width and terminal rendering agree.
	diff = strings.ReplaceAll(diff, "\t", "    ")
	lines := strings.Split(strings.ReplaceAll(diff, "\r\n", "\n"), "\n")
	maxLines := len(lines)
	if m.compactBlocks {
		maxLines = min(maxLines, 24)
	}
	var out []string
	leftWidth := max(12, (width-5)/2)
	rightWidth := max(12, width-leftWidth-3)
	flushPaired := func(left, right []string) {
		rows := max(len(left), len(right))
		for i := 0; i < rows; i++ {
			var lText, rText string
			if i < len(left) {
				lText = styleRed.Render(" - " + trimForLine(left[i], max(6, leftWidth-3)))
			}
			if i < len(right) {
				rText = styleGreen.Render(" + " + trimForLine(right[i], max(6, rightWidth-3)))
			}
			if lText == "" {
				lText = styleMuted.Render("   ")
			}
			if rText == "" {
				rText = styleMuted.Render("   ")
			}
			out = append(out, padToWidth(lText, leftWidth)+styleSep.Render(" │ ")+padToWidth(rText, rightWidth))
		}
	}
	var pendingLeft []string
	var pendingRight []string
	flushPending := func() {
		if len(pendingLeft) == 0 && len(pendingRight) == 0 {
			return
		}
		flushPaired(pendingLeft, pendingRight)
		pendingLeft = nil
		pendingRight = nil
	}
	for i := 0; i < maxLines; i++ {
		l := lines[i]
		switch {
		case strings.HasPrefix(l, "diff --git"):
			flushPending()
			out = append(out, styleAccent.Render("▌ split diff  "+trimForLine(strings.TrimPrefix(l, "diff --git "), max(12, width-14))))
		case strings.HasPrefix(l, "index "):
			flushPending()
			out = append(out, styleMuted.Render("  "+trimForLine(l, max(12, width-2))))
		case strings.HasPrefix(l, "@@"):
			flushPending()
			out = append(out, styleCyan.Render("┃ "+trimForLine(l, max(12, width-2))))
		case strings.HasPrefix(l, "+++"), strings.HasPrefix(l, "---"):
			flushPending()
			out = append(out, styleViolet.Render("│ "+trimForLine(l, max(12, width-2))))
		case strings.HasPrefix(l, "+"):
			pendingRight = append(pendingRight, strings.TrimPrefix(l, "+"))
		case strings.HasPrefix(l, "-"):
			pendingLeft = append(pendingLeft, strings.TrimPrefix(l, "-"))
		case strings.HasPrefix(l, " "):
			flushPending()
			body := trimForLine(strings.TrimPrefix(l, " "), max(6, min(leftWidth, rightWidth)-3))
			leftCell := styleMuted.Render("   " + body)
			rightCell := styleMuted.Render("   " + body)
			out = append(out, padToWidth(leftCell, leftWidth)+styleSep.Render(" │ ")+padToWidth(rightCell, rightWidth))
		case strings.HasPrefix(l, "\\ No newline at end of file"):
			flushPending()
			out = append(out, styleMuted.Render("  "+trimForLine(l, max(12, width-2))))
		default:
			flushPending()
			out = append(out, styleText.Render("┃   "+trimForLine(l, max(12, width-4))))
		}
	}
	flushPending()
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

const pendingNotifTimeout = 12 * time.Second

// flushPendingNotifications checks pending run_complete notifications against
// the current session data. If a session now has an AI-generated summary,
// fire an enriched notification. After a timeout, fire with basic info.
func (m *tuiModel) flushPendingNotifications() {
	if len(m.pendingNotifs) == 0 {
		return
	}

	// Build a lookup of current sessions by ID.
	sessionByID := make(map[string]WSSessionInfo, len(m.sessions))
	for _, s := range m.sessions {
		sessionByID[s.ID] = s
	}

	for id, pn := range m.pendingNotifs {
		s, exists := sessionByID[id]
		timedOut := time.Since(pn.createdAt) > pendingNotifTimeout
		hasSummary := exists && s.Summary != ""

		if hasSummary || timedOut {
			title := "Agent finished"
			name := "session"
			if exists {
				name = defaultString(s.Title, filepath.Base(s.WorkingDir))
			}

			var body string
			if hasSummary {
				body = name + "\n" + s.Summary
			} else {
				body = name
				if pn.stopReason != "" {
					body += " — " + pn.stopReason
				}
			}
			if pn.prURL != "" {
				body += "\nPR: " + pn.prURL
			}

			go sendNotification(title, body)
			delete(m.pendingNotifs, id)
		}
	}
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
	var leftW, rightW int
	if m.hideSidebar {
		leftW = 0
		rightW = m.width
	} else {
		leftW = max(22, m.width*19/100)
		rightW = m.width - leftW
		if rightW < 50 {
			rightW = 50
			leftW = m.width - rightW
		}
	}
	vw := max(24, rightW-8)
	m.viewport.Width = vw
	m.input.SetWidth(max(18, rightW-8))
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

	// Build set of hidden log indices from collapsed sub-agent blocks.
	hidden := make(map[int]bool)
	for tcID, block := range m.subAgentBlocks {
		if !m.subAgentExpanded[tcID] {
			for _, idx := range block.childIndices {
				hidden[idx] = true
			}
		}
	}

	var out []string
	for i, entry := range m.logs {
		if hidden[i] {
			continue
		}
		lines := strings.Split(entry, "\n")
		for _, line := range lines {
			out = append(out, bg.Render(padToWidth(line, w)))
		}
		out = append(out, bg.Render(padToWidth("", w)))
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
		m.trimLogsWithSubAgents()
	}
	if !m.replayingHistory {
		m.rebuildViewport()
	}
}

func (m *tuiModel) syncInputChrome() {
	promptStyle := lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	if m.activeSessionID == "" {
		promptStyle = lipgloss.NewStyle().Foreground(colMuted).Bold(true)
	}
	m.input.Prompt = "❯ "
	m.input.FocusedStyle.Prompt = promptStyle
	m.input.BlurredStyle.Prompt = promptStyle
	m.input.FocusedStyle.Text = lipgloss.NewStyle().Foreground(colText)
	m.input.BlurredStyle.Text = lipgloss.NewStyle().Foreground(colText)
	m.input.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(colMuted)
	m.input.BlurredStyle.Placeholder = lipgloss.NewStyle().Foreground(colMuted)
}

func promptVisualLineCount(text string, width int) int {
	width = max(1, width)
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return 1
	}
	total := 0
	for _, line := range lines {
		lineWidth := lipgloss.Width(line)
		if lineWidth == 0 {
			total++
			continue
		}
		total += max(1, (lineWidth+width-1)/width)
	}
	return max(1, total)
}

func (m *tuiModel) promptEditorHeight(maxRows int) int {
	maxRows = max(1, maxRows)
	// Use the textarea's actual content width so line-count matches its wrapping.
	usableWidth := max(8, m.input.Width())
	content := m.input.Value()
	if content == "" {
		content = m.input.Placeholder
	}
	return min(maxRows, max(1, promptVisualLineCount(content, usableWidth)))
}

func inputRowCol(value string, pos int) (int, int) {
	runes := []rune(value)
	pos = clamp(pos, 0, len(runes))
	row := 0
	col := 0
	for i := 0; i < pos; i++ {
		if runes[i] == '\n' {
			row++
			col = 0
			continue
		}
		col++
	}
	return row, col
}

func inputAbsolutePosition(value string, row, col int) int {
	lines := strings.Split(value, "\n")
	if len(lines) == 0 {
		return 0
	}
	row = clamp(row, 0, len(lines)-1)
	pos := 0
	for i := 0; i < row; i++ {
		pos += len([]rune(lines[i])) + 1
	}
	lineRunes := []rune(lines[row])
	col = clamp(col, 0, len(lineRunes))
	return pos + col
}

func (m *tuiModel) inputCursorPosition() int {
	value := m.input.Value()
	lines := strings.Split(value, "\n")
	row := clamp(m.input.Line(), 0, max(0, len(lines)-1))
	col := m.input.LineInfo().CharOffset
	if len(lines) == 0 {
		return 0
	}
	col = clamp(col, 0, len([]rune(lines[row])))
	return inputAbsolutePosition(value, row, col)
}

func (m *tuiModel) setInputValueAndCursor(value string, pos int) {
	m.input.Reset()
	m.input.SetValue(value)
	remaining := len([]rune(value)) - clamp(pos, 0, len([]rune(value)))
	for i := 0; i < remaining; i++ {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(tea.KeyMsg{Type: tea.KeyLeft})
		_ = cmd
	}
}

func (m *tuiModel) insertAtCursor(s string) {
	if s == "" {
		return
	}
	v := []rune(m.input.Value())
	pos := m.inputCursorPosition()
	if pos < 0 {
		pos = 0
	}
	if pos > len(v) {
		pos = len(v)
	}
	ins := []rune(s)
	newVal := string(v[:pos]) + string(ins) + string(v[pos:])
	m.setInputValueAndCursor(newVal, pos+len(ins))
}

func (m *tuiModel) insertPromptTokenAtCursor(token string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	runes := []rune(m.input.Value())
	pos := m.inputCursorPosition()
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	prefix := ""
	suffix := ""
	if pos > 0 && !isSpacingRune(runes[pos-1]) {
		prefix = " "
	}
	if pos < len(runes) && !isSpacingRune(runes[pos]) {
		suffix = " "
	}
	m.insertAtCursor(prefix + token + suffix)
}

var errNoClipboardImage = errors.New("no image found in clipboard")

func pasteClipboardCmd() tea.Cmd {
	return func() tea.Msg {
		insert, note, err := clipboardPasteInsertion()
		return clipboardPasteMsg{insert: insert, note: note, err: err}
	}
}

func clipboardPasteInsertion() (string, string, error) {
	if token, note, err := clipboardImagePromptToken(); err == nil {
		return token, note, nil
	} else if err != nil && !errors.Is(err, errNoClipboardImage) {
		// Fall back to text clipboard handling below.
	}

	text, err := readClipboardText()
	if err != nil {
		return "", "", err
	}
	if token, note, ok := clipboardTextPromptInsert(text); ok {
		return token, note, nil
	}
	text = strings.TrimRight(text, "\r\n")
	if strings.TrimSpace(text) == "" {
		return "", "", fmt.Errorf("clipboard is empty")
	}
	return text, "Pasted clipboard text", nil
}

func clipboardImagePromptToken() (string, string, error) {
	data, err := readClipboardImage()
	if err != nil {
		return "", "", err
	}
	path, err := saveClipboardImage(data)
	if err != nil {
		return "", "", err
	}
	return "@" + path, "Attached clipboard image " + homeTildePath(path), nil
}

func readClipboardImage() ([]byte, error) {
	switch runtime.GOOS {
	case "darwin":
		return readDarwinClipboardImage()
	case "linux":
		return readLinuxClipboardImage()
	default:
		return nil, errNoClipboardImage
	}
}

func readDarwinClipboardImage() ([]byte, error) {
	swiftPath, err := exec.LookPath("swift")
	if err != nil {
		return nil, errNoClipboardImage
	}
	script := `import AppKit
let pasteboard = NSPasteboard.general
guard let image = NSImage(pasteboard: pasteboard) else { exit(2) }
guard let tiff = image.tiffRepresentation,
      let rep = NSBitmapImageRep(data: tiff),
      let png = rep.representation(using: .png, properties: [:]) else { exit(3) }
FileHandle.standardOutput.write(png)`
	out, err := exec.Command(swiftPath, "-e", script).Output()
	if err != nil || len(out) == 0 {
		return nil, errNoClipboardImage
	}
	return out, nil
}

func readLinuxClipboardImage() ([]byte, error) {
	candidates := [][]string{
		{"wl-paste", "--no-newline", "--type", "image/png"},
		{"xclip", "-selection", "clipboard", "-t", "image/png", "-o"},
	}
	for _, candidate := range candidates {
		path, err := exec.LookPath(candidate[0])
		if err != nil {
			continue
		}
		out, err := exec.Command(path, candidate[1:]...).Output()
		if err == nil && len(out) > 0 {
			return out, nil
		}
	}
	return nil, errNoClipboardImage
}

func saveClipboardImage(data []byte) (string, error) {
	dir := filepath.Join(os.TempDir(), "orbitor-pasted")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("clipboard-image-%d.png", time.Now().UnixNano()))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func readClipboardText() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		path, err := exec.LookPath("pbpaste")
		if err != nil {
			return "", fmt.Errorf("pbpaste not available")
		}
		out, err := exec.Command(path).Output()
		if err != nil {
			return "", err
		}
		return string(out), nil
	case "linux":
		candidates := [][]string{
			{"wl-paste", "--no-newline"},
			{"xclip", "-selection", "clipboard", "-o"},
			{"xsel", "--clipboard", "--output"},
		}
		for _, candidate := range candidates {
			path, err := exec.LookPath(candidate[0])
			if err != nil {
				continue
			}
			out, err := exec.Command(path, candidate[1:]...).Output()
			if err == nil {
				return string(out), nil
			}
		}
		return "", fmt.Errorf("clipboard text command not available")
	case "windows":
		path, err := exec.LookPath("powershell")
		if err != nil {
			return "", fmt.Errorf("powershell not available")
		}
		out, err := exec.Command(path, "-NoProfile", "-Command", "Get-Clipboard").Output()
		if err != nil {
			return "", err
		}
		return string(out), nil
	default:
		return "", fmt.Errorf("clipboard paste not supported on %s", runtime.GOOS)
	}
}

func clipboardTextPromptInsert(text string) (string, string, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", "", false
	}
	if strings.HasPrefix(trimmed, "@") {
		return trimmed, "Pasted file mention " + trimForLine(trimmed, 80), true
	}
	lines := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == '\n' || r == '\r'
	})
	if len(lines) == 0 {
		return "", "", false
	}
	tokens := make([]string, 0, len(lines))
	for _, line := range lines {
		path, ok := normalizeClipboardFilePath(line)
		if !ok {
			return "", "", false
		}
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			return "", "", false
		}
		tokens = append(tokens, "@"+path)
	}
	if len(tokens) == 0 {
		return "", "", false
	}
	note := "Attached file " + homeTildePath(strings.TrimPrefix(tokens[0], "@"))
	if len(tokens) > 1 {
		note = fmt.Sprintf("Attached %d files from clipboard paths", len(tokens))
	}
	return strings.Join(tokens, " "), note, true
}

func normalizeClipboardFilePath(raw string) (string, bool) {
	s := strings.TrimSpace(strings.Trim(raw, `"'`))
	if s == "" {
		return "", false
	}
	if strings.HasPrefix(s, "@") {
		s = strings.TrimSpace(strings.TrimPrefix(s, "@"))
	}
	if strings.HasPrefix(s, "file://") {
		u, err := url.Parse(s)
		if err != nil || (u.Host != "" && u.Host != "localhost") {
			return "", false
		}
		s = u.Path
	}
	switch {
	case strings.HasPrefix(s, "~/"):
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false
		}
		s = filepath.Join(home, strings.TrimPrefix(s, "~/"))
	case strings.HasPrefix(s, "/"):
		// already absolute
	case strings.HasPrefix(s, "./"), strings.HasPrefix(s, "../"):
		abs, err := filepath.Abs(s)
		if err != nil {
			return "", false
		}
		s = abs
	default:
		if !strings.ContainsRune(s, os.PathSeparator) {
			return "", false
		}
		abs, err := filepath.Abs(s)
		if err != nil {
			return "", false
		}
		s = abs
	}
	return filepath.Clean(s), true
}

func (m *tuiModel) capturePTTTriggerSnapshot() {
	if m.pttTriggerCaptured {
		return
	}
	m.pttTriggerCaptured = true
	m.pttTriggerValue = m.input.Value()
	m.pttTriggerCursor = m.inputCursorPosition()
}

func (m *tuiModel) restorePTTTriggerSnapshot() {
	if !m.pttTriggerCaptured {
		return
	}
	m.setInputValueAndCursor(m.pttTriggerValue, m.pttTriggerCursor)
	m.pttInsertCursor = m.pttTriggerCursor
	m.pttInsertValueVersion = m.pttTriggerValue
}

func (m *tuiModel) resetPTTTrigger() {
	m.resetPTTSpaceRun()
	m.pttStarting = false
	m.pttStreaming = false
	m.pttLocalSession = nil
	m.pttReleaseAt = time.Time{}
	m.pttTriggerCaptured = false
	m.pttTriggerValue = ""
	m.pttTriggerCursor = 0
	m.pttInsertCursor = 0
	m.pttInsertValueVersion = ""
	m.pttLiveText = ""
}

func (m *tuiModel) resetPTTSpaceRun() {
	if m.pttActive || m.pttBusy || m.pttStarting {
		return
	}
	m.pttSpaceRun = 0
	m.pttTriggerCaptured = false
}

func (m *tuiModel) applyDictationText(text string) {
	text = strings.TrimSpace(text)
	if m.pttTriggerCaptured {
		if text == "" {
			m.restorePTTTriggerSnapshot()
			m.pttLiveText = ""
			return
		}
		newValue, newCursor := composeDictationValue(m.pttTriggerValue, m.pttTriggerCursor, text)
		m.setInputValueAndCursor(newValue, newCursor)
		m.pttInsertCursor = newCursor
		m.pttInsertValueVersion = newValue
		m.pttLiveText = text
		return
	}
	if text == "" {
		return
	}
	newValue, newCursor := composeDictationValue(m.input.Value(), m.inputCursorPosition(), text)
	m.setInputValueAndCursor(newValue, newCursor)
	m.pttInsertCursor = newCursor
	m.pttInsertValueVersion = newValue
	m.pttLiveText = text
}

func composeDictationValue(value string, cursor int, text string) (string, int) {
	runes := []rune(value)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	prefix := ""
	suffix := ""
	if cursor > 0 {
		prev := runes[cursor-1]
		if !isSpacingRune(prev) && !startsWithPunctuation(text) {
			prefix = " "
		}
	}
	if cursor < len(runes) {
		next := runes[cursor]
		if !isSpacingRune(next) && !endsWithPunctuation(text) {
			suffix = " "
		}
	}
	insert := []rune(prefix + text + suffix)
	newValue := string(runes[:cursor]) + string(insert) + string(runes[cursor:])
	return newValue, cursor + len(insert)
}

func (m *tuiModel) insertDictationAtCursor(text string) {
	m.applyDictationText(text)
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
					extCh <- wsDisconnectedMsg{conn: conn, err: err}
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
	return tea.Tick(spinnerTickInterval, func(t time.Time) tea.Msg {
		return spinnerTickMsg(t)
	})
}

// ── undo/redo git helpers ─────────────────────────────────────────────────────

// undoResultMsg is the tea.Msg returned by async undo/redo git operations.
type undoResultMsg struct {
	text      string
	err       error
	snapshot  fileChangeSnapshot // the snapshot that was operated on
	wasUndo   bool               // true = undo, false = redo
	sessionID string
}

// gitDiffHead runs `git diff HEAD` in the given directory and returns the output.
func gitDiffHead(dir string) (string, error) {
	cmd := exec.Command("git", "diff", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git diff HEAD: %w", err)
	}
	return string(out), nil
}

// gitRevParseHead runs `git rev-parse HEAD` in the given directory.
func gitRevParseHead(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// gitDiffBetween runs `git diff <from> <to>` to capture committed changes.
func gitDiffBetween(dir, from, to string) (string, error) {
	cmd := exec.Command("git", "diff", from, to)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git diff %s %s: %w", from, to, err)
	}
	return string(out), nil
}

// gitApplyPatch applies a unified diff patch in the given directory.
// If reverse is true, it applies the patch in reverse (undo).
func gitApplyPatch(dir, patch string, reverse bool) error {
	args := []string{"apply", "--whitespace=nowarn"}
	if reverse {
		args = append(args, "--reverse")
	}
	args = append(args, "-")
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(patch)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git apply: %s (%w)", strings.TrimSpace(stderr.String()), err)
	}
	return nil
}

// parseDiffFiles extracts the list of file paths from a unified diff.
func parseDiffFiles(patch string) []string {
	var files []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "+++ b/") {
			f := strings.TrimPrefix(line, "+++ b/")
			if !seen[f] {
				seen[f] = true
				files = append(files, f)
			}
		} else if strings.HasPrefix(line, "--- a/") {
			f := strings.TrimPrefix(line, "--- a/")
			if f != "/dev/null" && !seen[f] {
				seen[f] = true
				files = append(files, f)
			}
		}
	}
	return files
}

// sessionUndoStack returns (or creates) the undo stack for a session.
func (m *tuiModel) sessionUndoStack(sessionID string) *undoStack {
	if st, ok := m.undoStacks[sessionID]; ok {
		return st
	}
	st := &undoStack{}
	m.undoStacks[sessionID] = st
	return st
}

// capturePreRunBaseline saves the current git state before a prompt run begins.
func (m *tuiModel) capturePreRunBaseline(sessionID string) {
	s, ok := m.findSessionByID(sessionID)
	if !ok || s.WorkingDir == "" {
		return
	}
	diff, _ := gitDiffHead(s.WorkingDir)
	m.preRunDiff[sessionID] = diff
	commit, _ := gitRevParseHead(s.WorkingDir)
	m.preRunCommit[sessionID] = commit
}

// capturePostRunSnapshot compares the current git state against the pre-run
// baseline and pushes any new changes onto the session's undo stack.
func (m *tuiModel) capturePostRunSnapshot(sessionID string) {
	s, ok := m.findSessionByID(sessionID)
	if !ok || s.WorkingDir == "" {
		return
	}

	// Capture committed changes (if HEAD moved during the run).
	var committedPatch string
	if preCommit, ok := m.preRunCommit[sessionID]; ok && preCommit != "" {
		curCommit, err := gitRevParseHead(s.WorkingDir)
		if err == nil && curCommit != preCommit {
			committedPatch, _ = gitDiffBetween(s.WorkingDir, preCommit, curCommit)
		}
	}

	// Capture uncommitted (working tree) changes relative to HEAD.
	postDiff, _ := gitDiffHead(s.WorkingDir)

	// Calculate the net new uncommitted changes.
	preDiff := m.preRunDiff[sessionID]
	var uncommittedPatch string
	if postDiff != preDiff {
		uncommittedPatch = postDiff
	}

	// Clean up baseline tracking.
	delete(m.preRunDiff, sessionID)
	delete(m.preRunCommit, sessionID)

	hasPatch := committedPatch != "" || uncommittedPatch != ""
	if !hasPatch {
		return
	}

	// Store the uncommitted patch for undo. If only committed changes exist,
	// store those for informational purposes.
	patch := uncommittedPatch
	if patch == "" {
		patch = committedPatch
	}

	files := parseDiffFiles(patch)
	if len(files) == 0 {
		return
	}

	desc := fmt.Sprintf("%d file(s) changed", len(files))
	if len(files) <= 3 {
		desc = strings.Join(files, ", ")
	}

	stack := m.sessionUndoStack(sessionID)
	stack.undoable = append(stack.undoable, fileChangeSnapshot{
		patch:     patch,
		files:     files,
		timestamp: time.Now(),
		message:   desc,
	})
	// Clear redo stack when new changes are made.
	stack.redoable = nil
}

// performUndo applies the top undo snapshot in reverse and moves it to redo.
func (m *tuiModel) performUndo() tea.Cmd {
	if m.activeSessionID == "" {
		m.logSystem("Nothing to undo (no active session)")
		return nil
	}
	s, ok := m.findSessionByID(m.activeSessionID)
	if !ok || s.WorkingDir == "" {
		m.logSystem("Nothing to undo (session has no working directory)")
		return nil
	}
	stack := m.sessionUndoStack(m.activeSessionID)
	if len(stack.undoable) == 0 {
		m.logSystem("Nothing to undo")
		return nil
	}

	snap := stack.undoable[len(stack.undoable)-1]
	stack.undoable = stack.undoable[:len(stack.undoable)-1]
	dir := s.WorkingDir
	sid := m.activeSessionID

	return func() tea.Msg {
		if err := gitApplyPatch(dir, snap.patch, true); err != nil {
			return undoResultMsg{err: fmt.Errorf("undo failed: %w", err), snapshot: snap, wasUndo: true, sessionID: sid}
		}
		return undoResultMsg{
			text:      fmt.Sprintf("Undid changes: %s (%d file(s))", snap.message, len(snap.files)),
			snapshot:  snap,
			wasUndo:   true,
			sessionID: sid,
		}
	}
}

// performRedo reapplies the top redo snapshot and moves it back to undo.
func (m *tuiModel) performRedo() tea.Cmd {
	if m.activeSessionID == "" {
		m.logSystem("Nothing to redo (no active session)")
		return nil
	}
	s, ok := m.findSessionByID(m.activeSessionID)
	if !ok || s.WorkingDir == "" {
		m.logSystem("Nothing to redo (session has no working directory)")
		return nil
	}
	stack := m.sessionUndoStack(m.activeSessionID)
	if len(stack.redoable) == 0 {
		m.logSystem("Nothing to redo")
		return nil
	}

	snap := stack.redoable[len(stack.redoable)-1]
	stack.redoable = stack.redoable[:len(stack.redoable)-1]
	dir := s.WorkingDir
	sid := m.activeSessionID

	return func() tea.Msg {
		if err := gitApplyPatch(dir, snap.patch, false); err != nil {
			return undoResultMsg{err: fmt.Errorf("redo failed: %w", err), snapshot: snap, wasUndo: false, sessionID: sid}
		}
		return undoResultMsg{
			text:      fmt.Sprintf("Redid changes: %s (%d file(s))", snap.message, len(snap.files)),
			snapshot:  snap,
			wasUndo:   false,
			sessionID: sid,
		}
	}
}

// ── renderMissionControl ──────────────────────────────────────────────────────

// sessionVisualLines returns the number of rendered lines a session occupies
// in the sessions list (base 4 + 1 per sub-agent when expanded).
func (m *tuiModel) sessionVisualLines(idx int) int {
	base := 4
	s := m.sessions[idx]
	if m.expandedSessions[s.ID] && len(s.SubAgents) > 0 {
		return base + len(s.SubAgents)
	}
	return base
}

func (m *tuiModel) renderMissionControl(selectedStyle, activeStyle lipgloss.Style, availH int, listW int) string {
	if len(m.sessions) == 0 {
		return styleMuted.Render("  No sessions yet\n\n") +
			styleText.Render("  Press ") + styleCyan.Render("Ctrl+N") + styleText.Render(" to start a new session\n") +
			styleText.Render("  Press ") + styleCyan.Render("?") + styleText.Render(" for help")
	}

	// Determine visible window using a line-budget approach so that expanded
	// sub-agent rows are properly accounted for.
	selectedLines := m.sessionVisualLines(m.selected)
	start := m.selected
	usedLines := selectedLines
	// Extend upward while there is budget.
	for start > 0 {
		prev := m.sessionVisualLines(start - 1)
		if usedLines+prev > availH {
			break
		}
		start--
		usedLines += prev
	}
	// Extend downward while there is budget.
	end := m.selected + 1
	for end < len(m.sessions) {
		next := m.sessionVisualLines(end)
		if usedLines+next > availH {
			break
		}
		usedLines += next
		end++
	}

	m.layoutSessionStart = start
	m.layoutSessionEnd = end

	var out []string
	for i := start; i < end; i++ {
		s := m.sessions[i]
		state := sessionStateLabel(s)
		isSelected := i == m.selected
		isHover := i == m.hoverSession && !isSelected
		isActive := s.ID == m.activeSessionID
		expanded := m.expandedSessions[s.ID]

		prefix := "  "
		if isSelected {
			prefix = "▶ "
		} else if isHover {
			prefix = "» "
		} else if isActive {
			prefix = "● "
		}

		backendModel := s.Backend
		if s.Model != "" {
			backendModel += ":" + trimForLine(s.Model, 16)
		}
		titleText := defaultString(s.Title, defaultString(s.CurrentPrompt, defaultString(s.LastMessage, "—")))
		fullPath := homeTildePath(s.WorkingDir)

		// Sub-agent count badge shown on sessions with tracked sub-agents.
		var subAgentBadge string
		if n := len(s.SubAgents); n > 0 {
			if expanded {
				subAgentBadge = " " + styleCyan.Render(fmt.Sprintf("▾%d", n))
			} else {
				subAgentBadge = " " + styleCyan.Render(fmt.Sprintf("▸%d", n))
			}
		}

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
			out = append(out, selectedStyle.Render(topLine)+subAgentBadge, selectedStyle.Render(titleLine), selectedStyle.Render(botLine), sep)
		} else if isHover {
			hoverStyle := lipgloss.NewStyle().Foreground(colText).Underline(true)
			titleLine := trimForLine("  "+titleText, max(10, listW))
			out = append(out, hoverStyle.Render(topLine)+subAgentBadge, hoverStyle.Render(titleLine), styleMuted.Render(botLine), sep)
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
			unreadBadge := ""
			if m.sessionUnread[s.ID] {
				unreadBadge = " " + styleYellow.Render("◆")
			}
			topLine := prefix + s.ID + "  " + stateTag + "  " + styleCyan.Render(trimForLine(backendModel, max(8, listW/3)))
			titleLine := styleText.Render(trimForLine("  "+titleText, max(10, listW)))
			botLine := styleMuted.Render(trimForLine("  "+fullPath, max(10, listW)))
			if isActive {
				topLine = activeStyle.Render(prefix+s.ID) + "  " + stateTag + "  " + styleCyan.Render(trimForLine(backendModel, max(8, listW/3)))
			}
			out = append(out, trimForLine(topLine, max(10, listW))+badges+subAgentBadge+unreadBadge, titleLine, botLine, sep)
		}

		// Render sub-agent rows when expanded.
		if expanded && len(s.SubAgents) > 0 {
			for j, sa := range s.SubAgents {
				connector := "├─"
				if j == len(s.SubAgents)-1 {
					connector = "└─"
				}
				var statusIcon string
				switch sa.Status {
				case "completed":
					statusIcon = styleGreen.Render("✓")
				case "failed", "error":
					statusIcon = styleRed.Render("✗")
				default:
					statusIcon = styleOrange.Render(spinnerFrames[m.spinnerFrame])
				}
				title := sa.Title
				if title == "" {
					title = sa.ToolCallID
				}
				subLine := styleMuted.Render("  "+connector+" ") + statusIcon + " " + styleText.Render(trimForLine(title, max(6, listW-12)))
				// Remove the last entry from out (the sep line) and re-append sub-agent row + sep.
				if len(out) > 0 {
					out = out[:len(out)-1] // remove previous sep
				}
				out = append(out, subLine)
				if j == len(s.SubAgents)-1 {
					out = append(out, sep) // restore sep after last sub-agent
				}
			}
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

func clamp(v, low, high int) int {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

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
	pos := m.inputCursorPosition()
	if pos <= 0 {
		m.setInputValueAndCursor(m.input.Value(), 0)
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
	m.setInputValueAndCursor(m.input.Value(), pos)
}

func (m *tuiModel) moveCursorWordRight() {
	runes := []rune(m.input.Value())
	pos := m.inputCursorPosition()
	n := len(runes)
	if pos >= n {
		m.setInputValueAndCursor(m.input.Value(), n)
		return
	}
	for pos < n && isWordBoundaryRune(runes[pos]) {
		pos++
	}
	for pos < n && !isWordBoundaryRune(runes[pos]) {
		pos++
	}
	m.setInputValueAndCursor(m.input.Value(), pos)
}

func (m *tuiModel) deleteWordBackward() {
	v := []rune(m.input.Value())
	pos := m.inputCursorPosition()
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
	m.setInputValueAndCursor(newVal, start)
}

func (m *tuiModel) deleteWordForward() {
	v := []rune(m.input.Value())
	pos := m.inputCursorPosition()
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
	m.setInputValueAndCursor(newVal, pos)
}

func (m *tuiModel) deleteToLineStart() {
	value := m.input.Value()
	pos := m.inputCursorPosition()
	row, col := inputRowCol(value, pos)
	if col <= 0 {
		return
	}
	lines := strings.Split(value, "\n")
	lineRunes := []rune(lines[row])
	lines[row] = string(lineRunes[col:])
	m.setInputValueAndCursor(strings.Join(lines, "\n"), inputAbsolutePosition(strings.Join(lines, "\n"), row, 0))
}

func (m *tuiModel) deleteToLineEnd() {
	value := m.input.Value()
	pos := m.inputCursorPosition()
	row, col := inputRowCol(value, pos)
	lines := strings.Split(value, "\n")
	lineRunes := []rune(lines[row])
	if col >= len(lineRunes) {
		return
	}
	lines[row] = string(lineRunes[:col])
	m.setInputValueAndCursor(strings.Join(lines, "\n"), inputAbsolutePosition(strings.Join(lines, "\n"), row, col))
}

func isWordBoundaryRune(r rune) bool {
	return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
}

func isSpacingRune(r rune) bool {
	return unicode.IsSpace(r)
}

func startsWithPunctuation(s string) bool {
	r := []rune(strings.TrimSpace(s))
	if len(r) == 0 {
		return false
	}
	return unicode.IsPunct(r[0]) || unicode.IsSymbol(r[0])
}

func endsWithPunctuation(s string) bool {
	r := []rune(strings.TrimSpace(s))
	if len(r) == 0 {
		return false
	}
	return unicode.IsPunct(r[len(r)-1]) || unicode.IsSymbol(r[len(r)-1])
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

func formatDurationRounded(d time.Duration) string {
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Round(10*time.Millisecond).Milliseconds())
	case d < 10*time.Second:
		return fmt.Sprintf("%.1fs", d.Round(100*time.Millisecond).Seconds())
	default:
		return formatElapsed(d)
	}
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
		// Try local whisper STT first — cross-platform, zero extra dependencies.
		if sess, err := startLocalSTTSession(m.extCh); err == nil {
			return sttStartedMsg{localSession: sess, streaming: true}
		}
		// Fall back to Darwin native (SFSpeechRecognizer) then legacy ffmpeg+whisper.
		if runtime.GOOS == "darwin" && !m.pttDisableNativeLive {
			msg := m.startDarwinStreamingPTTCapture()
			if msg.err == nil {
				return msg
			}
			legacy := m.startLegacyPTTCapture()
			legacy.disableNative = true
			legacy.disableNote = nativeDictationFallbackNote(msg.err)
			return legacy
		}
		return m.startLegacyPTTCapture()
	}
}

func (m *tuiModel) startLegacyPTTCapture() sttStartedMsg {
	if m.pttBusy || m.pttActive {
		return sttStartedMsg{err: fmt.Errorf("dictation already active")}
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return sttStartedMsg{err: fmt.Errorf("ffmpeg not found")}
	}
	audioPath := filepath.Join(os.TempDir(), fmt.Sprintf("orbitor-stt-%d.wav", time.Now().UnixNano()))
	cmd := exec.Command("ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-f", "avfoundation", "-i", ":0",
		"-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le",
		audioPath,
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return sttStartedMsg{err: err}
	}
	if err := cmd.Start(); err != nil {
		return sttStartedMsg{err: err}
	}
	return sttStartedMsg{proc: cmd, stdin: stdin, audioPath: audioPath}
}

func (m *tuiModel) startDarwinStreamingPTTCapture() sttStartedMsg {
	if m.pttBusy || m.pttActive {
		return sttStartedMsg{err: fmt.Errorf("dictation already active")}
	}
	helperPath, err := ensureDarwinSpeechHelperBinary()
	if err != nil {
		return sttStartedMsg{err: err}
	}
	cmd := exec.Command(helperPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return sttStartedMsg{err: err}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return sttStartedMsg{err: err}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return sttStartedMsg{err: err}
	}
	if err := cmd.Start(); err != nil {
		return sttStartedMsg{err: err}
	}
	// Wait for the binary to signal ready (or fail) before returning.
	// This catches permission failures synchronously so the caller can
	// fall back to legacy dictation without requiring a second space-hold.
	readyCh := make(chan error, 1)
	go m.consumeDarwinSpeechStream(cmd, stdout, stderr, readyCh)
	select {
	case startErr := <-readyCh:
		if startErr != nil {
			_ = cmd.Process.Kill()
			return sttStartedMsg{err: startErr}
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		return sttStartedMsg{err: fmt.Errorf("native dictation startup timeout")}
	}
	return sttStartedMsg{proc: cmd, stdin: stdin, streaming: true}
}

func (m *tuiModel) stopPTTCapture() tea.Cmd {
	return func() tea.Msg {
		if !m.pttActive {
			return nil
		}
		m.pttActive = false
		m.pttSpaceRun = 0

		// Local whisper STT path — stop the session in a goroutine; it will
		// send sttResultMsg via extCh when the final inference completes.
		if localSess := m.pttLocalSession; localSess != nil {
			m.pttLocalSession = nil
			go localSess.stop()
			return nil
		}

		proc := m.pttProc
		procInput := m.pttProcInput
		audioPath := m.pttAudioPath
		m.pttProc = nil
		m.pttProcInput = nil
		if m.pttStreaming {
			m.pttReleaseAt = time.Now()
			if procInput != nil {
				_, _ = io.WriteString(procInput, "stop\n")
				_ = procInput.Close()
			}
			if proc == nil {
				return sttResultMsg{err: fmt.Errorf("native dictation process missing")}
			}
			return nil
		}
		stopStart := time.Now()
		if proc != nil && proc.Process != nil {
			done := make(chan error, 1)
			go func() { done <- proc.Wait() }()
			if procInput != nil {
				_, _ = io.WriteString(procInput, "q\n")
				_ = procInput.Close()
			}
			select {
			case <-done:
			case <-time.After(350 * time.Millisecond):
				_ = proc.Process.Signal(os.Interrupt)
				select {
				case <-done:
				case <-time.After(750 * time.Millisecond):
					_ = proc.Process.Kill()
					select {
					case <-done:
					case <-time.After(250 * time.Millisecond):
					}
				}
			}
		}
		captureStopDelay := time.Since(stopStart)
		if audioPath == "" {
			return sttResultMsg{err: fmt.Errorf("no audio captured")}
		}
		transcribeStart := time.Now()
		text, err := transcribeAudioSwift(audioPath)
		return sttResultMsg{
			text:             text,
			err:              err,
			captureStopDelay: captureStopDelay,
			transcribeDelay:  time.Since(transcribeStart),
		}
	}
}

type darwinSpeechEvent struct {
	Type    string `json:"type"`
	Text    string `json:"text,omitempty"`
	Message string `json:"message,omitempty"`
}

func (m *tuiModel) consumeDarwinSpeechStream(cmd *exec.Cmd, stdout io.ReadCloser, stderr io.ReadCloser, readyCh chan<- error) {
	defer stdout.Close()
	defer stderr.Close()

	stderrDone := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(stderr)
		stderrDone <- strings.TrimSpace(string(b))
	}()

	readySignaled := false
	signalReady := func(err error) {
		if readySignaled {
			return
		}
		readySignaled = true
		readyCh <- err
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	terminalSeen := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var evt darwinSpeechEvent
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			continue
		}
		switch evt.Type {
		case "ready":
			signalReady(nil)
			continue
		case "partial":
			m.extCh <- sttPartialMsg{text: evt.Text, external: true}
		case "final":
			terminalSeen = true
			delay := time.Duration(0)
			if !m.pttReleaseAt.IsZero() {
				delay = time.Since(m.pttReleaseAt)
			}
			m.extCh <- sttResultMsg{text: evt.Text, releaseToTextDelay: delay, external: true}
		case "error":
			terminalSeen = true
			msg := strings.TrimSpace(evt.Message)
			if msg == "" {
				msg = "native dictation failed"
			}
			err := fmt.Errorf("%s", msg)
			// If ready hasn't been signaled yet, this is a startup failure
			// (e.g. permission denied). Signal the caller so it can fall back
			// to legacy dictation synchronously.
			if !readySignaled {
				signalReady(err)
				return
			}
			m.extCh <- sttResultMsg{
				err:           err,
				disableNative: true,
				disableNote:   nativeDictationFallbackNote(err),
				external:      true,
			}
		}
	}
	scanErr := scanner.Err()
	waitErr := cmd.Wait()
	stderrText := <-stderrDone
	if terminalSeen {
		return
	}
	if scanErr != nil {
		if !readySignaled {
			signalReady(scanErr)
			return
		}
		m.extCh <- sttResultMsg{err: scanErr, external: true}
		return
	}
	if waitErr != nil {
		msg := strings.TrimSpace(stderrText)
		if msg == "" {
			msg = waitErr.Error()
		}
		err := fmt.Errorf("native dictation failed: %s", msg)
		if !readySignaled {
			signalReady(err)
			return
		}
		m.extCh <- sttResultMsg{
			err:           err,
			disableNative: true,
			disableNote:   nativeDictationFallbackNote(err),
			external:      true,
		}
	}
}

func nativeDictationFallbackNote(err error) string {
	if err == nil {
		return "Native live dictation unavailable; falling back to local file transcription for this session."
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "abort trap") || strings.Contains(msg, "permission") || strings.Contains(msg, "not-determined") || strings.Contains(msg, "denied") {
		return "Speech Recognition not authorized — enable it for your terminal in System Settings > Privacy & Security > Speech Recognition, then try again. Falling back to local file transcription for this session."
	}
	return "Native live dictation unavailable; falling back to local file transcription for this session."
}

func describeProcessFailure(err error) string {
	if err == nil {
		return ""
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return err.Error()
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok {
		return err.Error()
	}
	switch {
	case status.Signaled():
		return fmt.Sprintf("%s (signal=%s, exitstatus=%d)", err.Error(), status.Signal(), status.ExitStatus())
	case status.Exited():
		return fmt.Sprintf("%s (exitstatus=%d)", err.Error(), status.ExitStatus())
	default:
		return err.Error()
	}
}

func ensureDarwinSpeechHelperBinary() (string, error) {
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("native streaming dictation is only available on macOS")
	}
	swiftcPath, err := exec.LookPath("swiftc")
	if err != nil {
		return "", fmt.Errorf("swiftc not found")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".orbitor", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	sourcePath := filepath.Join(dir, "orbitor-native-dictation.swift")
	binaryPath := filepath.Join(dir, "orbitor-native-dictation")

	compile := false
	if existing, err := os.ReadFile(sourcePath); err != nil || string(existing) != darwinSpeechHelperSource {
		if err := os.WriteFile(sourcePath, []byte(darwinSpeechHelperSource), 0o644); err != nil {
			return "", err
		}
		compile = true
	}
	if helperStat, err := os.Stat(binaryPath); err != nil {
		compile = true
	} else if selfPath, selfErr := os.Executable(); selfErr == nil {
		// Re-compile if the orbitor binary is newer than the cached helper — this
		// ensures a Homebrew update triggers a fresh compile+codesign so that macOS
		// TCC associates the new binary identity with the helper.
		if selfStat, selfErr := os.Stat(selfPath); selfErr == nil {
			if selfStat.ModTime().After(helperStat.ModTime()) {
				compile = true
			}
		}
	}
	if !compile {
		return binaryPath, nil
	}

	plistPath := filepath.Join(dir, "orbitor-native-dictation-Info.plist")
	const plistContent = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleIdentifier</key>
	<string>com.orbitor.native-dictation</string>
	<key>CFBundleName</key>
	<string>orbitor-native-dictation</string>
	<key>NSMicrophoneUsageDescription</key>
	<string>Orbitor uses the microphone for voice dictation.</string>
	<key>NSSpeechRecognitionUsageDescription</key>
	<string>Orbitor uses speech recognition to transcribe your voice input.</string>
</dict>
</plist>
`
	if err := os.WriteFile(plistPath, []byte(plistContent), 0o644); err != nil {
		return "", err
	}

	tmpPath := binaryPath + ".tmp"
	cmd := exec.Command(swiftcPath, sourcePath,
		"-Xlinker", "-sectcreate",
		"-Xlinker", "__TEXT",
		"-Xlinker", "__info_plist",
		"-Xlinker", plistPath,
		"-o", tmpPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("compile native dictation helper failed: %s", msg)
	}
	// swiftc embeds the output filename as the code signing identifier, so the .tmp suffix
	// ends up baked in. Re-sign with the correct identifier before rename so that macOS TCC
	// can track microphone/speech permissions under a stable identity.
	if codesignPath, csErr := exec.LookPath("codesign"); csErr == nil {
		_ = exec.Command(codesignPath, "--sign", "-", "--force", "--identifier", "orbitor-native-dictation", tmpPath).Run()
	}
	if err := os.Rename(tmpPath, binaryPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	return binaryPath, nil
}

const darwinSpeechHelperSource = `import Foundation
import Speech
import AVFoundation

struct Event: Encodable {
    let type: String
    let text: String?
    let message: String?
}

func emit(type: String, text: String? = nil, message: String? = nil) {
    let event = Event(type: type, text: text, message: message)
    guard let data = try? JSONEncoder().encode(event),
          let line = String(data: data, encoding: .utf8) else { return }
    FileHandle.standardOutput.write(Data(line.utf8))
    FileHandle.standardOutput.write(Data([0x0a]))
}

func authLabel(_ status: SFSpeechRecognizerAuthorizationStatus) -> String {
    switch status {
    case .authorized: return "authorized"
    case .denied: return "denied"
    case .restricted: return "restricted"
    case .notDetermined: return "not-determined"
    @unknown default: return "unknown"
    }
}

final class DictationDriver {
    private let audioEngine = AVAudioEngine()
    private let recognizer = SFSpeechRecognizer(locale: Locale(identifier: "en-US"))
    private var request: SFSpeechAudioBufferRecognitionRequest?
    private var task: SFSpeechRecognitionTask?
    private var stopRequested = false
    private var terminalSent = false
    // Text committed from segments that auto-finalized due to long pauses.
    private var committedText = ""
    // Text from the current in-progress recognition segment.
    private var segmentText = ""

    private func combinedText() -> String {
        if committedText.isEmpty { return segmentText }
        if segmentText.isEmpty { return committedText }
        return committedText + " " + segmentText
    }

    func run() {
        DispatchQueue.global(qos: .userInitiated).async {
            while let line = readLine(strippingNewline: true) {
                if line.lowercased().contains("stop") {
                    DispatchQueue.main.async {
                        self.stop()
                    }
                    break
                }
            }
        }

        DispatchQueue.main.async {
            let speechStatus = SFSpeechRecognizer.authorizationStatus()
            switch speechStatus {
            case .authorized:
                self.startRecognition()
            case .notDetermined:
                // requestAuthorization crashes with SIGABRT on macOS 14+ when called from
                // a subprocess context. Fail cleanly with instructions instead.
                self.fail("speech-recognition-not-determined")
            default:
                self.fail("speech-recognition-" + authLabel(speechStatus))
            }
        }

        RunLoop.main.run()
    }

    private func startRecognition() {
        guard let recognizer = recognizer, recognizer.isAvailable else {
            fail("speech recognizer unavailable")
            return
        }

        let inputNode = audioEngine.inputNode
        inputNode.removeTap(onBus: 0)
        let format = inputNode.outputFormat(forBus: 0)
        inputNode.installTap(onBus: 0, bufferSize: 1024, format: format) { [weak self] buffer, _ in
            self?.request?.append(buffer)
        }

        audioEngine.prepare()
        do {
            try audioEngine.start()
        } catch {
            fail("audio engine start failed: " + error.localizedDescription)
            return
        }

        emit(type: "ready")
        startNewRecognitionTask()
    }

    // Creates a fresh recognition task on a new request. The audio tap writes into
    // self.request, so updating that pointer redirects audio without reinstalling the tap.
    private func startNewRecognitionTask() {
        guard !stopRequested, !terminalSent, let recognizer = recognizer else { return }

        task?.cancel()
        segmentText = ""

        let req = SFSpeechAudioBufferRecognitionRequest()
        req.shouldReportPartialResults = true
        if #available(macOS 10.15, *) {
            req.requiresOnDeviceRecognition = recognizer.supportsOnDeviceRecognition
        }
        req.taskHint = .dictation
        if #available(macOS 13.0, *) {
            req.addsPunctuation = true
        }
        request = req

        task = recognizer.recognitionTask(with: req) { [weak self] result, error in
            guard let self = self else { return }

            if let result = result {
                let text = result.bestTranscription.formattedString.trimmingCharacters(in: .whitespacesAndNewlines)
                self.segmentText = text
                let combined = self.combinedText()

                if result.isFinal {
                    if self.stopRequested {
                        self.finish(combined)
                        return
                    }
                    // Auto-finalized due to long pause: commit and restart so subsequent
                    // speech appends rather than replaces the accumulated text.
                    if !text.isEmpty {
                        self.committedText = combined
                    }
                    self.startNewRecognitionTask()
                    return
                }

                if !combined.isEmpty {
                    emit(type: "partial", text: combined)
                }
            }

            if let error = error {
                if self.stopRequested {
                    self.finish(self.combinedText())
                } else {
                    self.fail(error.localizedDescription)
                }
            }
        }
    }

    private func stop() {
        guard !stopRequested else { return }
        stopRequested = true
        request?.endAudio()
        audioEngine.inputNode.removeTap(onBus: 0)
        audioEngine.stop()

        DispatchQueue.main.asyncAfter(deadline: .now() + 1.0) {
            if !self.terminalSent {
                self.finish(self.combinedText())
            }
        }
    }

    private func finish(_ text: String) {
        guard !terminalSent else { return }
        terminalSent = true
        emit(type: "final", text: text)
        cleanup()
        exit(0)
    }

    private func fail(_ message: String) {
        guard !terminalSent else { return }
        terminalSent = true
        emit(type: "error", message: message)
        cleanup()
        exit(1)
    }

    private func cleanup() {
        task?.cancel()
        request = nil
        audioEngine.stop()
        audioEngine.inputNode.removeTap(onBus: 0)
    }
}

DictationDriver().run()
`

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
	txtPaths := []string{
		outBase + ".txt",
		path + ".txt",
		filepath.Join(filepath.Dir(outBase), filepath.Base(path)+".txt"),
	}
	var lastErr error
	for _, txtPath := range txtPaths {
		if txtPath == "" {
			continue
		}
		b, err := os.ReadFile(txtPath)
		if err != nil {
			lastErr = err
			continue
		}
		defer os.Remove(txtPath)
		return strings.TrimSpace(string(b)), nil
	}
	if lastErr == nil {
		lastErr = os.ErrNotExist
	}
	return "", fmt.Errorf("dictation output missing: %w", lastErr)
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
	m.setInputValueAndCursor(m.input.Value(), len([]rune(m.input.Value())))
	return true
}

func (m *tuiModel) updatePlaceholder() {
	if len(m.sessions) == 0 {
		m.input.Placeholder = "Press Ctrl+N to create a new session..."
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
			m.input.Placeholder = "Session needs permission — press any key to open approval dialog"
		default:
			m.input.Placeholder = "Type a prompt and press Enter..."
		}
		return
	}
	m.input.Placeholder = "Type a prompt and press Enter..."
}

// ── file picker methods ───────────────────────────────────────────────────────

// filePickerSync detects @ mentions at the cursor and activates/deactivates
// the file picker accordingly. Returns a tea.Cmd if files need to be loaded.
func (m *tuiModel) filePickerSync() tea.Cmd {
	cursorPos := m.inputCursorPosition()
	active, query, atPos := filePickerDetect(m.input.Value(), cursorPos)

	if !active {
		m.picker.active = false
		return nil
	}

	// Find the working directory of the active or selected session.
	var workingDir, sessionID string
	if m.activeSessionID != "" {
		if s, ok := m.findSessionByID(m.activeSessionID); ok {
			workingDir = s.WorkingDir
			sessionID = s.ID
		}
	}
	if workingDir == "" && len(m.sessions) > 0 && m.selected < len(m.sessions) {
		s := m.sessions[m.selected]
		workingDir = s.WorkingDir
		sessionID = s.ID
	}
	if workingDir == "" {
		m.picker.active = false
		return nil
	}

	m.picker.active = true
	m.picker.query = query
	m.picker.atPosition = atPos

	// If files are not loaded or session changed, trigger async load.
	var cmd tea.Cmd
	if m.picker.sessionID != sessionID || m.picker.workingDir != workingDir || len(m.picker.files) == 0 {
		if !m.picker.loading {
			m.picker.sessionID = sessionID
			m.picker.workingDir = workingDir
			m.picker.loading = true
			cmd = filePickerLoadCmd(sessionID, workingDir)
		}
	}

	// Update filtered results.
	m.picker.filtered = filePickerFuzzyMatch(m.picker.files, m.picker.query, m.picker.maxVisible)
	if m.picker.selected >= len(m.picker.filtered) {
		m.picker.selected = max(0, len(m.picker.filtered)-1)
	}

	// If no results and no files loading, dismiss.
	if len(m.picker.filtered) == 0 && !m.picker.loading && len(m.picker.files) > 0 {
		// Keep active but show "no matches" — don't dismiss yet.
	}

	return cmd
}

// filePickerComplete replaces the @query text in the textarea with the
// selected file path.
func (m *tuiModel) filePickerComplete() {
	if !m.picker.active || len(m.picker.filtered) == 0 {
		return
	}
	selected := m.picker.filtered[m.picker.selected]
	runes := []rune(m.input.Value())
	atPos := m.picker.atPosition
	cursorPos := m.inputCursorPosition()

	// Build the new value: everything before @, then @path, then everything after cursor.
	var newVal string
	if atPos > 0 {
		newVal = string(runes[:atPos])
	}
	insertion := "@" + selected
	newVal += insertion
	if cursorPos < len(runes) {
		newVal += string(runes[cursorPos:])
	}

	newCursor := atPos + len([]rune(insertion))
	// Add a trailing space if there isn't one already.
	if newCursor >= len([]rune(newVal)) || !unicode.IsSpace([]rune(newVal)[newCursor]) {
		newVal = string([]rune(newVal)[:newCursor]) + " " + string([]rune(newVal)[newCursor:])
		newCursor++
	}

	m.setInputValueAndCursor(newVal, newCursor)
	m.picker.active = false
}

// renderFilePicker renders the file picker popup as a string block.
// Returns empty string if the picker is not active or has no results.
func (m *tuiModel) renderFilePicker(maxWidth int) string {
	if !m.picker.active {
		return ""
	}
	if len(m.picker.filtered) == 0 && !m.picker.loading {
		return ""
	}

	contentW := max(20, min(maxWidth-4, 60))

	var lines []string
	if m.picker.loading && len(m.picker.filtered) == 0 {
		lines = append(lines, styleMuted.Render("  loading files..."))
	}
	for i, f := range m.picker.filtered {
		// Truncate long paths.
		display := f
		if len(display) > contentW-4 {
			display = "..." + display[len(display)-(contentW-7):]
		}

		if i == m.picker.selected {
			line := lipgloss.NewStyle().
				Foreground(lipgloss.Color("255")).
				Background(colSelBg).
				Bold(true).
				Render("  " + display + "  ")
			// Pad to contentW.
			padded := line + lipgloss.NewStyle().Background(colSelBg).Render(strings.Repeat(" ", max(0, contentW-lipgloss.Width(line))))
			lines = append(lines, padded)
		} else {
			// Highlight matching characters in the filename.
			highlighted := filePickerHighlight(display, m.picker.query)
			lines = append(lines, "  "+highlighted)
		}
	}
	if len(m.picker.files) > len(m.picker.filtered) && len(m.picker.filtered) > 0 {
		more := len(m.picker.files) - len(m.picker.filtered)
		if m.picker.query != "" {
			// The actual number of total matches may be unknown for subsequence,
			// but we can indicate there are more files.
			lines = append(lines, styleMuted.Render(fmt.Sprintf("  … %d+ more files", more)))
		}
	}

	if len(lines) == 0 {
		return ""
	}

	header := styleAccent.Render("  @ files") + styleMuted.Render("  ↑↓ select  tab/enter complete  esc dismiss")
	body := header + "\n" + strings.Join(lines, "\n")

	popup := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colAccent).
		Background(colPanel).
		Foreground(colText).
		Padding(0, 0).
		Width(contentW).
		Render(body)

	return popup
}

// filePickerHighlight renders a file path with matching characters highlighted.
func filePickerHighlight(path, query string) string {
	if query == "" {
		return styleText.Render(path)
	}
	queryLower := strings.ToLower(query)
	pathRunes := []rune(path)
	queryRunes := []rune(queryLower)
	qi := 0
	var result strings.Builder
	for _, r := range pathRunes {
		if qi < len(queryRunes) && unicode.ToLower(r) == queryRunes[qi] {
			result.WriteString(styleAccent.Render(string(r)))
			qi++
		} else {
			result.WriteString(styleText.Render(string(r)))
		}
	}
	return result.String()
}
