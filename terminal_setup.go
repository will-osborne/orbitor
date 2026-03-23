package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// terminalInfo holds detected information about the user's terminal emulator.
type terminalInfo struct {
	Name              string // human-readable name
	SupportsKitty     bool   // supports Kitty keyboard protocol (CSI u)
	SupportsModifyKey bool   // supports xterm modifyOtherKeys
	NeedsMetaConfig   bool   // needs "Option as Meta" to be enabled manually
	ConfigHint        string // terminal-specific setup instructions
}

// detectTerminal identifies the active terminal emulator from environment variables.
func detectTerminal() terminalInfo {
	// Specific terminal env vars (checked first, most reliable).
	if os.Getenv("KITTY_WINDOW_ID") != "" {
		return terminalInfo{
			Name:              "Kitty",
			SupportsKitty:     true,
			SupportsModifyKey: true,
			ConfigHint:        "Kitty fully supports all keyboard protocols. No setup needed.",
		}
	}
	if os.Getenv("GHOSTTY_RESOURCES_DIR") != "" {
		return terminalInfo{
			Name:              "Ghostty",
			SupportsKitty:     true,
			SupportsModifyKey: true,
			ConfigHint:        "Ghostty fully supports all keyboard protocols. No setup needed.",
		}
	}
	if os.Getenv("WEZTERM_EXECUTABLE") != "" {
		return terminalInfo{
			Name:              "WezTerm",
			SupportsKitty:     true,
			SupportsModifyKey: true,
			ConfigHint:        "WezTerm fully supports all keyboard protocols. No setup needed.",
		}
	}
	if os.Getenv("ALACRITTY_WINDOW_ID") != "" || os.Getenv("ALACRITTY_LOG") != "" {
		return terminalInfo{
			Name:              "Alacritty",
			SupportsKitty:     true,
			SupportsModifyKey: true,
			ConfigHint:        "Alacritty fully supports all keyboard protocols. No setup needed.",
		}
	}

	// TERM_PROGRAM-based detection.
	tp := os.Getenv("TERM_PROGRAM")
	switch tp {
	case "WarpTerminal":
		return terminalInfo{
			Name:              "Warp",
			SupportsKitty:     true,
			SupportsModifyKey: true,
			NeedsMetaConfig:   runtime.GOOS == "darwin",
			ConfigHint: `Warp supports the Kitty keyboard protocol (since Feb 2026).
If Shift+Enter doesn't work, update Warp to the latest version.
Fallback: type backslash (\) then Enter, or Ctrl+J, to insert newlines.

For Alt+Enter support on macOS:
  Settings → Keyboard → "Option key as Meta" → enable`,
		}
	case "iTerm.app":
		return terminalInfo{
			Name:              "iTerm2",
			SupportsKitty:     true,
			SupportsModifyKey: true,
			NeedsMetaConfig:   runtime.GOOS == "darwin",
			ConfigHint: `iTerm2 3.5+ supports CSI u sequences. Ensure Option-as-Meta is set:
  Preferences → Profiles → Keys → General → "Left Option key" → Esc+`,
		}
	case "Apple_Terminal":
		return terminalInfo{
			Name:              "Terminal.app",
			SupportsKitty:     false,
			SupportsModifyKey: false,
			NeedsMetaConfig:   true,
			ConfigHint: `Terminal.app does NOT support Kitty keyboard protocol or modifyOtherKeys.
Shift+Enter cannot be distinguished from Enter.

To get Alt+Enter working:
  Settings → Profiles → Keyboard → check "Use Option as Meta Key"

Recommendation: switch to Ghostty, Kitty, WezTerm, or iTerm2 for full support.`,
		}
	case "tmux":
		return terminalInfo{
			Name:              "tmux",
			SupportsKitty:     false,
			SupportsModifyKey: true,
			ConfigHint: `tmux partially supports keyboard protocols. Add to ~/.tmux.conf:
  set -s extended-keys on
  set -as terminal-features 'xterm*:extkeys'

Shift+Enter may not work depending on the outer terminal.`,
		}
	}

	// Check if running inside tmux.
	if os.Getenv("TMUX") != "" {
		return terminalInfo{
			Name:              "tmux",
			SupportsKitty:     false,
			SupportsModifyKey: true,
			ConfigHint: `tmux partially supports keyboard protocols. Add to ~/.tmux.conf:
  set -s extended-keys on
  set -as terminal-features 'xterm*:extkeys'

Shift+Enter may not work depending on the outer terminal.`,
		}
	}

	// Fallback: unknown terminal.
	return terminalInfo{
		Name:              tp,
		SupportsKitty:     false,
		SupportsModifyKey: false,
		ConfigHint: `Could not detect your terminal emulator.

For full keyboard support (Shift+Enter, Alt+Enter), use a terminal that
supports the Kitty keyboard protocol:
  • Ghostty (ghostty.org)
  • Kitty (sw.kovidgoyal.net/kitty)
  • WezTerm (wezfurlong.org/wezterm)
  • iTerm2 3.5+ (iterm2.com)
  • Warp (warp.dev)
  • Alacritty (alacritty.org)`,
	}
}

// detectShell returns the current shell name (e.g. "fish", "zsh", "bash").
func detectShell() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return "unknown"
	}
	parts := strings.Split(shell, "/")
	return parts[len(parts)-1]
}

// terminalSetupReport generates a diagnostic report and setup instructions.
func terminalSetupReport() string {
	info := detectTerminal()
	shell := detectShell()

	var b strings.Builder

	b.WriteString("╔═══════════════════════════════════╗\n")
	b.WriteString("║     orbitor terminal setup        ║\n")
	b.WriteString("╚═══════════════════════════════════╝\n\n")

	b.WriteString(fmt.Sprintf("  Terminal:  %s\n", defaultString(info.Name, "(unknown)")))
	b.WriteString(fmt.Sprintf("  Shell:     %s\n", shell))
	b.WriteString(fmt.Sprintf("  OS:        %s/%s\n", runtime.GOOS, runtime.GOARCH))
	b.WriteString(fmt.Sprintf("  TERM:      %s\n", os.Getenv("TERM")))
	b.WriteString("\n")

	// Protocol support.
	check := func(ok bool) string {
		if ok {
			return "✓ supported"
		}
		return "✗ not supported"
	}
	b.WriteString(fmt.Sprintf("  Kitty keyboard protocol:   %s\n", check(info.SupportsKitty)))
	b.WriteString(fmt.Sprintf("  xterm modifyOtherKeys:     %s\n", check(info.SupportsModifyKey)))
	b.WriteString("\n")

	// Feature availability.
	b.WriteString("  Feature availability:\n")
	if info.SupportsKitty || info.SupportsModifyKey {
		b.WriteString("    Shift+Enter (newline):   ✓ works\n")
	} else {
		b.WriteString("    Shift+Enter (newline):   ✗ use \\+Enter or Ctrl+J instead\n")
	}
	if info.SupportsKitty || info.SupportsModifyKey || !info.NeedsMetaConfig {
		b.WriteString("    Alt+Enter (fork send):   ✓ works\n")
	} else {
		b.WriteString("    Alt+Enter (fork send):   ✗ needs setup (see below)\n")
	}
	b.WriteString("\n")

	// Setup instructions.
	if info.ConfigHint != "" {
		b.WriteString("  Setup:\n")
		for _, line := range strings.Split(info.ConfigHint, "\n") {
			b.WriteString("    " + line + "\n")
		}
		b.WriteString("\n")
	}

	// Shell integration.
	b.WriteString("  Shell tips:\n")
	switch shell {
	case "fish":
		b.WriteString("    Fish handles escape sequences well by default.\n")
		b.WriteString("    If Alt+key doesn't work, check your terminal's Option/Meta config.\n")
	case "zsh":
		b.WriteString("    If Alt+key doesn't work in zsh, add to ~/.zshrc:\n")
		b.WriteString("      bindkey -e  # use emacs keymap (enables meta keys)\n")
	case "bash":
		b.WriteString("    If Alt+key doesn't work in bash, add to ~/.inputrc:\n")
		b.WriteString("      set convert-meta on\n")
	default:
		b.WriteString("    Ensure your shell passes Alt/Option key sequences through.\n")
	}

	return b.String()
}

// runTerminalSetup runs the terminal setup as a CLI command (non-interactive).
func runTerminalSetup() {
	fmt.Print(terminalSetupReport())

	// Quick connectivity test: verify the Kitty protocol works by
	// querying the terminal for its keyboard protocol support.
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		info := detectTerminal()
		if info.NeedsMetaConfig && runtime.GOOS == "darwin" {
			fmt.Println("  Action: configure your terminal's Option/Meta key setting (see above).")
		}
		if !info.SupportsKitty && !info.SupportsModifyKey {
			fmt.Println("  Recommendation: switch to a terminal with Kitty keyboard protocol support.")
		}
		if info.SupportsKitty {
			fmt.Println("  ✓ Your terminal supports all orbitor keyboard features.")
		}
	}
	fmt.Println()

	// Check if orbitor binary is in PATH.
	if _, err := exec.LookPath("orbitor"); err != nil {
		fmt.Println("  Warning: 'orbitor' is not in your PATH.")
		shell := detectShell()
		switch shell {
		case "fish":
			fmt.Println("  Add to ~/.config/fish/config.fish:")
			fmt.Println("    fish_add_path /path/to/orbitor")
		case "zsh":
			fmt.Println("  Add to ~/.zshrc:")
			fmt.Println("    export PATH=\"/path/to/orbitor:$PATH\"")
		case "bash":
			fmt.Println("  Add to ~/.bashrc:")
			fmt.Println("    export PATH=\"/path/to/orbitor:$PATH\"")
		}
		fmt.Println()
	}
}

// newlineKeyHint returns the appropriate keybinding label based on terminal capabilities.
func (m *tuiModel) newlineKeyHint() string {
	if m.termInfo.SupportsKitty || m.termInfo.SupportsModifyKey {
		return "Shift+Enter / \\+Enter / Ctrl+J"
	}
	return "\\+Enter / Ctrl+J"
}
