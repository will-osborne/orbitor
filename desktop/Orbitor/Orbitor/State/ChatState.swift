import AppKit
import Foundation
import UserNotifications

/// Delegate that allows notifications to be displayed even when the app is frontmost.
final class NotificationDelegate: NSObject, UNUserNotificationCenterDelegate {
    static let shared = NotificationDelegate()

    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification,
        withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void
    ) {
        completionHandler([.banner, .sound])
    }
}

@Observable
final class ChatState {
    /// The visible messages — a tail window of allMessages.
    var messages: [ChatMessage] = []
    var isRunning = false
    var pendingPermission: PermissionRequestData?
    var activeSessionID: String?
    var promptHistory: [String] = []
    /// True when there are older messages that can be loaded by scrolling up.
    var hasMoreHistory: Bool { visibleStart > 0 }
    /// True while the initial history load is in progress.
    var isLoadingHistory = false
    /// True while connecting to a session (before any messages arrive).
    var isConnecting = false
    /// Queued prompts waiting to be sent when the current run finishes.
    var queuedPrompts: [String] = []

    private var baseURL: URL
    private var wsClient: WebSocketClient?
    private var streamTask: Task<Void, Never>?
    private var toolCallCache: [String: Int] = [:]

    /// Full message buffer (history + live). Only a tail slice is exposed via `messages`.
    private var allMessages: [ChatMessage] = []
    /// Index into allMessages where the visible window starts.
    private var visibleStart: Int = 0
    /// How many messages to show initially and per "load more" batch.
    private let pageSize = 50

    /// Whether the app window is currently active (suppress notifications when focused).
    var isAppFocused = true

    init(baseURL: URL) {
        self.baseURL = baseURL
        requestNotificationPermission()
    }

    private func requestNotificationPermission() {
        let center = UNUserNotificationCenter.current()
        center.delegate = NotificationDelegate.shared
        center.requestAuthorization(options: [.alert, .sound, .badge]) { granted, error in
            if let error {
                print("[Notifications] authorization error: \(error)")
            } else if !granted {
                print("[Notifications] user denied notification permission")
            }
        }
    }

    private func postNotification(title: String, body: String) {
        // Skip if the app window is currently active
        guard !NSApp.isActive else { return }
        let content = UNMutableNotificationContent()
        content.title = title
        content.body = body
        content.sound = .default
        let request = UNNotificationRequest(identifier: UUID().uuidString, content: content, trigger: nil)
        UNUserNotificationCenter.current().add(request) { error in
            if let error {
                print("[Notifications] delivery error: \(error)")
            }
        }
    }

    func updateBaseURL(_ url: URL) {
        self.baseURL = url
    }

    /// Load an earlier page of messages when the user scrolls to the top.
    @MainActor
    func loadMoreHistory() {
        guard visibleStart > 0 else { return }
        let newStart = max(0, visibleStart - pageSize)
        let prepend = Array(allMessages[newStart..<visibleStart])
        visibleStart = newStart
        messages.insert(contentsOf: prepend, at: 0)
    }

    @MainActor
    func connectToSession(_ sessionID: String) {
        guard sessionID != activeSessionID else { return }

        streamTask?.cancel()
        wsClient?.disconnect()

        activeSessionID = sessionID
        allMessages = []
        messages = []
        visibleStart = 0
        isRunning = false
        isConnecting = true
        pendingPermission = nil
        toolCallCache = [:]
        queuedPrompts = []
        isLoadingHistory = true

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

        // Queue if agent is currently running
        if isRunning {
            queuedPrompts.append(text)
            return
        }

        do {
            try await wsClient?.sendPrompt(text)
        } catch {
            appendLive(.error(id: UUID(), message: "Failed to send: \(error.localizedDescription)", timestamp: Date()))
        }
    }

    @MainActor
    func removeQueuedPrompt(at index: Int) {
        guard index >= 0 && index < queuedPrompts.count else { return }
        queuedPrompts.remove(at: index)
    }

    /// Send the next queued prompt if any.
    @MainActor
    private func drainQueue() async {
        guard !queuedPrompts.isEmpty else { return }
        let next = queuedPrompts.removeFirst()
        do {
            try await wsClient?.sendPrompt(next)
        } catch {
            appendLive(.error(id: UUID(), message: "Failed to send queued message: \(error.localizedDescription)", timestamp: Date()))
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
        // Don't clear isConnecting for status messages — they may set it to true.
        if case .sessionStatus = message { } else { isConnecting = false }

        switch message {
        case .agentText(_, let text, _):
            // Coalesce consecutive agent text
            if let last = allMessages.last, case .agentText(let existingId, let existingText, _) = last {
                let merged = ChatMessage.agentText(id: existingId, text: existingText + text, timestamp: Date())
                allMessages[allMessages.count - 1] = merged
                // Update visible copy if it's in the window
                if !messages.isEmpty, case .agentText = messages.last {
                    messages[messages.count - 1] = merged
                }
            } else {
                appendLive(message)
            }
            isLoadingHistory = false

        case .toolCall(_, let call, _):
            if let bufIdx = toolCallCache[call.toolCallId] {
                if bufIdx < allMessages.count, case .toolCall(let id, var existing, let ts) = allMessages[bufIdx] {
                    if !call.title.isEmpty { existing.title = call.title }
                    if !call.kind.isEmpty { existing.kind = call.kind }
                    if !call.status.isEmpty { existing.status = call.status }
                    if let c = call.content { existing.content = c }
                    let updated = ChatMessage.toolCall(id: id, call: existing, timestamp: ts)
                    allMessages[bufIdx] = updated
                    // Update visible copy
                    let visIdx = bufIdx - visibleStart
                    if visIdx >= 0 && visIdx < messages.count {
                        messages[visIdx] = updated
                    }
                }
            } else {
                toolCallCache[call.toolCallId] = allMessages.count
                appendLive(message)
            }
            isLoadingHistory = false

        case .permissionRequest(_, let request, _):
            pendingPermission = request
            appendLive(message)
            postNotification(title: "Permission Needed", body: request.title)

        case .permissionResolved:
            pendingPermission = nil
            appendLive(message)

        case .promptSent:
            isRunning = true
            isLoadingHistory = false
            appendLive(message)

        case .runComplete:
            isRunning = false
            appendLive(message)
            // Send next queued prompt if any
            Task { @MainActor in
                await drainQueue()
            }

        case .interrupted:
            isRunning = false
            appendLive(message)
            // Send next queued prompt if any
            Task { @MainActor in
                await drainQueue()
            }

        case .historyBatch(_, let batch, _):
            // Load entire history at once — no per-message re-renders.
            // Build tool call cache from history for proper merging of later updates.
            for msg in batch {
                if case .toolCall(_, let call, _) = msg {
                    toolCallCache[call.toolCallId] = allMessages.count
                }
                allMessages.append(msg)
            }
            // Derive isRunning from history: if the last prompt_sent has no
            // subsequent run_complete/interrupted, the agent is still active.
            isRunning = deriveRunningState(from: batch)
            // Show only the tail page
            trimToTail()
            isLoadingHistory = false

        case .sessionStatus(_, let status, _):
            if status == "respawning" {
                isConnecting = true
                isRunning = false
                isLoadingHistory = true
            }

        default:
            appendLive(message)
        }
    }

    /// Append a new live message to the buffer and the visible window.
    @MainActor
    private func appendLive(_ message: ChatMessage) {
        allMessages.append(message)
        messages.append(message)

        // On the first batch of history messages, trim the visible window
        // so we don't render thousands of messages at once.
        if isLoadingHistory && allMessages.count > pageSize {
            trimToTail()
        }
    }

    /// Trim the visible window to the last `pageSize` messages.
    @MainActor
    private func trimToTail() {
        let total = allMessages.count
        if total > pageSize {
            visibleStart = total - pageSize
            messages = Array(allMessages[visibleStart...])
        }
    }

    /// Check whether the agent is mid-run by scanning history for the last
    /// prompt_sent that wasn't followed by run_complete or interrupted.
    private func deriveRunningState(from messages: [ChatMessage]) -> Bool {
        for msg in messages.reversed() {
            switch msg {
            case .runComplete, .interrupted:
                return false
            case .promptSent:
                return true
            default:
                continue
            }
        }
        return false
    }

    /// Called once history loading is done (first non-history message arrives,
    /// or we detect we've received the full history dump).
    @MainActor
    func finalizeHistory() {
        isLoadingHistory = false
        trimToTail()
    }
}
