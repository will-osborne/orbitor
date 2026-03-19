package main

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── zoo tick ──────────────────────────────────────────────────────────────────

type zooTickMsg time.Time

func zooTickCmd() tea.Cmd {
	return tea.Tick(110*time.Millisecond, func(t time.Time) tea.Msg {
		return zooTickMsg(t)
	})
}

// ── bot ───────────────────────────────────────────────────────────────────────

type zooBot struct {
	sessionID  string
	x, y       float64 // top-left position on canvas (in chars)
	dx, dy     float64 // velocity per tick
	ticks      int     // local tick counter (drives per-bot animations)
	pauseTicks int     // remaining pause ticks (standing still)
	greetTicks int     // remaining greeting ticks (waving at nearby bot)
	behavior   string
}

const (
	zooBotW = 7 // chars wide per bot
	zooBotH = 3 // rows: face, body, legs (plus 1 label row below)
)

// ── canvas ────────────────────────────────────────────────────────────────────

type zooCanvas struct {
	cells  [][]rune
	colors [][]string // "" = muted default; otherwise lipgloss xterm-256 color index
	w, h   int
}

func newZooCanvas(w, h int) *zooCanvas {
	cells := make([][]rune, h)
	colors := make([][]string, h)
	for i := range cells {
		cells[i] = []rune(strings.Repeat(" ", w))
		colors[i] = make([]string, w)
	}
	return &zooCanvas{cells: cells, colors: colors, w: w, h: h}
}

// write writes string s at position (x, y) with the given xterm-256 color.
// Out-of-bounds cells are silently skipped.
func (c *zooCanvas) write(x, y int, s string, color string) {
	if y < 0 || y >= c.h {
		return
	}
	for i, r := range []rune(s) {
		cx := x + i
		if cx < 0 || cx >= c.w {
			continue
		}
		c.cells[y][cx] = r
		c.colors[y][cx] = color
	}
}

// render converts the canvas to an ANSI-styled string by grouping consecutive
// same-coloured cells into lipgloss Render calls.
func (c *zooCanvas) render() string {
	cache := map[string]lipgloss.Style{}
	style := func(col string) lipgloss.Style {
		if s, ok := cache[col]; ok {
			return s
		}
		s := lipgloss.NewStyle().Foreground(lipgloss.Color(col)).Bold(
			col != "" && col != "237" && col != "244" && col != "240",
		)
		cache[col] = s
		return s
	}

	var sb strings.Builder
	for y, row := range c.cells {
		if y > 0 {
			sb.WriteByte('\n')
		}
		i := 0
		for i < len(row) {
			col := c.colors[y][i]
			j := i + 1
			for j < len(row) && c.colors[y][j] == col {
				j++
			}
			seg := string(row[i:j])
			if col == "" {
				sb.WriteString(styleMuted.Render(seg))
			} else {
				sb.WriteString(style(col).Render(seg))
			}
			i = j
		}
	}
	return sb.String()
}

// ── bot appearance (all strings are exactly zooBotW=7 chars wide) ─────────────

func zooBotFace(state string, ticks int, greeting bool) string {
	if greeting && (ticks/5)%2 == 0 {
		return " (^o^) "
	}
	blink := (ticks/12)%7 == 0
	switch state {
	case "working":
		f := []string{"(>_<)", "(>_-)", "(-_<)", "(>_<)"}
		return " " + f[(ticks/4)%len(f)] + " "
	case "waiting-input":
		if blink {
			return " (O_O) "
		}
		return " (!_!) "
	case "error":
		return " (x_x) "
	case "starting":
		f := []string{"(o_o)", "(._o)", "(o_.)"}
		return " " + f[(ticks/5)%len(f)] + " "
	case "offline":
		return "  ---  "
	default: // idle
		if blink {
			return " (-_-) "
		}
		return " (^_^) "
	}
}

func zooBotBody(state string, ticks int) string {
	switch state {
	case "working":
		f := []string{"[>--]", "[->-]", "[-->]", "[>-<]"}
		return " " + f[(ticks/3)%len(f)] + " "
	case "waiting-input":
		if (ticks/6)%2 == 0 {
			return " [!?!] "
		}
		return " [ ? ] "
	case "error":
		return " [ERR] "
	case "starting":
		f := []string{"[.  ]", "[.. ]", "[...]", "[ ..]", "[  .]"}
		return " " + f[(ticks/4)%len(f)] + " "
	case "offline":
		return " [   ] "
	default:
		return " [___] "
	}
}

func zooBotLegs(state string, moving bool, ticks int) string {
	if state == "offline" {
		return "       "
	}
	if state == "error" {
		return "  v_v  "
	}
	if !moving {
		return "  | |  "
	}
	f := []string{"  | |  ", " /| |  ", "  |_|  ", "  | |\\ "}
	return f[(ticks/3)%len(f)]
}

func zooBotColor(state string) string {
	switch state {
	case "working":
		return "214" // orange
	case "waiting-input":
		return "220" // yellow
	case "error":
		return "196" // red
	case "starting":
		return "39" // cyan
	case "offline":
		return "237" // dark gray
	default: // idle
		return "42" // green
	}
}

// ── bot lifecycle ─────────────────────────────────────────────────────────────

func newZooBot(sessionID string, canvasW, canvasH int) zooBot {
	maxX := float64(max(1, canvasW-zooBotW))
	maxY := float64(max(1, canvasH-zooBotH-2)) // -2: label row + floor row
	angle := rand.Float64() * 2 * math.Pi
	speed := 0.25 + rand.Float64()*0.35
	return zooBot{
		sessionID: sessionID,
		x:         rand.Float64() * maxX,
		y:         1 + rand.Float64()*(maxY-1),
		dx:        math.Cos(angle) * speed,
		dy:        math.Sin(angle) * speed * 0.4,
		behavior:  "wander",
	}
}

// syncZooBots reconciles the zoo bot slice with the current session list.
// Existing bots keep their position/animation state; new sessions get fresh bots.
// The returned slice is in the same order as sessions.
func syncZooBots(bots []zooBot, sessions []WSSessionInfo, canvasW, canvasH int) []zooBot {
	existing := make(map[string]zooBot, len(bots))
	for _, b := range bots {
		existing[b.sessionID] = b
	}
	out := make([]zooBot, 0, len(sessions))
	for _, s := range sessions {
		if b, ok := existing[s.ID]; ok {
			out = append(out, b)
		} else {
			out = append(out, newZooBot(s.ID, canvasW, canvasH))
		}
	}
	return out
}

// updateZooBots advances one animation tick for all bots.
func updateZooBots(bots []zooBot, sessions []WSSessionInfo, canvasW, canvasH int) []zooBot {
	maxX := float64(max(0, canvasW-zooBotW))
	maxY := float64(max(0, canvasH-zooBotH-2))

	stateFor := make(map[string]string, len(sessions))
	for _, s := range sessions {
		stateFor[s.ID] = sessionStateLabel(s)
	}

	for i := range bots {
		b := &bots[i]
		b.ticks++

		if b.greetTicks > 0 {
			b.greetTicks--
			b.behavior = "greet"
			continue
		}
		if b.pauseTicks > 0 {
			b.pauseTicks--
			b.behavior = "pause"
			continue
		}

		state := stateFor[b.sessionID]
		var speed float64
		switch state {
		case "working":
			speed = 1.4
			b.behavior = "dash"
		case "waiting-input":
			speed = 0.15
			b.behavior = "sit"
		case "offline", "error":
			speed = 0
			b.behavior = "pause"
		default: // idle, starting
			speed = 1.0
			b.behavior = "wander"
		}

		if speed > 0 {
			b.x += b.dx * speed
			b.y += b.dy * speed
			if b.x < 0 {
				b.x = 0
				b.dx = math.Abs(b.dx)
			} else if b.x > maxX {
				b.x = maxX
				b.dx = -math.Abs(b.dx)
			}
			if b.y < 1 {
				b.y = 1
				b.dy = math.Abs(b.dy)
			} else if b.y > maxY {
				b.y = maxY
				b.dy = -math.Abs(b.dy)
			}
		}

		// Random direction change.
		if rand.Intn(90) == 0 {
			angle := rand.Float64() * 2 * math.Pi
			sp := 0.25 + rand.Float64()*0.35
			b.dx = math.Cos(angle) * sp
			b.dy = math.Sin(angle) * sp * 0.4
		}
		// Random pause.
		if rand.Intn(70) == 0 {
			b.pauseTicks = 8 + rand.Intn(25)
		}
		if rand.Intn(100) == 0 && state != "waiting-input" && state != "error" && state != "offline" {
			if rand.Intn(2) == 0 {
				b.behavior = "zigzag"
				b.dy = (rand.Float64() - 0.5) * 0.9
			} else {
				b.behavior = "stretch"
			}
		}
	}

	// Proximity greetings between pairs of bots.
	for i := range bots {
		if bots[i].greetTicks > 0 || bots[i].pauseTicks > 0 {
			continue
		}
		for j := i + 1; j < len(bots); j++ {
			if bots[j].greetTicks > 0 || bots[j].pauseTicks > 0 {
				continue
			}
			ddx := bots[i].x - bots[j].x
			ddy := bots[i].y - bots[j].y
			if math.Sqrt(ddx*ddx+ddy*ddy) < float64(zooBotW+1) {
				dur := 18 + rand.Intn(14)
				bots[i].greetTicks = dur
				bots[j].greetTicks = dur
			}
		}
	}

	return bots
}

// ── thought bubbles ───────────────────────────────────────────────────────────

func zooThoughtColor(state, behavior string) string {
	if behavior == "greet" {
		return "42"
	}
	switch state {
	case "working":
		return "42"
	case "waiting-input":
		return "220"
	case "error":
		return "198"
	case "starting":
		return "39"
	default:
		return "244"
	}
}

func zooTrimThought(s string, maxRunes int) string {
	s = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\r", " "))
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	if maxRunes <= 3 {
		return string(r[:maxRunes])
	}
	return string(r[:maxRunes-3]) + "..."
}

// zooThoughtLines returns a label line and optional detail line for the thought bubble.
// detail may be empty for single-line bubbles.
func zooThoughtLines(s WSSessionInfo, ticks int) (label, detail string) {
	switch sessionStateLabel(s) {
	case "working":
		if tool := strings.TrimSpace(s.CurrentTool); tool != "" {
			return "tool:", zooTrimThought(tool, 42)
		}
		if prompt := strings.TrimSpace(s.CurrentPrompt); prompt != "" {
			return "task:", zooTrimThought(prompt, 42)
		}
		thoughts := []string{"working hard...", "on it!", "processing...", "deep in thought..."}
		return thoughts[ticks/35%len(thoughts)], ""
	case "waiting-input":
		if tool := strings.TrimSpace(s.CurrentTool); tool != "" {
			return "need permission:", zooTrimThought(tool, 34)
		}
		return "need your input!", ""
	case "starting":
		frames := []string{"booting .  ", "booting .. ", "booting ..."}
		return frames[ticks/8%len(frames)], ""
	case "error":
		if msg := strings.TrimSpace(s.LastMessage); msg != "" {
			return "error:", zooTrimThought(msg, 42)
		}
		return "something went wrong", ""
	default:
		idle := []string{
			"ready for action",
			"standing by...",
			"taking a breather",
			"idle thoughts...",
			"waiting for a task",
			"watching the stars",
			"daydreaming...",
			"all systems nominal",
			"what's next?",
		}
		hash := 0
		for _, r := range s.ID {
			hash += int(r)
		}
		return idle[(hash+(ticks/50))%len(idle)], ""
	}
}

// drawZooThoughtBubble draws a 1- or 2-line thought bubble above the bot at (bx, by).
// label is always shown; detail adds a second content line when non-empty.
// Returns false if there is not enough vertical room or the bubble would be degenerate.
func drawZooThoughtBubble(c *zooCanvas, bx, by, maxWidth int, label, detail, color string) bool {
	if maxWidth < 10 {
		return false
	}

	innerMax := max(6, maxWidth-4)
	label = zooTrimThought(label, innerMax)
	if detail != "" {
		detail = zooTrimThought(detail, innerMax)
	}

	var contentLines []string
	if detail != "" {
		contentLines = []string{label, detail}
	} else {
		contentLines = []string{label}
	}

	innerW := 0
	for _, l := range contentLines {
		if w := len([]rune(l)); w > innerW {
			innerW = w
		}
	}
	bubbleW := innerW + 4 // "( " + content + " )"
	nLines := len(contentLines)

	// Rows consumed above the bot: top border + nLines content rows + tail = nLines+2
	if by < nLines+2 {
		return false
	}

	// Horizontal: centre over the bot, clamp to canvas.
	x := bx - (bubbleW-zooBotW)/2
	if x+bubbleW > c.w {
		x = c.w - bubbleW
	}
	if x < 0 {
		x = 0
	}

	topY := by - nLines - 2

	c.write(x, topY, "."+strings.Repeat("-", bubbleW-2)+".", color)
	for i, line := range contentLines {
		padded := line + strings.Repeat(" ", innerW-len([]rune(line)))
		c.write(x, topY+1+i, "( "+padded+" )", color)
	}
	tailX := clamp(x+bubbleW/2, x+1, x+bubbleW-2)
	c.write(tailX, topY+1+nLines, "o", color)
	return true
}

// ── updateZoo ─────────────────────────────────────────────────────────────────

// updateZoo handles key events while the zoo view is active.
func (m *tuiModel) updateZoo(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.closeConn()
		return m, tea.Quit
	case "z", "esc", "q":
		m.showZoo = false
		return m, nil
	case "up", "k":
		if m.selected > 0 {
			m.selected--
		}
		return m, nil
	case "down", "j":
		if m.selected < len(m.sessions)-1 {
			m.selected++
		}
		return m, nil
	case "tab":
		if len(m.sessions) > 0 {
			m.selected = (m.selected + 1) % len(m.sessions)
		}
		return m, nil
	case "shift+tab":
		if len(m.sessions) > 0 {
			m.selected = (m.selected + len(m.sessions) - 1) % len(m.sessions)
		}
		return m, nil
	case "enter":
		if len(m.sessions) > 0 {
			m.showZoo = false
			return m, attachSessionCmd(m.api, m.sessions[m.selected].ID, m.extCh)
		}
		return m, nil
	case "n":
		m.showZoo = false
		m.openWizard()
		return m, nil
	case "?":
		m.showZoo = false
		m.showHelp = true
		return m, nil
	case "ctrl+r", "f5":
		return m, refreshSessionsCmd(m.api)
	case "ctrl+i", "ctrl+.", "ctrl+\\":
		if m.activeSessionID != "" {
			return m, sendWSCmd(m, map[string]any{"type": "interrupt"})
		}
		return m, nil
	}
	return m, nil
}

// ── renderZoo ─────────────────────────────────────────────────────────────────

func (m *tuiModel) renderZoo() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	panelStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")).
		Padding(0, 1)

	// Count states for header pills.
	working, waiting, idle, errored := 0, 0, 0, 0
	for _, s := range m.sessions {
		switch sessionStateLabel(s) {
		case "working":
			working++
		case "waiting-input":
			waiting++
		case "idle":
			idle++
		case "error":
			errored++
		}
	}
	pills := fmt.Sprintf(
		"%s idle:%d  %s working:%d  %s waiting:%d",
		styleGreen.Render("●"), idle,
		styleOrange.Render("●"), working,
		styleYellow.Render("!"), waiting,
	)
	if errored > 0 {
		pills += fmt.Sprintf("  %s error:%d", styleRed.Render("x"), errored)
	}

	header := panelStyle.Width(m.width - 2).Height(2).Render(
		titleStyle.Render(" AI Zoo") + " " + styleMuted.Render("z=exit  up/down=select  enter=connect  ctrl+./ctrl+i=abort  n=new  ?=help") + "\n" + pills,
	)
	headerH := lipgloss.Height(header)

	statusLeft := fmt.Sprintf("  server: %s  ·  sessions: %d  ·  z=zoo  ctrl+c=quit", m.api.baseURL, len(m.sessions))
	statusRight := time.Now().Format("15:04:05  ")
	statusPad := m.width - lipgloss.Width(statusLeft) - lipgloss.Width(statusRight)
	if statusPad < 0 {
		statusPad = 0
	}
	statusBar := styleMuted.Render(statusLeft + strings.Repeat(" ", statusPad) + statusRight)
	statusH := 1

	canvasH := max(8, m.height-headerH-statusH)
	canvasW := max(20, m.width)

	// Sync bots with current sessions (preserves positions for existing bots).
	m.zooBots = syncZooBots(m.zooBots, m.sessions, canvasW, canvasH)

	c := newZooCanvas(canvasW, canvasH)

	// ── sky ──────────────────────────────────────────────────────────────────
	// Twinkling stars: each cycles through glyphs at its own phase.
	starGlyphs := []string{"·", "⋆", "✦", "⋆", "*", "⋆", "·", "·"}
	nStars := max(12, canvasW/7)
	for i := 0; i < nStars; i++ {
		sx := (i*17 + 3) % max(1, canvasW)
		sy := 1 + (i*5)%max(1, canvasH/3)
		phase := (m.spinnerFrame/4 + i*3) % len(starGlyphs)
		starColor := "63"
		if i%7 == 0 {
			starColor = "147"
		} else if i%11 == 0 {
			starColor = "69"
		}
		c.write(sx, sy, starGlyphs[phase], starColor)
	}

	// Moon: pulses between glyphs.
	moonX := max(0, canvasW-12)
	moonFaces := []string{"(  )", "(· )", "( ·)", "(··)"}
	if canvasW > 12 && canvasH > 4 {
		c.write(moonX, 1, moonFaces[(m.spinnerFrame/25)%len(moonFaces)], "220")
		c.write(moonX, 2, "(__)", "220")
	}

	// Shooting star / comet (briefly streaks every ~350 ticks).
	cometCycle := m.spinnerFrame % 350
	if cometCycle < 25 && canvasW > 20 && canvasH > 5 {
		cometX := (cometCycle * canvasW) / 30
		cometY := 1 + cometCycle/12
		c.write(cometX, cometY, "━", "255")
		if cometX > 1 {
			c.write(cometX-1, cometY, "⋆", "244")
		}
		if cometX > 2 {
			c.write(cometX-2, cometY, "·", "240")
		}
	}

	// Drifting cloud.
	if canvasW > 14 && canvasH > 5 {
		cloudX := (canvasW + 8 - (m.spinnerFrame/22)%(canvasW+10)) % (canvasW + 10)
		cloudY := max(1, canvasH/5)
		if cloudX < canvasW-6 {
			c.write(cloudX, cloudY, "( ~~~ )", "245")
			c.write(cloudX+1, cloudY+1, "~~~~~~", "240")
		}
	}

	// ── ground ───────────────────────────────────────────────────────────────
	// Background dot texture.
	for row := 1; row < canvasH-1; row++ {
		offset := (row % 2) * 4
		for col := offset; col < canvasW-1; col += 9 {
			c.write(col, row, "·", "237")
		}
	}
	// Floor line.
	for col := 0; col < canvasW; col++ {
		c.write(col, canvasH-1, "─", "237")
	}
	// Animated grass tufts.
	grassStep := max(7, canvasW/14)
	for gx := 2; gx < canvasW-2; gx += grassStep {
		grassPhase := (m.spinnerFrame/18 + gx) % 3
		grassChars := []string{"⌒⌒", "⌒~", "~⌒"}
		c.write(gx, canvasH-2, grassChars[grassPhase], "34")
	}

	// Server racks with alternating blink indicators.
	if canvasW > 22 && canvasH > 8 {
		blinkA, blinkB := "#", "*"
		if (m.spinnerFrame/10)%2 == 0 {
			blinkA, blinkB = blinkB, blinkA
		}
		c.write(1, canvasH-8, ".---.", "61")
		c.write(1, canvasH-7, "|###|", "61")
		c.write(1, canvasH-6, "|["+blinkA+"]|", "61")
		c.write(1, canvasH-5, "|   |", "61")
		c.write(1, canvasH-4, "`---'", "61")
		c.write(canvasW-6, canvasH-8, ".---.", "61")
		c.write(canvasW-6, canvasH-7, "|###|", "61")
		c.write(canvasW-6, canvasH-6, "|["+blinkB+"]|", "61")
		c.write(canvasW-6, canvasH-5, "|   |", "61")
		c.write(canvasW-6, canvasH-4, "`---'", "61")
	}

	// Centre tree.
	if canvasW > 20 && canvasH > 7 {
		tx := canvasW/2 - 1
		treeTops := []string{" /\\ ", " ^^ "}
		c.write(tx-1, canvasH-6, treeTops[(m.spinnerFrame/30)%2], "34")
		c.write(tx-1, canvasH-5, "/  \\", "34")
		c.write(tx, canvasH-4, "||", "34")
		c.write(tx, canvasH-3, "||", "34")
	}

	// Empty-state message.
	if len(m.zooBots) == 0 {
		msg := "No sessions — press n to create one"
		msgX := max(0, (canvasW-len(msg))/2)
		c.write(msgX, canvasH/2, msg, "244")
	}

	// Build state lookup.
	stateFor := make(map[string]string, len(m.sessions))
	for _, s := range m.sessions {
		stateFor[s.ID] = sessionStateLabel(s)
	}

	maxBubbleW := min(50, max(16, canvasW*2/5))

	// Draw each bot. zooBots is in the same order as m.sessions so botIdx == session index.
	for botIdx, bot := range m.zooBots {
		if botIdx >= len(m.sessions) {
			continue
		}
		session := m.sessions[botIdx]
		bx := int(bot.x)
		by := int(bot.y)
		// Clamp to safe canvas area.
		if bx < 0 {
			bx = 0
		}
		if bx > canvasW-zooBotW {
			bx = canvasW - zooBotW
		}
		minBotY := 1
		if canvasH >= 12 {
			minBotY = 5
		}
		if by < minBotY {
			by = minBotY
		}
		if by > canvasH-zooBotH-2 {
			by = canvasH - zooBotH - 2
		}

		state := stateFor[bot.sessionID]
		isSelected := botIdx == m.selected
		isActive := bot.sessionID == m.activeSessionID
		greeting := bot.greetTicks > 0

		color := zooBotColor(state)
		if isSelected {
			color = "230" // bright white highlight
		} else if isActive {
			color = "86" // bright green for connected session
		}

		// Thought bubble.
		bubbleColor := zooThoughtColor(state, bot.behavior)
		bubbleLabel, bubbleDetail := zooThoughtLines(session, bot.ticks)
		if greeting {
			bubbleLabel = "hey there! o/"
			bubbleDetail = ""
		}
		bubbleDrawn := drawZooThoughtBubble(c, bx, by, maxBubbleW, bubbleLabel, bubbleDetail, bubbleColor)

		// Fallback indicator when no bubble fits.
		if !bubbleDrawn {
			if isSelected {
				c.write(bx+2, by-1, " v ", "86")
			} else if state == "waiting-input" {
				c.write(bx+2, by-1, "[!]", "220")
			}
		}

		// Working-bot energy sparks.
		if state == "working" {
			sparkGlyphs := []string{"✦", "·", "*", "·", "✦", " ", "*", " "}
			sparkOffsets := [][2]int{{-1, -1}, {zooBotW, 0}, {zooBotW, -1}, {-1, 0}}
			for si, sp := range sparkOffsets {
				ch := sparkGlyphs[(bot.ticks/2+si*2)%len(sparkGlyphs)]
				if ch != " " {
					c.write(bx+sp[0], by+sp[1], ch, "214")
				}
			}
		}

		moving := !greeting && bot.pauseTicks == 0 &&
			(math.Abs(bot.dx)+math.Abs(bot.dy)) > 0.08 &&
			state != "offline" && state != "error"

		face := zooBotFace(state, bot.ticks, greeting)
		body := zooBotBody(state, bot.ticks)
		legs := zooBotLegs(state, moving, bot.ticks)

		c.write(bx, by, face, color)
		c.write(bx, by+1, body, color)
		c.write(bx, by+2, legs, color)

		// Greeting wave arm (drawn just outside the bot to the right).
		if greeting && (bot.ticks/4)%2 == 0 {
			c.write(bx+zooBotW, by, "o/", color)
		}
		if bot.behavior == "stretch" && by-1 >= 0 {
			c.write(bx+1, by-1, "\\o/", color)
		}
		if bot.behavior == "sit" && by+2 < canvasH {
			c.write(bx+1, by+2, "_/\\_", color)
		}

		// Label: session ID (7 chars padded) below legs.
		if by+3 < canvasH {
			label := bot.sessionID
			if len(label) > zooBotW {
				label = label[:zooBotW]
			}
			for len(label) < zooBotW {
				label += " "
			}
			labelColor := "244"
			switch {
			case isSelected:
				labelColor = "230"
			case isActive:
				labelColor = "86"
			case state == "waiting-input":
				labelColor = "220"
			case state == "error":
				labelColor = "196"
			}
			c.write(bx, by+3, label, labelColor)
		}
		if state == "waiting-input" && !bubbleDrawn && by+4 < canvasH {
			c.write(bx, by+4, "needs input", "220")
		}

		// Active-session dot just after the label.
		if isActive && by+3 < canvasH {
			c.write(bx+zooBotW, by+3, "o", "42")
		}
	}

	zooStr := c.render()
	return lipgloss.JoinVertical(lipgloss.Left, header, zooStr, statusBar)
}
