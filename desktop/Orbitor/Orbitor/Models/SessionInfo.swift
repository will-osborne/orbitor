import Foundation

struct SessionInfo: Codable, Identifiable, Equatable {
    let id: String
    let workingDir: String
    var acpSessionId: String
    var status: String
    let backend: String
    var model: String?
    var skipPermissions: Bool
    var planMode: Bool
    var lastMessage: String?
    var currentTool: String?
    var currentPrompt: String?
    var isRunning: Bool
    var queueDepth: Int
    var pendingPermission: Bool
    var title: String?
    var summary: String?
    var prUrl: String?
    let createdAt: Date
    var subAgents: [SubAgentInfo]?

    var displayTitle: String {
        title ?? lastMessage?.prefix(60).description ?? workingDir.components(separatedBy: "/").last ?? id
    }

    var stateLabel: String {
        if isRunning { return "working" }
        if pendingPermission { return "waiting-input" }
        switch status {
        case "ready": return "idle"
        case "starting": return "starting"
        case "error": return "error"
        case "closed": return "offline"
        case "suspended": return "suspended"
        case "killed": return "killed"
        default: return status
        }
    }

    var shortDir: String {
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        if workingDir.hasPrefix(home) {
            return "~" + workingDir.dropFirst(home.count)
        }
        return workingDir
    }
}

struct SubAgentInfo: Codable, Identifiable, Equatable {
    let toolCallId: String
    let title: String
    let status: String
    let startedAt: Date

    var id: String { toolCallId }
}

struct CreateSessionRequest: Codable {
    let workingDir: String
    let backend: String
    let model: String
    let skipPermissions: Bool
    let planMode: Bool
}

struct GroupSuggestion: Codable, Identifiable {
    let name: String
    let sessionIds: [String]

    var id: String { name }
}
