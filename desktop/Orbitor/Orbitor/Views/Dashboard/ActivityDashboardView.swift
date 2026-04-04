import SwiftUI

struct ActivityDashboardView: View {
    @Environment(AppState.self) private var appState
    @Environment(\.theme) private var theme
    @Environment(\.dismiss) private var dismiss
    @State private var digestTitle = ""
    @State private var digestSummary = ""
    @State private var isLoadingDigest = false
    @State private var suggestedGroups: [GroupSuggestion] = []
    @State private var isLoadingGroups = false

    private let columns = [
        GridItem(.adaptive(minimum: 320), spacing: 16)
    ]

    var body: some View {
        VStack(spacing: 0) {
            // Title bar
            HStack {
                Text("Activity Dashboard")
                    .font(.title2.weight(.semibold))
                    .foregroundStyle(theme.text)

                Text("\(appState.sessionList.sessions.count)")
                    .font(.caption.weight(.medium))
                    .foregroundStyle(theme.muted)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 2)
                    .background(theme.muted.opacity(0.15))
                    .clipShape(Capsule())

                Spacer()

                Button {
                    Task { await appState.sessionList.refresh() }
                } label: {
                    Image(systemName: "arrow.clockwise")
                        .font(.body)
                }
                .buttonStyle(.plain)
                .foregroundStyle(theme.accent)
            }
            .padding(.horizontal, 20)
            .padding(.vertical, 14)
            .background(theme.panel)

            Divider()

            // Content
            if appState.sessionList.sessions.isEmpty {
                Spacer()
                VStack(spacing: 8) {
                    Image(systemName: "tray")
                        .font(.largeTitle)
                        .foregroundStyle(theme.muted)
                    Text("No active sessions")
                        .font(.headline)
                        .foregroundStyle(theme.muted)
                }
                Spacer()
            } else {
                ScrollView {
                    VStack(spacing: 16) {
                        // AI cross-session digest
                        if !digestTitle.isEmpty || !digestSummary.isEmpty || isLoadingDigest {
                            HStack(spacing: 10) {
                                if isLoadingDigest {
                                    ProgressView().controlSize(.small).tint(theme.violet)
                                } else {
                                    Image(systemName: "sparkles")
                                        .font(.callout)
                                        .foregroundStyle(theme.violet)
                                }
                                VStack(alignment: .leading, spacing: 2) {
                                    if !digestTitle.isEmpty {
                                        Text(digestTitle)
                                            .font(.subheadline.weight(.semibold))
                                            .foregroundStyle(theme.text)
                                    }
                                    if !digestSummary.isEmpty {
                                        Text(digestSummary)
                                            .font(.caption)
                                            .foregroundStyle(theme.muted)
                                    }
                                }
                                Spacer()
                            }
                            .padding(12)
                            .background(theme.violet.opacity(0.08))
                            .clipShape(RoundedRectangle(cornerRadius: 8))
                            .overlay(
                                RoundedRectangle(cornerRadius: 8)
                                    .strokeBorder(theme.violet.opacity(0.2), lineWidth: 1)
                            )
                        }

                        // AI-suggested groups
                        if !suggestedGroups.isEmpty {
                            VStack(alignment: .leading, spacing: 6) {
                                HStack(spacing: 4) {
                                    Image(systemName: "sparkles")
                                        .font(.caption2)
                                        .foregroundStyle(theme.violet)
                                    Text("Suggested Groups")
                                        .font(.caption2.bold())
                                        .foregroundStyle(theme.muted)
                                }

                                ScrollView(.horizontal, showsIndicators: false) {
                                    HStack(spacing: 8) {
                                        ForEach(suggestedGroups) { group in
                                            Button {
                                                appState.sessionGroups[group.name] = Set(group.sessionIds)
                                            } label: {
                                                HStack(spacing: 4) {
                                                    Text(group.name)
                                                        .font(.caption)
                                                        .foregroundStyle(theme.text)
                                                    Text("\(group.sessionIds.count)")
                                                        .font(.caption2.monospacedDigit())
                                                        .foregroundStyle(theme.muted)
                                                }
                                                .padding(.horizontal, 10)
                                                .padding(.vertical, 5)
                                                .background(theme.violet.opacity(0.12))
                                                .clipShape(RoundedRectangle(cornerRadius: 6))
                                                .overlay(
                                                    RoundedRectangle(cornerRadius: 6)
                                                        .strokeBorder(theme.violet.opacity(0.3), lineWidth: 1)
                                                )
                                            }
                                            .buttonStyle(.plain)
                                        }
                                    }
                                }
                            }
                        }

                        // Session grid
                        LazyVGrid(columns: columns, spacing: 16) {
                            ForEach(appState.sessionList.sessions) { session in
                                SessionCard(
                                    session: session,
                                    isUnread: appState.sessionList.unreadSessionIDs.contains(session.id)
                                )
                                .onTapGesture {
                                    appState.sessionList.selectedSessionID = session.id
                                    dismiss()
                                }
                            }
                        }
                    }
                    .padding(20)
                }
            }
        }
        .frame(minWidth: 700, minHeight: 500)
        .onAppear {
            loadDigest()
            loadGroupSuggestions()
        }
    }

    private func loadDigest() {
        guard appState.sessionList.sessions.count >= 2 else { return }
        isLoadingDigest = true
        Task {
            let result = try? await appState.api.missionSummary()
            await MainActor.run {
                digestTitle = result?.title ?? ""
                digestSummary = result?.summary ?? ""
                isLoadingDigest = false
            }
        }
    }

    private func loadGroupSuggestions() {
        guard appState.sessionList.sessions.count >= 2 else { return }
        isLoadingGroups = true
        Task {
            let groups = try? await appState.api.groupSuggestions()
            await MainActor.run {
                suggestedGroups = groups ?? []
                isLoadingGroups = false
            }
        }
    }
}

// MARK: - Session Card

private struct SessionCard: View {
    let session: SessionInfo
    let isUnread: Bool
    @Environment(\.theme) private var theme
    @State private var isHovered = false

    private var borderColor: Color {
        if session.pendingPermission { return theme.yellow }
        if session.stateLabel == "error" { return theme.red }
        if session.isRunning { return theme.accent }
        return theme.border
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            // Header: title + status badge
            HStack {
                Text(session.displayTitle)
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(theme.text)
                    .lineLimit(1)

                Spacer()

                StatusBadge(state: session.stateLabel)
            }

            // Current activity
            HStack(spacing: 6) {
                if session.stateLabel == "waiting-input" || session.pendingPermission {
                    Image(systemName: "hand.raised.fill")
                        .font(.caption2)
                        .foregroundStyle(theme.yellow)
                    Text("Needs permission")
                        .font(.caption)
                        .foregroundStyle(theme.yellow)
                } else if session.isRunning, let tool = session.currentTool, !tool.isEmpty {
                    Image(systemName: "gearshape.fill")
                        .font(.caption2)
                        .foregroundStyle(theme.orange)
                    Text(tool)
                        .font(.system(.caption, design: .monospaced))
                        .foregroundStyle(theme.orange)
                        .lineLimit(1)
                } else {
                    Image(systemName: "moon.fill")
                        .font(.caption2)
                        .foregroundStyle(theme.gray)
                    Text("Idle")
                        .font(.caption)
                        .foregroundStyle(theme.gray)
                }
            }

            // Current prompt (if running)
            if session.isRunning, let prompt = session.currentPrompt, !prompt.isEmpty {
                Text(prompt)
                    .font(.caption)
                    .foregroundStyle(theme.muted)
                    .lineLimit(2)
            }

            Divider()

            // Stats row
            HStack(spacing: 8) {
                Label(session.shortDir, systemImage: "folder")
                    .font(.caption2)
                    .foregroundStyle(theme.muted)
                    .lineLimit(1)

                if let model = session.model, !model.isEmpty {
                    Text("·")
                        .foregroundStyle(theme.muted)
                    Text(model)
                        .font(.caption2)
                        .foregroundStyle(theme.muted)
                        .lineLimit(1)
                }

                if session.queueDepth > 0 {
                    Text("·")
                        .foregroundStyle(theme.muted)
                    Label("\(session.queueDepth) queued", systemImage: "tray.full")
                        .font(.caption2)
                        .foregroundStyle(theme.cyan)
                }

                Spacer()
            }

            // Sub-agents
            if let agents = session.subAgents, !agents.isEmpty {
                HStack(spacing: 4) {
                    Image(systemName: "person.2.fill")
                        .font(.caption2)
                        .foregroundStyle(theme.violet)
                    Text("\(agents.count) sub-agent\(agents.count == 1 ? "" : "s")")
                        .font(.caption2)
                        .foregroundStyle(theme.violet)

                    let running = agents.filter { $0.status == "running" }.count
                    if running > 0 {
                        Text("(\(running) active)")
                            .font(.caption2)
                            .foregroundStyle(theme.orange)
                    }
                }
            }

            // Footer: created time + unread badge
            HStack {
                Text(relativeTime(session.createdAt))
                    .font(.caption2)
                    .foregroundStyle(theme.muted.opacity(0.7))

                Spacer()

                if isUnread {
                    Text("unread")
                        .font(.system(size: 9, weight: .medium))
                        .foregroundStyle(theme.accent)
                        .padding(.horizontal, 5)
                        .padding(.vertical, 2)
                        .background(theme.accent.opacity(0.15))
                        .clipShape(Capsule())
                }
            }
        }
        .padding(14)
        .background(theme.panel)
        .clipShape(RoundedRectangle(cornerRadius: 10))
        .overlay(
            RoundedRectangle(cornerRadius: 10)
                .strokeBorder(borderColor.opacity(isHovered ? 0.8 : 0.5), lineWidth: 1)
        )
        .shadow(
            color: session.isRunning ? theme.accent.opacity(0.15) : Color.black.opacity(0.1),
            radius: session.isRunning ? 8 : 3,
            y: 2
        )
        .scaleEffect(isHovered ? 1.02 : 1.0)
        .animation(.easeOut(duration: 0.15), value: isHovered)
        .onHover { isHovered = $0 }
        .contentShape(Rectangle())
    }

    private func relativeTime(_ date: Date) -> String {
        let seconds = Int(Date().timeIntervalSince(date))
        if seconds < 60 { return "just now" }
        if seconds < 3600 { return "\(seconds / 60)m ago" }
        if seconds < 86400 { return "\(seconds / 3600)h ago" }
        return "\(seconds / 86400)d ago"
    }
}
