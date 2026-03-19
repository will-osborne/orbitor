package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ServiceInstall installs and enables the orbitor server as a background service.
func ServiceInstall() {
	switch runtime.GOOS {
	case "darwin":
		serviceInstallDarwin()
	case "linux":
		serviceInstallLinux()
	default:
		fmt.Println("Service management is not supported on this platform.")
	}
}

// ServiceUninstall stops and removes the background service.
func ServiceUninstall() {
	switch runtime.GOOS {
	case "darwin":
		serviceUninstallDarwin()
	case "linux":
		serviceUninstallLinux()
	default:
		fmt.Println("Service management is not supported on this platform.")
	}
}

// ServiceStart starts the background service.
func ServiceStart() {
	switch runtime.GOOS {
	case "darwin":
		fmt.Println("Starting orbitor service...")
		runCmd("launchctl", "start", "io.orbitor.server")
	case "linux":
		fmt.Println("Starting orbitor service...")
		runCmd("systemctl", "--user", "start", "orbitor")
	default:
		fmt.Println("Service management is not supported on this platform.")
	}
}

// ServiceStop stops the background service.
func ServiceStop() {
	switch runtime.GOOS {
	case "darwin":
		fmt.Println("Stopping orbitor service...")
		runCmd("launchctl", "stop", "io.orbitor.server")
	case "linux":
		fmt.Println("Stopping orbitor service...")
		runCmd("systemctl", "--user", "stop", "orbitor")
	default:
		fmt.Println("Service management is not supported on this platform.")
	}
}

// ServiceRestart restarts the background service.
func ServiceRestart() {
	switch runtime.GOOS {
	case "darwin":
		fmt.Println("Restarting orbitor service...")
		runCmd("launchctl", "stop", "io.orbitor.server")
		runCmd("launchctl", "start", "io.orbitor.server")
	case "linux":
		fmt.Println("Restarting orbitor service...")
		runCmd("systemctl", "--user", "restart", "orbitor")
	default:
		fmt.Println("Service management is not supported on this platform.")
	}
}

// ServiceStatus prints the current status of the background service.
func ServiceStatus() {
	switch runtime.GOOS {
	case "darwin":
		fmt.Println("orbitor service status:")
		runCmd("launchctl", "list", "io.orbitor.server")
	case "linux":
		fmt.Println("orbitor service status:")
		runCmd("systemctl", "--user", "status", "orbitor")
	default:
		fmt.Println("Service management is not supported on this platform.")
	}
}

// ServiceLogs tails the orbitor server log.
func ServiceLogs() {
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Printf("Error finding home directory: %v\n", err)
			return
		}
		logPath := filepath.Join(home, ".orbitor", "server.log")
		fmt.Printf("Tailing %s (Ctrl-C to stop)...\n", logPath)
		runCmdInteractive("tail", "-n", "50", "-f", logPath)
	case "linux":
		fmt.Println("Tailing orbitor service logs (Ctrl-C to stop)...")
		runCmdInteractive("journalctl", "--user", "-u", "orbitor", "-n", "50", "-f")
	default:
		fmt.Println("Service management is not supported on this platform.")
	}
}

// serviceInstallDarwin installs the launchd plist and loads it.
func serviceInstallDarwin() {
	binaryPath, err := os.Executable()
	if err != nil {
		fmt.Printf("Error detecting executable path: %v\n", err)
		binaryPath = "orbitor"
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Error finding home directory: %v\n", err)
		return
	}

	plistDir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(plistDir, 0o755); err != nil {
		fmt.Printf("Error creating LaunchAgents directory: %v\n", err)
		return
	}

	plistPath := filepath.Join(plistDir, "io.orbitor.server.plist")
	logPath := filepath.Join(home, ".orbitor", "server.log")

	// Capture PATH at install time so the service can find claude-agent-acp,
	// copilot, and other tools that live outside the default launchd PATH.
	currentPath := os.Getenv("PATH")
	if currentPath == "" {
		currentPath = "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
	}

	plistContent := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key><string>io.orbitor.server</string>
    <key>ProgramArguments</key>
    <array><string>` + binaryPath + `</string><string>server</string></array>
    <key>RunAtLoad</key><true/>
    <key>KeepAlive</key><true/>
    <key>StandardOutPath</key><string>` + logPath + `</string>
    <key>StandardErrorPath</key><string>` + logPath + `</string>
    <key>WorkingDirectory</key><string>` + home + `</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key><string>` + currentPath + `</string>
    </dict>
</dict>
</plist>
`

	fmt.Printf("Writing plist to %s...\n", plistPath)
	if err := os.WriteFile(plistPath, []byte(plistContent), 0o644); err != nil {
		fmt.Printf("Error writing plist: %v\n", err)
		return
	}

	fmt.Println("Loading service with launchctl...")
	runCmd("launchctl", "load", "-w", plistPath)
	fmt.Println("orbitor server service installed and started.")
}

// serviceUninstallDarwin unloads and removes the launchd plist.
func serviceUninstallDarwin() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Error finding home directory: %v\n", err)
		return
	}

	plistPath := filepath.Join(home, "Library", "LaunchAgents", "io.orbitor.server.plist")

	fmt.Println("Unloading service with launchctl...")
	runCmd("launchctl", "unload", "-w", plistPath)

	fmt.Printf("Removing plist at %s...\n", plistPath)
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		fmt.Printf("Error removing plist: %v\n", err)
		return
	}
	fmt.Println("orbitor server service uninstalled.")
}

// serviceInstallLinux installs the systemd user unit and enables it.
func serviceInstallLinux() {
	binaryPath, err := os.Executable()
	if err != nil {
		fmt.Printf("Error detecting executable path: %v\n", err)
		binaryPath = "orbitor"
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Error finding home directory: %v\n", err)
		return
	}

	unitDir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		fmt.Printf("Error creating systemd user directory: %v\n", err)
		return
	}

	unitPath := filepath.Join(unitDir, "orbitor.service")

	unitContent := `[Unit]
Description=Orbitor AI coding assistant bridge server
After=network.target

[Service]
ExecStart=` + binaryPath + ` server
Restart=on-failure
RestartSec=5
StandardOutput=append:%h/.orbitor/server.log
StandardError=append:%h/.orbitor/server.log

[Install]
WantedBy=default.target
`

	fmt.Printf("Writing unit file to %s...\n", unitPath)
	if err := os.WriteFile(unitPath, []byte(unitContent), 0o644); err != nil {
		fmt.Printf("Error writing unit file: %v\n", err)
		return
	}

	fmt.Println("Reloading systemd daemon...")
	runCmd("systemctl", "--user", "daemon-reload")

	fmt.Println("Enabling and starting orbitor service...")
	runCmd("systemctl", "--user", "enable", "--now", "orbitor")
	fmt.Println("orbitor server service installed and started.")
}

// serviceUninstallLinux disables and removes the systemd user unit.
func serviceUninstallLinux() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Error finding home directory: %v\n", err)
		return
	}

	unitPath := filepath.Join(home, ".config", "systemd", "user", "orbitor.service")

	fmt.Println("Disabling and stopping orbitor service...")
	runCmd("systemctl", "--user", "disable", "--now", "orbitor")

	fmt.Printf("Removing unit file at %s...\n", unitPath)
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		fmt.Printf("Error removing unit file: %v\n", err)
	}

	fmt.Println("Reloading systemd daemon...")
	runCmd("systemctl", "--user", "daemon-reload")
	fmt.Println("orbitor server service uninstalled.")
}

// runCmd runs a command, printing its combined output. Errors are reported but not fatal.
func runCmd(name string, args ...string) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		fmt.Print(strings.TrimRight(string(out), "\n") + "\n")
	}
	if err != nil {
		fmt.Printf("Command %q exited with error: %v\n", name+" "+strings.Join(args, " "), err)
	}
}

// runCmdInteractive runs a command with stdio connected to the terminal.
func runCmdInteractive(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("Command %q exited: %v\n", name+" "+strings.Join(args, " "), err)
	}
}
