import Foundation

final class WebSocketClient: @unchecked Sendable {
    private let baseURL: URL
    private var task: URLSessionWebSocketTask?
    private var continuation: AsyncStream<ChatMessage>.Continuation?
    private var isConnected = false
    private let decoder: JSONDecoder

    init(baseURL: URL) {
        self.baseURL = baseURL
        let dec = JSONDecoder()
        dec.dateDecodingStrategy = .custom { decoder in
            let container = try decoder.singleValueContainer()
            let str = try container.decode(String.self)
            let f1 = ISO8601DateFormatter()
            f1.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            let f2 = ISO8601DateFormatter()
            f2.formatOptions = [.withInternetDateTime]
            if let d = f1.date(from: str) { return d }
            if let d = f2.date(from: str) { return d }
            throw DecodingError.dataCorruptedError(in: container, debugDescription: "Bad date: \(str)")
        }
        self.decoder = dec
    }

    func connect(sessionID: String) -> AsyncStream<ChatMessage> {
        disconnect()

        let wsScheme = baseURL.scheme == "https" ? "wss" : "ws"
        var components = URLComponents(url: baseURL, resolvingAgainstBaseURL: false)!
        components.scheme = wsScheme
        components.path = "/ws/sessions/\(sessionID)"
        let url = components.url!

        let session = URLSession(configuration: .default)
        let wsTask = session.webSocketTask(with: url)
        wsTask.maximumMessageSize = 64 * 1024 * 1024  // 64 MB for large chat histories
        self.task = wsTask

        let stream = AsyncStream<ChatMessage> { cont in
            self.continuation = cont
        }

        wsTask.resume()
        isConnected = true

        Task { [weak self] in
            await self?.receiveLoop()
        }

        return stream
    }

    func send(_ message: Encodable) async throws {
        guard let task else { return }
        let data = try JSONEncoder().encode(message)
        try await task.send(.string(String(data: data, encoding: .utf8)!))
    }

    func sendPrompt(_ text: String) async throws {
        try await send(WSPromptMessage(text: text))
    }

    func sendInterrupt() async throws {
        try await send(WSInterruptMessage())
    }

    func sendPermissionResponse(requestId: String, optionId: String) async throws {
        try await send(WSPermissionResponseMessage(requestId: requestId, optionId: optionId))
    }

    func disconnect() {
        isConnected = false
        task?.cancel(with: .normalClosure, reason: nil)
        task = nil
        continuation?.finish()
        continuation = nil
    }

    // MARK: - Private

    private func receiveLoop() async {
        guard let task else { return }
        while isConnected {
            do {
                let msg = try await task.receive()
                switch msg {
                case .string(let text):
                    guard let data = text.data(using: .utf8) else { continue }
                    await processRawMessage(data)
                case .data(let data):
                    await processRawMessage(data)
                @unknown default:
                    break
                }
            } catch {
                if isConnected {
                    continuation?.yield(.error(id: UUID(), message: "WebSocket disconnected: \(error.localizedDescription)", timestamp: Date()))
                }
                break
            }
        }
        continuation?.finish()
    }

    private func processRawMessage(_ data: Data) async {
        // First determine the type
        guard let envelope = try? decoder.decode(WSEnvelope.self, from: data) else { return }

        let now = Date()

        if envelope.type == "history" {
            let parsed = (envelope.messages ?? []).compactMap { parseEnvelope($0, timestamp: now) }
            continuation?.yield(.historyBatch(id: UUID(), messages: parsed, timestamp: now))
            return
        }

        if let parsed = parseEnvelope(envelope, timestamp: now) {
            continuation?.yield(parsed)
        }
    }

    private func parseEnvelope(_ envelope: WSEnvelope, timestamp: Date) -> ChatMessage? {
        let id = UUID()
        switch envelope.type {
        case "agent_text":
            let text = envelope.data?["text"]?.stringValue ?? ""
            return .agentText(id: id, text: text, timestamp: timestamp)

        case "tool_call":
            guard let d = envelope.data else { return nil }
            let call = ToolCallData(
                toolCallId: d["toolCallId"]?.stringValue ?? "",
                title: d["title"]?.stringValue ?? "",
                kind: d["kind"]?.stringValue ?? "",
                status: d["status"]?.stringValue ?? "",
                content: d["content"]?.stringValue
            )
            return .toolCall(id: id, call: call, timestamp: timestamp)

        case "tool_result":
            guard let d = envelope.data else { return nil }
            let result = ToolResultData(
                toolCallId: d["toolCallId"]?.stringValue ?? "",
                content: d["content"]?.stringValue ?? ""
            )
            return .toolResult(id: id, result: result, timestamp: timestamp)

        case "permission_request":
            guard let d = envelope.data else { return nil }
            var options: [PermissionOption] = []
            if case .array(let arr) = d["options"] {
                for opt in arr {
                    if case .dictionary(let dict) = opt {
                        options.append(PermissionOption(
                            optionId: dict["optionId"]?.stringValue ?? "",
                            name: dict["name"]?.stringValue ?? "",
                            kind: dict["kind"]?.stringValue ?? ""
                        ))
                    }
                }
            }
            let request = PermissionRequestData(
                requestId: d["requestId"]?.stringValue ?? "",
                title: d["title"]?.stringValue ?? "",
                kind: d["kind"]?.stringValue ?? "",
                command: d["command"]?.stringValue,
                options: options
            )
            return .permissionRequest(id: id, request: request, timestamp: timestamp)

        case "permission_resolved":
            guard let d = envelope.data else { return nil }
            return .permissionResolved(
                id: id,
                requestId: d["requestId"]?.stringValue ?? "",
                optionId: d["optionId"]?.stringValue ?? "",
                timestamp: timestamp
            )

        case "prompt_sent":
            let text = envelope.data?["text"]?.stringValue ?? envelope.text ?? ""
            return .promptSent(id: id, text: text, timestamp: timestamp)

        case "run_complete":
            let reason = envelope.data?["stopReason"]?.stringValue ?? "end_turn"
            return .runComplete(id: id, stopReason: reason, timestamp: timestamp)

        case "interrupted":
            return .interrupted(id: id, timestamp: timestamp)

        case "error":
            let msg = envelope.data?["message"]?.stringValue ?? "Unknown error"
            return .error(id: id, message: msg, timestamp: timestamp)

        default:
            return nil
        }
    }
}
