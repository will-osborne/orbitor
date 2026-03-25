import Foundation

@Observable
final class SessionListState {
    var sessions: [SessionInfo] = []
    var selectedSessionID: String?
    var isLoading = false
    var error: String?

    private var api: APIClient
    private var pollingTask: Task<Void, Never>?

    init(api: APIClient) {
        self.api = api
    }

    func updateAPI(_ newAPI: APIClient) {
        self.api = newAPI
    }

    var selectedSession: SessionInfo? {
        sessions.first { $0.id == selectedSessionID }
    }

    func startPolling() {
        pollingTask?.cancel()
        pollingTask = Task { [weak self] in
            while !Task.isCancelled {
                await self?.refresh()
                try? await Task.sleep(for: .seconds(3))
            }
        }
    }

    func stopPolling() {
        pollingTask?.cancel()
        pollingTask = nil
    }

    @MainActor
    func refresh() async {
        do {
            let fetched = try await api.listSessions()
            // Preserve selection even if list order changes
            sessions = fetched.sorted { ($0.createdAt) > ($1.createdAt) }
            error = nil
            if selectedSessionID == nil, let first = sessions.first {
                selectedSessionID = first.id
            }
        } catch {
            self.error = error.localizedDescription
        }
    }

    @MainActor
    func createSession(workingDir: String, backend: String, model: String, skip: Bool = false, plan: Bool = false) async {
        do {
            let req = CreateSessionRequest(
                workingDir: workingDir, backend: backend,
                model: model, skipPermissions: skip, planMode: plan
            )
            let session = try await api.createSession(req)
            sessions.insert(session, at: 0)
            selectedSessionID = session.id
        } catch {
            self.error = error.localizedDescription
        }
    }

    @MainActor
    func deleteSession(_ id: String) async {
        do {
            try await api.deleteSession(id: id)
            sessions.removeAll { $0.id == id }
            if selectedSessionID == id {
                selectedSessionID = sessions.first?.id
            }
        } catch {
            self.error = error.localizedDescription
        }
    }

    func selectNext() {
        guard let current = selectedSessionID,
              let idx = sessions.firstIndex(where: { $0.id == current }),
              idx + 1 < sessions.count else { return }
        selectedSessionID = sessions[idx + 1].id
    }

    func selectPrevious() {
        guard let current = selectedSessionID,
              let idx = sessions.firstIndex(where: { $0.id == current }),
              idx > 0 else { return }
        selectedSessionID = sessions[idx - 1].id
    }
}
