package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// runSetup runs the interactive orbitor setup wizard.
func runSetup() error {
	fmt.Println("╔═══════════════════════════════════╗")
	fmt.Println("║       orbitor setup wizard        ║")
	fmt.Println("╚═══════════════════════════════════╝")
	fmt.Println()
	fmt.Println("Welcome! This wizard configures orbitor on this machine.")
	fmt.Println("Run it again any time with: orbitor setup")
	fmt.Println()

	// Check for existing config.
	configPath, err := ClientConfigPath()
	if err != nil {
		return fmt.Errorf("finding config path: %w", err)
	}
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("Existing config found at %s — reconfiguring.\n", configPath)
		fmt.Println()
	}

	reader := bufio.NewReader(os.Stdin)
	cfg := defaultClientConfig()

	// --- Step 1: Role ---
	fmt.Println("--- Step 1: Role ---")
	isServer := promptYesNo(reader, "Is this machine running the orbitor server?", true)
	fmt.Println()

	serviceInstalled := false

	if isServer {
		tsIP := detectTailscaleIP()
		var defaultAddr string
		if tsIP != "" {
			fmt.Printf("✓ Tailscale detected: %s\n", tsIP)
			fmt.Println("Orbitor will listen on your Tailscale IP so colleagues can connect.")
			defaultAddr = tsIP + ":8080"
		} else {
			fmt.Println("Tailscale not detected. Defaulting to localhost.")
			fmt.Println("Tip: Install Tailscale (tailscale.com) so colleagues can connect from their phones.")
			defaultAddr = "127.0.0.1:8080"
		}

		listenAddr := promptDefault(reader, fmt.Sprintf("Server listen address [%s]: ", defaultAddr), defaultAddr)
		cfg.ListenAddr = listenAddr
		cfg.ServerURL = "http://" + listenAddr
	} else {
		serverURL := promptDefault(reader, "Enter the orbitor server URL (e.g. http://100.x.x.x:8080): ", "")
		cfg.ServerURL = serverURL
		cfg.ListenAddr = ""
	}
	fmt.Println()

	// --- Step 2: Backend ---
	fmt.Println("--- Step 2: Backend ---")
	backend := promptDefault(reader, "Default backend (claude/copilot) [claude]: ", "claude")
	if backend != "claude" && backend != "copilot" {
		fmt.Printf("Unknown backend %q, defaulting to claude.\n", backend)
		backend = "claude"
	}
	cfg.DefaultBackend = backend
	fmt.Println()

	// --- Step 3: Model ---
	fmt.Println("--- Step 3: Model (optional) ---")
	model := promptDefault(reader, "Default model (leave blank for backend default): ", "")
	cfg.DefaultModel = model
	fmt.Println()

	// --- Step 4: Service (server machines only) ---
	if isServer && (runtime.GOOS == "darwin" || runtime.GOOS == "linux") {
		fmt.Println("--- Step 4: Service ---")
		installService := promptYesNo(reader, "Install orbitor server as a background service?", true)
		if installService {
			ServiceInstall()
			serviceInstalled = true
		}
		fmt.Println()
	}

	// --- Write config ---
	if err := writeClientConfig(cfg, configPath); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	fmt.Printf("\n✓ Config written to %s\n", configPath)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  • Run 'orbitor setup-terminal' to check keyboard/terminal compatibility")
	fmt.Println("  • Run 'orbitor' to open the TUI")
	if isServer {
		if !serviceInstalled {
			fmt.Println("  • Run 'orbitor service install' to start the server automatically")
		} else {
			fmt.Println("  • Server is running in the background")
		}
		if tsIP := detectTailscaleIP(); tsIP != "" {
			fmt.Printf("  • Share your Tailscale IP (%s) with colleagues so they can connect\n", tsIP)
		}
	}

	return nil
}

// detectTailscaleIP returns the machine's Tailscale IPv4 address, or "" if not available.
func detectTailscaleIP() string {
	out, err := exec.Command("tailscale", "ip", "-4").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// promptDefault prints a prompt and returns the user's input, or defaultVal if blank.
func promptDefault(reader *bufio.Reader, prompt, defaultVal string) string {
	fmt.Print(prompt)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultVal
	}
	return line
}

// promptYesNo prints a yes/no prompt and returns the boolean result.
func promptYesNo(reader *bufio.Reader, prompt string, defaultYes bool) bool {
	hint := "[Y/n]"
	if !defaultYes {
		hint = "[y/N]"
	}
	fmt.Printf("%s %s: ", prompt, hint)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return defaultYes
	}
	return line == "y" || line == "yes"
}

// writeClientConfig marshals cfg and writes it to path, ensuring the directory exists.
func writeClientConfig(cfg ClientConfig, path string) error {
	if err := os.MkdirAll(strings.TrimSuffix(path, "/config.json"), 0o755); err != nil {
		// best effort — OrbitorDir already handles this
		_ = err
	}
	// Ensure directory exists via OrbitorDir (which calls MkdirAll).
	if _, err := OrbitorDir(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
