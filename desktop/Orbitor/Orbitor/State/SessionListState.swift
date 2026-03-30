import AppKit
import Foundation
import UserNotifications

@Observable
final class SessionListState {
    var sessions: [SessionInfo] = []
    var selectedSessionID: String?
    var isLoading = false
    var error: String?
    /// Sessions that have completed a run since the user last viewed them.
    var unreadSessionIDs: Set<String> = []

    private var api: APIClient
    private var pollingTask: Task<Void, Never>?
    /// Previous isRunning state per session, used to detect run completions.
    private var prevRunningStates: [String: Bool] = [:]

    init(api: APIClient) {
        self.api = api
    }

    func markRead(_ id: String) {
        unreadSessionIDs.remove(id)
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
            sessions = fetched.sorted { $0.createdAt > $1.createdAt }
            error = nil
            if selectedSessionID == nil, let first = sessions.first {
                selectedSessionID = first.id
            }
            detectCompletions(in: fetched)
        } catch {
            self.error = error.localizedDescription
        }
    }

    /// Detect sessions that transitioned from running → idle since the last poll
    /// and mark them unread / fire a notification.
    @MainActor
    private func detectCompletions(in fetched: [SessionInfo]) {
        for session in fetched {
            let wasRunning = prevRunningStates[session.id] ?? false
            prevRunningStates[session.id] = session.isRunning

            guard wasRunning && !session.isRunning else { continue }
            // Session just finished — mark unread if not currently selected.
            if session.id != selectedSessionID {
                unreadSessionIDs.insert(session.id)
            }
            // Skip notification only when the user is actively watching this
            // exact session. The NotificationDelegate shows banners even when
            // the app is focused, so other sessions still notify.
            if session.id == selectedSessionID && NSApp.isActive { continue }
            let content = UNMutableNotificationContent()
            content.title = "Agent Finished"
            content.body = session.displayTitle
            content.sound = .default
            let req = UNNotificationRequest(identifier: "run-\(session.id)-\(Date().timeIntervalSince1970)", content: content, trigger: nil)
            UNUserNotificationCenter.current().add(req) { err in
                if let err { print("[Notifications] \(err)") }
            }
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
    func forkSession(sourceID: String, prompt: String) async {
        do {
            let session = try await api.forkSession(sourceID: sourceID, prompt: prompt)
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
