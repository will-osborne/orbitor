import SwiftUI

// MARK: - Model

struct ActivityEvent: Identifiable {
    let id = UUID()
    let timestamp: Date
    let sessionID: String
    let sessionTitle: String
    let kind: EventKind

    enum EventKind {
        case started
        case completed
        case error
        case permissionNeeded
        case permissionCleared
        case toolChanged(String)
        case subAgentStarted(String)
        case subAgentCompleted(String)
    }
}

private struct SessionSnapshot {
    let isRunning: Bool
    let pendingPermission: Bool
    let status: String
    let currentTool: String?
    let subAgentIDs: Set<String>
}

// MARK: - View

struct ActivityFeedView: View {
    @Environment(AppState.self) private var appState
    @Environment(\.theme) private var theme
    @Environment(\.dismiss) private var dismiss

    @State private var events: [ActivityEvent] = []
    @State private var prevStates: [String: SessionSnapshot] = [:]

    private let maxEvents = 200

    var body: some View {
        VStack(spacing: 0) {
            titleBar
            Divider().background(theme.sep)

            if events.isEmpty {
                emptyState
            } else {
                timeline
            }
        }
        .background(theme.panel)
        .frame(minWidth: 400, minHeight: 300)
        .onChange(of: appState.sessionList.sessions) { _, newSessions in
            processChanges(newSessions)
        }
    }

    // MARK: - Title Bar

    private var titleBar: some View {
        HStack {
            Image(systemName: "list.bullet.below.rectangle")
                .foregroundStyle(theme.accent)
            Text("Activity Feed")
                .font(.headline)
                .foregroundStyle(theme.text)

            if !events.isEmpty {
                Text("\(events.count)")
                    .font(.caption2.bold().monospacedDigit())
                    .foregroundStyle(theme.panel)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 2)
                    .background(theme.accent, in: Capsule())
            }

            Spacer()

            if !events.isEmpty {
                Button {
                    events.removeAll()
                } label: {
                    HStack(spacing: 4) {
                        Image(systemName: "trash")
                        Text("Clear")
                    }
                    .font(.caption)
                    .foregroundStyle(theme.muted)
                }
                .buttonStyle(.plain)
            }
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 12)
    }

    // MARK: - Empty State

    private var emptyState: some View {
        VStack(spacing: 12) {
            Image(systemName: "clock.badge.xmark")
                .font(.system(size: 36))
                .foregroundStyle(theme.muted)
            Text("No activity yet")
                .font(.headline)
                .foregroundStyle(theme.muted)
            Text("Events will appear as sessions run.")
                .font(.caption)
                .foregroundStyle(theme.muted.opacity(0.7))
                .multilineTextAlignment(.center)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    // MARK: - Timeline

    private var timeline: some View {
        ScrollView {
            LazyVStack(spacing: 0) {
                ForEach(events) { event in
                    eventRow(event)
                        .contentShape(Rectangle())
                        .onTapGesture {
                            appState.sessionList.selectedSessionID = event.sessionID
                        }
                }
            }
        }
    }

    private func eventRow(_ event: ActivityEvent) -> some View {
        HStack(spacing: 10) {
            Circle()
                .fill(dotColor(for: event.kind))
                .frame(width: 8, height: 8)

            VStack(alignment: .leading, spacing: 2) {
                Text(eventDescription(event.kind))
                    .font(.caption)
                    .foregroundStyle(theme.text)
                Text(event.sessionTitle)
                    .font(.caption2)
                    .foregroundStyle(theme.muted)
                    .lineLimit(1)
            }

            Spacer()

            Text(event.timestamp, style: .relative)
                .font(.caption2)
                .foregroundStyle(theme.muted)
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 8)
        .background(
            event.sessionID == appState.sessionList.selectedSessionID
                ? theme.selBg.opacity(0.5)
                : Color.clear
        )
    }

    // MARK: - Event Helpers

    private func dotColor(for kind: ActivityEvent.EventKind) -> Color {
        switch kind {
        case .started:                return theme.orange
        case .completed:              return theme.green
        case .error:                  return theme.red
        case .permissionNeeded:       return theme.yellow
        case .permissionCleared:      return theme.yellow
        case .toolChanged:            return theme.cyan
        case .subAgentStarted:        return theme.violet
        case .subAgentCompleted:      return theme.violet
        }
    }

    private func eventDescription(_ kind: ActivityEvent.EventKind) -> String {
        switch kind {
        case .started:                       return "Session started running"
        case .completed:                     return "Session completed"
        case .error:                         return "Session entered error state"
        case .permissionNeeded:              return "Permission needed"
        case .permissionCleared:             return "Permission resolved"
        case .toolChanged(let tool):         return "Tool changed to \(tool)"
        case .subAgentStarted(let title):    return "Sub-agent started: \(title)"
        case .subAgentCompleted(let title):  return "Sub-agent completed: \(title)"
        }
    }

    // MARK: - Change Detection

    private func processChanges(_ sessions: [SessionInfo]) {
        let now = Date()
        var newEvents: [ActivityEvent] = []

        for session in sessions {
            let current = SessionSnapshot(
                isRunning: session.isRunning,
                pendingPermission: session.pendingPermission,
                status: session.status,
                currentTool: session.currentTool,
                subAgentIDs: Set((session.subAgents ?? []).map(\.toolCallId))
            )

            if let prev = prevStates[session.id] {
                let title = session.displayTitle

                // Started running
                if !prev.isRunning && current.isRunning {
                    newEvents.append(ActivityEvent(
                        timestamp: now, sessionID: session.id,
                        sessionTitle: title, kind: .started
                    ))
                }

                // Completed running
                if prev.isRunning && !current.isRunning {
                    newEvents.append(ActivityEvent(
                        timestamp: now, sessionID: session.id,
                        sessionTitle: title, kind: .completed
                    ))
                }

                // Error state
                if current.status == "error" && prev.status != "error" {
                    newEvents.append(ActivityEvent(
                        timestamp: now, sessionID: session.id,
                        sessionTitle: title, kind: .error
                    ))
                }

                // Permission needed
                if current.pendingPermission && !prev.pendingPermission {
                    newEvents.append(ActivityEvent(
                        timestamp: now, sessionID: session.id,
                        sessionTitle: title, kind: .permissionNeeded
                    ))
                }

                // Permission cleared
                if !current.pendingPermission && prev.pendingPermission {
                    newEvents.append(ActivityEvent(
                        timestamp: now, sessionID: session.id,
                        sessionTitle: title, kind: .permissionCleared
                    ))
                }

                // Tool changed
                if let tool = current.currentTool, tool != prev.currentTool {
                    newEvents.append(ActivityEvent(
                        timestamp: now, sessionID: session.id,
                        sessionTitle: title, kind: .toolChanged(tool)
                    ))
                }

                // New sub-agents
                let newAgentIDs = current.subAgentIDs.subtracting(prev.subAgentIDs)
                for agent in (session.subAgents ?? []) where newAgentIDs.contains(agent.toolCallId) {
                    newEvents.append(ActivityEvent(
                        timestamp: now, sessionID: session.id,
                        sessionTitle: title, kind: .subAgentStarted(agent.title)
                    ))
                }

                // Completed sub-agents (were present before, now gone)
                let removedAgentIDs = prev.subAgentIDs.subtracting(current.subAgentIDs)
                if !removedAgentIDs.isEmpty {
                    // We don't have the old agent titles, so use a generic label
                    for _ in removedAgentIDs {
                        newEvents.append(ActivityEvent(
                            timestamp: now, sessionID: session.id,
                            sessionTitle: title, kind: .subAgentCompleted("sub-agent")
                        ))
                    }
                }
            }

            prevStates[session.id] = current
        }

        if !newEvents.isEmpty {
            events.insert(contentsOf: newEvents, at: 0)
            if events.count > maxEvents {
                events = Array(events.prefix(maxEvents))
            }
        }
    }
}
