import Foundation

// MARK: - Raw WebSocket message envelope

struct WSEnvelope: Codable {
    let type: String
    let data: AnyCodableValue?
    let messages: [WSEnvelope]?  // only for "history" type
    let text: String?            // only for "prompt" type
}

// MARK: - Parsed message types

enum ChatMessage: Identifiable {
    case agentText(id: UUID, text: String, timestamp: Date)
    case toolCall(id: UUID, call: ToolCallData, timestamp: Date)
    case toolResult(id: UUID, result: ToolResultData, timestamp: Date)
    case permissionRequest(id: UUID, request: PermissionRequestData, timestamp: Date)
    case permissionResolved(id: UUID, requestId: String, optionId: String, timestamp: Date)
    case promptSent(id: UUID, text: String, timestamp: Date)
    case runComplete(id: UUID, stopReason: String, timestamp: Date)
    case interrupted(id: UUID, timestamp: Date)
    case error(id: UUID, message: String, timestamp: Date)
    /// Bulk history load — all messages arrive at once to avoid per-message re-renders.
    case historyBatch(id: UUID, messages: [ChatMessage], timestamp: Date)

    var id: UUID {
        switch self {
        case .agentText(let id, _, _),
             .toolCall(let id, _, _),
             .toolResult(let id, _, _),
             .permissionRequest(let id, _, _),
             .permissionResolved(let id, _, _, _),
             .promptSent(let id, _, _),
             .runComplete(let id, _, _),
             .interrupted(let id, _),
             .error(let id, _, _),
             .historyBatch(let id, _, _):
            return id
        }
    }

    var timestamp: Date {
        switch self {
        case .agentText(_, _, let t),
             .toolCall(_, _, let t),
             .toolResult(_, _, let t),
             .permissionRequest(_, _, let t),
             .permissionResolved(_, _, _, let t),
             .promptSent(_, _, let t),
             .runComplete(_, _, let t),
             .interrupted(_, let t),
             .error(_, _, let t),
             .historyBatch(_, _, let t):
            return t
        }
    }

    var isUserMessage: Bool {
        if case .promptSent = self { return true }
        return false
    }
}

struct ToolCallData: Codable {
    let toolCallId: String
    var title: String
    var kind: String
    var status: String
    var content: String?
}

struct ToolResultData: Codable {
    let toolCallId: String
    let content: String
}

struct PermissionRequestData: Codable, Identifiable {
    let requestId: String
    let title: String
    let kind: String
    let command: String?
    let options: [PermissionOption]

    var id: String { requestId }
}

struct PermissionOption: Codable, Identifiable {
    let optionId: String
    let name: String
    let kind: String

    var id: String { optionId }
}

// MARK: - Client → Server messages

struct WSPromptMessage: Codable {
    let type = "prompt"
    let text: String
}

struct WSInterruptMessage: Codable {
    let type = "interrupt"
}

struct WSPermissionResponseMessage: Codable {
    let type = "permission_response"
    let requestId: String
    let optionId: String
}

// MARK: - AnyCodableValue for flexible JSON parsing

enum AnyCodableValue: Codable {
    case string(String)
    case int(Int)
    case double(Double)
    case bool(Bool)
    case dictionary([String: AnyCodableValue])
    case array([AnyCodableValue])
    case null

    init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        if container.decodeNil() {
            self = .null
        } else if let v = try? container.decode(Bool.self) {
            self = .bool(v)
        } else if let v = try? container.decode(Int.self) {
            self = .int(v)
        } else if let v = try? container.decode(Double.self) {
            self = .double(v)
        } else if let v = try? container.decode(String.self) {
            self = .string(v)
        } else if let v = try? container.decode([String: AnyCodableValue].self) {
            self = .dictionary(v)
        } else if let v = try? container.decode([AnyCodableValue].self) {
            self = .array(v)
        } else {
            self = .null
        }
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.singleValueContainer()
        switch self {
        case .string(let v): try container.encode(v)
        case .int(let v): try container.encode(v)
        case .double(let v): try container.encode(v)
        case .bool(let v): try container.encode(v)
        case .dictionary(let v): try container.encode(v)
        case .array(let v): try container.encode(v)
        case .null: try container.encodeNil()
        }
    }

    var stringValue: String? {
        if case .string(let v) = self { return v }
        return nil
    }

    var intValue: Int? {
        if case .int(let v) = self { return v }
        return nil
    }

    subscript(key: String) -> AnyCodableValue? {
        if case .dictionary(let d) = self { return d[key] }
        return nil
    }
}
