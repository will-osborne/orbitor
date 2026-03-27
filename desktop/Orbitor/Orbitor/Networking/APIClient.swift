import Foundation

@Observable
final class APIClient: Sendable {
    let baseURL: URL
    private let session: URLSession
    private let decoder: JSONDecoder

    init(baseURL: URL) {
        self.baseURL = baseURL
        self.session = URLSession.shared
        let dec = JSONDecoder()
        dec.dateDecodingStrategy = .custom { decoder in
            let container = try decoder.singleValueContainer()
            let str = try container.decode(String.self)
            // Try ISO 8601 with fractional seconds first, then without
            let formatters: [ISO8601DateFormatter] = {
                let f1 = ISO8601DateFormatter()
                f1.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
                let f2 = ISO8601DateFormatter()
                f2.formatOptions = [.withInternetDateTime]
                return [f1, f2]
            }()
            for f in formatters {
                if let date = f.date(from: str) { return date }
            }
            throw DecodingError.dataCorruptedError(in: container, debugDescription: "Cannot parse date: \(str)")
        }
        self.decoder = dec
    }

    // MARK: - Sessions

    func listSessions() async throws -> [SessionInfo] {
        let url = baseURL.appendingPathComponent("api/sessions")
        let (data, _) = try await session.data(from: url)
        return try decoder.decode([SessionInfo].self, from: data)
    }

    func createSession(_ req: CreateSessionRequest) async throws -> SessionInfo {
        var request = URLRequest(url: baseURL.appendingPathComponent("api/sessions"))
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try JSONEncoder().encode(req)
        let (data, _) = try await session.data(for: request)
        return try decoder.decode(SessionInfo.self, from: data)
    }

    func deleteSession(id: String) async throws {
        var request = URLRequest(url: baseURL.appendingPathComponent("api/sessions/\(id)"))
        request.httpMethod = "DELETE"
        let _ = try await session.data(for: request)
    }

    func updateSession(id: String, skipPermissions: Bool? = nil, planMode: Bool? = nil, model: String? = nil) async throws {
        var request = URLRequest(url: baseURL.appendingPathComponent("api/sessions/\(id)"))
        request.httpMethod = "PATCH"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        var body: [String: AnyCodableValue] = [:]
        if let s = skipPermissions { body["skipPermissions"] = .bool(s) }
        if let p = planMode { body["planMode"] = .bool(p) }
        if let m = model { body["model"] = .string(m) }
        request.httpBody = try JSONEncoder().encode(body)
        let _ = try await session.data(for: request)
    }

    func killSession(id: String) async throws {
        var request = URLRequest(url: baseURL.appendingPathComponent("api/sessions/\(id)/kill"))
        request.httpMethod = "POST"
        let _ = try await session.data(for: request)
    }

    func reviveSession(id: String) async throws {
        var request = URLRequest(url: baseURL.appendingPathComponent("api/sessions/\(id)/revive"))
        request.httpMethod = "POST"
        let _ = try await session.data(for: request)
    }

    func forkSession(sourceID: String, prompt: String) async throws -> SessionInfo {
        var request = URLRequest(url: baseURL.appendingPathComponent("api/sessions/\(sourceID)/clone-prompt"))
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try JSONEncoder().encode(["text": prompt])
        let (data, _) = try await session.data(for: request)
        return try decoder.decode(SessionInfo.self, from: data)
    }

    // MARK: - File browsing

    func browseDirectory(path: String? = nil) async throws -> [BrowseEntry] {
        var components = URLComponents(url: baseURL.appendingPathComponent("api/browse"), resolvingAgainstBaseURL: false)!
        if let p = path {
            components.queryItems = [URLQueryItem(name: "path", value: p)]
        }
        let (data, _) = try await session.data(from: components.url!)
        return try decoder.decode([BrowseEntry].self, from: data)
    }

    // MARK: - LLM helpers

    /// Rewrites a rough prompt into a more precise instruction.
    func enhancePrompt(_ text: String) async throws -> String {
        var request = URLRequest(url: baseURL.appendingPathComponent("api/enhance-prompt"))
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try JSONEncoder().encode(["text": text])
        let (data, _) = try await session.data(for: request)
        let resp = try JSONDecoder().decode([String: String].self, from: data)
        return resp["enhanced"] ?? text
    }

    /// Returns a post-run debrief summary for a session.
    func sessionDebrief(id: String) async throws -> String {
        let url = baseURL.appendingPathComponent("api/sessions/\(id)/debrief")
        let (data, _) = try await session.data(from: url)
        let resp = try JSONDecoder().decode([String: String].self, from: data)
        return resp["debrief"] ?? ""
    }

    /// Returns up to 3 follow-up prompt suggestions for a session.
    func sessionSuggestions(id: String) async throws -> [String] {
        let url = baseURL.appendingPathComponent("api/sessions/\(id)/suggestions")
        let (data, _) = try await session.data(from: url)
        struct Resp: Decodable { let suggestions: [String] }
        let resp = try JSONDecoder().decode(Resp.self, from: data)
        return resp.suggestions
    }

    /// Returns the per-run file change history for a session.
    func sessionRunHistory(id: String) async throws -> [RunRecord] {
        let url = baseURL.appendingPathComponent("api/sessions/\(id)/run-history")
        let (data, _) = try await session.data(from: url)
        struct Resp: Decodable { let runs: [RunRecord] }
        return try decoder.decode(Resp.self, from: data).runs
    }
}

struct BrowseEntry: Codable, Identifiable {
    let name: String
    let path: String
    let isDir: Bool

    var id: String { path }
}
