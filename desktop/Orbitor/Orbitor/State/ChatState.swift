import Foundation

@Observable
final class ChatState {
    var messages: [ChatMessage] = []
    var isRunning = false
    var pendingPermission: PermissionRequestData?
    var activeSessionID: String?
    var promptHistory: [String] = []

    private var baseURL: URL
    private var wsClient: WebSocketClient?
    private var streamTask: Task<Void, Never>?
    // Track tool calls for merging updates
    private var toolCallCache: [String: Int] = [:]

    init(baseURL: URL) {
        self.baseURL = baseURL
    }

    func updateBaseURL(_ url: URL) {
        self.baseURL = url
    }

    @MainActor
    func connectToSession(_ sessionID: String) {
        guard sessionID != activeSessionID else { return }

        // Disconnect previous
        streamTask?.cancel()
        wsClient?.disconnect()

        activeSessionID = sessionID
        messages = []
        isRunning = false
        pendingPermission = nil
        toolCallCache = [:]

        let client = WebSocketClient(baseURL: baseURL)
        wsClient = client

        streamTask = Task { [weak self] in
            let stream = await client.connect(sessionID: sessionID)
            for await message in stream {
                guard !Task.isCancelled else { break }
                await self?.handleMessage(message)
            }
        }
    }

    @MainActor
    func disconnect() {
        streamTask?.cancel()
        wsClient?.disconnect()
        wsClient = nil
        activeSessionID = nil
    }

    @MainActor
    func sendPrompt(_ text: String) async {
        guard !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else { return }
        promptHistory.append(text)
        if promptHistory.count > 100 { promptHistory.removeFirst() }

        do {
            try await wsClient?.sendPrompt(text)
        } catch {
            messages.append(.error(id: UUID(), message: "Failed to send: \(error.localizedDescription)", timestamp: Date()))
        }
    }

    @MainActor
    func interrupt() async {
        try? await wsClient?.sendInterrupt()
    }

    @MainActor
    func respondToPermission(requestId: String, optionId: String) async {
        try? await wsClient?.sendPermissionResponse(requestId: requestId, optionId: optionId)
        pendingPermission = nil
    }

    // MARK: - Message handling

    @MainActor
    private func handleMessage(_ message: ChatMessage) {
        switch message {
        case .agentText(_, let text, _):
            // Coalesce consecutive agent text messages
            if case .agentText(let existingId, let existingText, _) = messages.last {
                messages[messages.count - 1] = .agentText(id: existingId, text: existingText + text, timestamp: Date())
            } else {
                messages.append(message)
            }

        case .toolCall(_, let call, _):
            // Merge with existing tool call or add new
            if let idx = toolCallCache[call.toolCallId] {
                if idx < messages.count, case .toolCall(let id, var existing, let ts) = messages[idx] {
                    if !call.title.isEmpty { existing.title = call.title }
                    if !call.kind.isEmpty { existing.kind = call.kind }
                    if !call.status.isEmpty { existing.status = call.status }
                    if let c = call.content { existing.content = c }
                    messages[idx] = .toolCall(id: id, call: existing, timestamp: ts)
                }
            } else {
                toolCallCache[call.toolCallId] = messages.count
                messages.append(message)
            }

        case .permissionRequest(_, let request, _):
            pendingPermission = request
            messages.append(message)

        case .permissionResolved:
            pendingPermission = nil
            messages.append(message)

        case .promptSent:
            isRunning = true
            messages.append(message)

        case .runComplete:
            isRunning = false
            messages.append(message)

        case .interrupted:
            isRunning = false
            messages.append(message)

        default:
            messages.append(message)
        }
    }
}
