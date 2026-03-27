import Foundation

struct RunRecord: Codable, Identifiable {
    let id: String
    let prompt: String
    let startedAt: Date
    let completedAt: Date?
    let files: [FileChange]
}

struct FileChange: Codable, Identifiable {
    let path: String
    let relativePath: String
    let before: String
    let after: String

    var id: String { path }
}
