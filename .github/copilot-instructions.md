# copilot-bridge

## Build, test, and lint commands

### Backend (Go)

- Build the backend from the repository root:
  - `go build ./...`
- Run the backend locally:
  - `go run .`

There are currently no Go test files in the repository.

### Mobile client (Flutter)

- Install Flutter dependencies:
  - `cd mobile && flutter pub get`
- Lint / static analysis:
  - `cd mobile && flutter analyze --no-pub`
- Run all Flutter tests:
  - `cd mobile && flutter test`
- Run the single existing Flutter test:
  - `cd mobile && flutter test test/widget_test.dart`
- Build the web client that the Go server can serve from `mobile/build/web`:
  - `cd mobile && flutter build web`

## High-level architecture

- The repository is a bridge between a local AI agent process and a Flutter UI.
- The Go backend starts and manages per-project sessions for either:
  - `copilot` over ACP on TCP (`copilot --acp --port ...`)
  - `claude-agent-acp` over ACP on stdio
- `main.go` wires a small HTTP API plus a WebSocket endpoint:
  - `POST /api/sessions` creates a session
  - `GET /api/sessions` lists sessions
  - `DELETE /api/sessions/{id}` deletes a session
  - `GET /ws/sessions/{id}` streams live session events
  - `GET /api/browse` returns non-hidden directories for the mobile directory picker
- `session.go` is the core orchestration layer:
  - creates the session object
  - spawns the selected backend process
  - completes the ACP handshake
  - serializes prompts through `PromptQueue`
  - translates ACP notifications and requests into WebSocket events for the UI
- `acp.go` and `protocol.go` define the ACP transport and JSON-RPC/WS payloads. This is the contract layer for both the agent process and the Flutter client.
- `hub.go` provides a per-session fanout hub with retained history. New WebSocket clients receive a synthetic `history` event first, then live updates.
- `terminal.go` handles ACP `terminal/*` requests inside the bridge rather than exposing raw shell execution directly to the mobile app.
- The Flutter app is a thin client around that backend:
  - `mobile/lib/services/api_service.dart` owns REST calls and the per-session WebSocket connection
  - `mobile/lib/screens/sessions_screen.dart` lists sessions and creates new ones
  - `mobile/lib/screens/chat_screen.dart` renders session history/live events and sends prompts
  - `mobile/lib/models/message.dart` maps WebSocket event types into UI message types

## Key conventions

- Keep backend and mobile protocol changes in sync. If you add or rename fields in `WSSessionInfo`, `WSMessage`, or ACP payload structs in `protocol.go`, update the corresponding Flutter models/parsers in `mobile/lib/models/` and `mobile/lib/services/api_service.dart`.
- Session creation flows through multiple layers and should stay aligned:
  - mobile sheet/UI -> `ApiService.createSession(...)`
  - `handlers.go` request body parsing
  - `SessionManager.Create(...)`
  - backend process startup arguments in `session.go`
- Do not add a local user message in the Flutter client when sending a prompt. The backend emits `prompt_sent`, and the UI relies on that server echo to avoid duplicate prompt rows.
- Adjacent `agent_text` chunks are intentionally coalesced in the Flutter client. Preserve that behavior when changing message streaming or history replay so partial ACP chunks still render as one assistant response.
- `PromptQueue` guarantees only one active prompt per session. Keep prompt execution serialized unless you are intentionally redesigning session semantics.
- The session list cards use server-maintained summary fields (`lastMessage`, `currentTool`). Those summaries are derived in `session.go`, not recomputed in the client.
- ACP `session/new` requires `mcpServers` to be encoded as an array, not `null`. In Go this means using an empty slice (`[]`) rather than a nil slice when building `SessionNewParams`.
- The ACP bridge currently logs raw JSON-RPC traffic with `acp >>>` / `acp <<<` in `acp.go`. Keep that in mind when troubleshooting handshake issues or noisy logs.
- The directory browser intentionally returns only directories and skips hidden entries. If you change `BrowseDir`, make sure the mobile directory picker behavior still matches.
- The backend can serve the Flutter web build from `mobile/build/web`, but only if that directory already exists. If you change web serving behavior, keep `main.go` and the Flutter build workflow aligned.
- Model choices are hard-coded in the mobile new-session sheet and also affect backend process startup flags. If model handling changes, update both sides together.
