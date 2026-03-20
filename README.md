# orbitor

**AI coding sessions from your terminal — or your phone.**

Orbitor is a bridge server, terminal UI, and mobile app that lets you run and interact with AI coding assistants (Claude Code and GitHub Copilot) from anywhere. Start a session on your dev machine, watch it work from the TUI in another terminal, and check in from your phone while you're away from your desk.

## What it does

- Runs a local HTTP/WebSocket server that manages multiple named AI coding sessions
- Provides a rich terminal UI (TUI) for creating, switching between, and chatting with sessions
- Has a mobile app (Android/iOS) for monitoring sessions, sending prompts, and approving permission requests on the go
- Supports push-to-talk dictation, session forking, real-time diff display, and graceful binary upgrades with zero downtime

## Screenshots

*Screenshots coming soon.*

---

## Quick Start

Get up and running in under five minutes on the machine where you'll run AI sessions.

**1. Install**

```bash
brew tap OWNER/orbitor
brew install orbitor
```

**2. Configure**

```bash
orbitor setup
```

The interactive setup wizard detects your Tailscale IP, writes `~/.orbitor/config.json`, and optionally installs orbitor as a background login service.

**3. Open the TUI**

```bash
orbitor
```

Press `n` to create a new session in the current directory, or use `orbitor new` from any project directory to create and attach immediately.

---

## Prerequisites

- macOS or Linux
- Go 1.22+ (only needed if building from source)
- At least one of:
  - `claude` CLI — [Claude Code docs](https://docs.anthropic.com/en/docs/claude-code)
  - `gh copilot` — GitHub Copilot in the CLI
- The CLI you use must already be authenticated
- [Tailscale](https://tailscale.com) — recommended for mobile access and remote TUI connections

---

## Installation

### Homebrew (recommended)

```bash
brew tap OWNER/orbitor
brew install orbitor
```

### Build from source

```bash
git clone https://github.com/OWNER/orbitor
cd orbitor
go build -o orbitor .
./orbitor setup
```

---

## Usage

### Subcommands

| Command | Description |
|---|---|
| `orbitor` | Open the TUI (default) |
| `orbitor new` | Create a new session for the current directory and attach |
| `orbitor server` | Run the HTTP server in the foreground |
| `orbitor setup` | Run the interactive setup wizard |
| `orbitor service <action>` | Manage the background service (see below) |

### Creating sessions from the command line

```bash
# Create a session in the current directory using config defaults
orbitor new

# Override backend and model
orbitor new claude claude-sonnet-4-6
orbitor new copilot gpt-4o
```

---

## Configuration

Orbitor reads `~/.orbitor/config.json`. Run `orbitor setup` to generate this file, or create it manually:

```json
{
  "serverURL": "http://100.x.x.x:8080",
  "listenAddr": "100.x.x.x:8080",
  "defaultBackend": "claude",
  "defaultModel": "claude-sonnet-4-6",
  "skipPermissions": false,
  "planMode": false
}
```

| Field | Description |
|---|---|
| `serverURL` | URL the TUI connects to. Use `http://127.0.0.1:8080` for local, or your Tailscale IP for remote. |
| `listenAddr` | Address the server binds to. Set to your Tailscale IP to allow remote clients. |
| `defaultBackend` | `"claude"` or `"copilot"` |
| `defaultModel` | Model name passed to the backend (e.g. `claude-sonnet-4-6`, `gpt-4o`) |
| `skipPermissions` | Auto-approve all permission prompts (`--dangerously-skip-permissions` / `--yolo`) |
| `planMode` | Start sessions in plan mode |

---

## Tailscale Setup

Tailscale provides secure peer-to-peer connectivity between the server and your phone or a colleague's TUI — no port forwarding or VPN configuration required.

1. Install Tailscale on both the server machine and your phone: [tailscale.com/download](https://tailscale.com/download)
2. Sign in with the same account on both devices
3. Run `orbitor setup` — it auto-detects your Tailscale IP and sets `listenAddr` and `serverURL` accordingly
4. Share your Tailscale IP with colleagues (`tailscale ip -4`)

### Connecting a remote TUI

A colleague on a different machine can connect to your server without running their own:

```bash
brew tap OWNER/orbitor && brew install orbitor
orbitor setup    # enter the server machine's Tailscale IP when prompted
orbitor          # opens TUI pointed at your server
```

---

## TUI Key Bindings

| Key | Action |
|---|---|
| `Enter` | Connect to session / send prompt |
| `Shift+Enter` | Insert newline in prompt |
| `Alt+Enter` | Fork: clone session and send prompt to the clone |
| `Tab` / `Shift+Tab` | Cycle through sessions |
| `↑` / `↓` | Scroll chat history |
| `Ctrl+↑` / `Ctrl+↓` | Navigate prompt history |
| `n` | New session |
| `Ctrl+D` | Delete session |
| `Ctrl+V` | Paste image or file path |
| `Hold Space` | Push-to-talk dictation |
| `Ctrl+.` / `Ctrl+\` | Interrupt / abort current run |
| `Ctrl+M` | Toggle markdown rendering |
| `Ctrl+B` | Toggle compact blocks |
| `Ctrl+T` | Cycle themes |
| `Ctrl+L` | Clear chat |

---

## Slash Commands

Type these in the TUI prompt:

| Command | Description |
|---|---|
| `/new <dir> [backend] [model]` | Create a new session in the given directory |
| `/fork <prompt>` | Clone the current session and send a prompt to the clone |
| `/use <id>` | Switch to a session by ID |
| `/interrupt` | Interrupt the current run |
| `/allow <reqId> <optId>` | Approve a pending permission request |
| `/skip [true\|false]` | Toggle skip-permissions for the current session |
| `/help` | List all commands |

---

## Mobile App

The Orbitor mobile app lets you monitor sessions, read output, send prompts, and approve permission requests from your phone.

1. Download the app *(link TBD)*
2. Open the app and enter your server's address (e.g. `http://100.x.x.x:8080`)
3. Tap a session to open the chat view
4. Tap the permission banner to approve or deny requests

Push notifications are sent when a session completes a run or needs your attention.

### Push notification setup (self-hosted)

The pre-built APK is configured to use the maintainer's Firebase project. If you're self-hosting and want push notifications, you'll need to wire up your own Firebase project:

1. Create a project at [console.firebase.google.com](https://console.firebase.google.com)
2. Add an Android app with package name matching `mobile/android/app/build.gradle.kts`
3. Download `google-services.json` and replace `mobile/android/app/google-services.json`
4. In Project Settings → Service Accounts, generate a new private key and save the JSON file on your server
5. Set `firebase.service_account_path` in your `config/config.yaml` to point at that file
6. Rebuild the APK: `cd mobile && flutter build apk --release`

> **Note:** This is intentionally manual for now. A future release will replace this with a self-hostable push relay (e.g. [ntfy](https://ntfy.sh)) that requires no Firebase account or APK rebuild.

### Mobile development

```bash
cd mobile
flutter pub get
flutter run
```

---

## Service Management

Run orbitor as a persistent background service that starts automatically at login.

```bash
orbitor service install    # install and enable auto-start
orbitor service uninstall  # remove the service
orbitor service start      # start now
orbitor service stop       # stop
orbitor service restart    # restart
orbitor service status     # check if running
orbitor service logs       # tail logs
```

Homebrew services also work if installed via Homebrew:

```bash
brew services start orbitor
brew services stop orbitor
```

---

## How It Works

Orbitor runs a lightweight HTTP/WebSocket server that spawns and manages AI assistant processes (`claude` or `gh copilot`). Each session gets its own working directory, backend process, and message history.

- **TUI and mobile clients** connect via WebSocket to stream session output in real time
- **Session state** is persisted to a local SQLite database (`orbitor.db`) and restored on restart
- **Graceful upgrades** are handled via [tableflip](https://github.com/cloudflare/tableflip): a `SIGHUP` spawns the new binary, hands off the listening socket, and the old process drains cleanly — clients reconnect automatically with no dropped sessions
- **Push notifications** use Firebase Cloud Messaging (FCM) with an optional local LLM summarizer to generate notification text

---

## License

*License TBD.*
