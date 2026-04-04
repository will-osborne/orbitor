import SwiftUI

struct SessionListView: View {
    @Environment(AppState.self) private var appState
    @Environment(\.theme) private var theme
    @Binding var showNewSession: Bool
    @State private var newSessionHovered = false
    @State private var sortByAttention = false
    @State private var filterGroup: String? = nil
    @State private var showGroupSheet = false
    @State private var newGroupName = ""
    @State private var groupTargetSessionID: String?

    private var displayedSessions: [SessionInfo] {
        var list = sortByAttention
            ? appState.sessionList.sessionsByAttention
            : appState.sessionList.sessions

        // Filter by group if selected
        if let group = filterGroup,
           let ids = appState.sessionGroups[group] {
            list = list.filter { ids.contains($0.id) }
        }

        return list
    }

    var body: some View {
        @Bindable var sessionList = appState.sessionList

        VStack(spacing: 0) {
            // Sort & filter controls
            HStack(spacing: 6) {
                Button {
                    sortByAttention.toggle()
                } label: {
                    HStack(spacing: 3) {
                        Image(systemName: sortByAttention ? "bell.badge.fill" : "clock")
                            .font(.caption2)
                        Text(sortByAttention ? "Priority" : "Recent")
                            .font(.caption2)
                    }
                    .foregroundStyle(sortByAttention ? theme.accent : theme.muted)
                }
                .buttonStyle(.plain)
                .help(sortByAttention ? "Sorted by attention priority" : "Sorted by creation time")

                if !appState.sessionGroups.isEmpty {
                    Picker("", selection: Binding(
                        get: { filterGroup ?? "__all__" },
                        set: { filterGroup = $0 == "__all__" ? nil : $0 }
                    )) {
                        Text("All").tag("__all__")
                        Divider()
                        ForEach(Array(appState.sessionGroups.keys).sorted(), id: \.self) { group in
                            Text(group).tag(group)
                        }
                    }
                    .pickerStyle(.menu)
                    .controlSize(.mini)
                    .frame(maxWidth: 100)
                }

                Spacer()

                // Attention summary
                let needsAttention = appState.sessionList.sessions.filter {
                    appState.sessionList.attentionKind(for: $0) != nil
                }.count
                if needsAttention > 0 {
                    HStack(spacing: 2) {
                        Image(systemName: "bell.badge.fill")
                            .font(.system(size: 9))
                        Text("\(needsAttention)")
                            .font(.caption2.bold().monospacedDigit())
                    }
                    .foregroundStyle(theme.yellow)
                }
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 6)

            Divider().background(theme.sep)

            List(selection: $sessionList.selectedSessionID) {
                ForEach(displayedSessions) { session in
                    SessionRowView(
                        session: session,
                        isUnread: appState.sessionList.unreadSessionIDs.contains(session.id),
                        attentionKind: appState.sessionList.attentionKind(for: session),
                        hasConflict: appState.sessionList.conflictingSessionIDs.contains(session.id)
                    )
                    .tag(session.id)
                    .listRowBackground(
                        session.id == appState.sessionList.selectedSessionID
                            ? theme.selBg : Color.clear
                    )
                    .contextMenu {
                        // Pin/unpin
                        if appState.pinnedSessionIDs.contains(session.id) {
                            Button("Unpin from Tab Bar") {
                                appState.pinnedSessionIDs.remove(session.id)
                            }
                        } else {
                            Button("Pin to Tab Bar") {
                                appState.pinnedSessionIDs.insert(session.id)
                            }
                        }

                        Divider()

                        // Group management
                        Menu("Add to Group") {
                            ForEach(Array(appState.sessionGroups.keys).sorted(), id: \.self) { group in
                                Button(group) {
                                    appState.sessionGroups[group]?.insert(session.id)
                                }
                            }
                            Divider()
                            Button("New Group…") {
                                groupTargetSessionID = session.id
                                newGroupName = ""
                                showGroupSheet = true
                            }
                        }

                        // Remove from current group filter
                        if let group = filterGroup,
                           appState.sessionGroups[group]?.contains(session.id) == true {
                            Button("Remove from \"\(group)\"") {
                                appState.sessionGroups[group]?.remove(session.id)
                                if appState.sessionGroups[group]?.isEmpty == true {
                                    appState.sessionGroups.removeValue(forKey: group)
                                    filterGroup = nil
                                }
                            }
                        }

                        Divider()

                        // Split view
                        if let selectedID = appState.sessionList.selectedSessionID,
                           selectedID != session.id {
                            Button("Open Split View with Selected") {
                                appState.splitLeftSessionID = selectedID
                                appState.splitRightSessionID = session.id
                            }
                        }

                        Divider()

                        Button("Delete Session", role: .destructive) {
                            Task { await appState.sessionList.deleteSession(session.id) }
                        }
                    }
                }
            }
            .listStyle(.sidebar)
            .scrollContentBackground(.hidden)
            .background(theme.panel)
        }
        .background(theme.panel)
        .safeAreaInset(edge: .bottom) {
            VStack(spacing: 0) {
                Divider()
                HStack(spacing: 8) {
                    Button {
                        showNewSession = true
                    } label: {
                        HStack(spacing: 6) {
                            Image(systemName: "plus.circle.fill")
                                .font(.body)
                                .symbolEffect(.bounce, value: newSessionHovered)
                            Text("New Session")
                                .font(.subheadline.weight(.medium))
                        }
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 6)
                        .background(theme.accent.opacity(newSessionHovered ? 0.25 : 0.15))
                        .clipShape(RoundedRectangle(cornerRadius: 6))
                        .overlay(
                            RoundedRectangle(cornerRadius: 6)
                                .strokeBorder(theme.accent.opacity(newSessionHovered ? 0.5 : 0.3), lineWidth: 1)
                        )
                    }
                    .buttonStyle(.plain)
                    .foregroundStyle(theme.accent)
                    .scaleEffect(newSessionHovered ? 1.03 : 1.0)
                    .animation(.easeOut(duration: 0.15), value: newSessionHovered)
                    .onHover { newSessionHovered = $0 }

                    if let error = appState.sessionList.error {
                        Image(systemName: "exclamationmark.triangle")
                            .foregroundStyle(theme.yellow)
                            .help(error)
                    }
                }
                .padding(.horizontal, 12)
                .padding(.vertical, 8)
            }
            .background(theme.panel)
        }
        .sheet(isPresented: $showGroupSheet) {
            VStack(spacing: 12) {
                Text("New Group")
                    .font(.headline)
                TextField("Group name", text: $newGroupName)
                    .textFieldStyle(.roundedBorder)
                HStack {
                    Button("Cancel") { showGroupSheet = false }
                    Spacer()
                    Button("Create") {
                        let name = newGroupName.trimmingCharacters(in: .whitespaces)
                        guard !name.isEmpty else { return }
                        var ids: Set<String> = []
                        if let target = groupTargetSessionID { ids.insert(target) }
                        appState.sessionGroups[name] = ids
                        showGroupSheet = false
                    }
                    .buttonStyle(.borderedProminent)
                    .disabled(newGroupName.trimmingCharacters(in: .whitespaces).isEmpty)
                }
            }
            .padding()
            .frame(width: 280)
        }
    }
}

struct SessionRowView: View {
    let session: SessionInfo
    var isUnread: Bool = false
    var attentionKind: AttentionKind? = nil
    var hasConflict: Bool = false
    @Environment(\.theme) private var theme
    @State private var isHovered = false
    @State private var runStart: Date? = nil
    @State private var elapsed: TimeInterval = 0
    private let timer = Timer.publish(every: 1, on: .main, in: .common).autoconnect()

    var body: some View {
        VStack(alignment: .leading, spacing: 3) {
            HStack {
                // Attention indicator
                if let kind = attentionKind {
                    Image(systemName: kind.icon)
                        .font(.system(size: 9))
                        .foregroundStyle(attentionColor(kind))
                }

                Text(session.displayTitle)
                    .font(.subheadline.weight(.medium))
                    .foregroundStyle(theme.text)
                    .lineLimit(1)

                if isUnread && attentionKind == nil {
                    Circle()
                        .fill(theme.accent)
                        .frame(width: 8, height: 8)
                }

                Spacer()

                // Live run timer
                if session.isRunning, elapsed > 0 {
                    Text(formatElapsed(elapsed))
                        .font(.system(size: 10, design: .monospaced))
                        .foregroundStyle(theme.orange)
                }

                StatusBadge(state: session.stateLabel)
            }

            // Current activity line
            if session.isRunning, let tool = session.currentTool, !tool.isEmpty {
                HStack(spacing: 4) {
                    Image(systemName: "gearshape")
                        .font(.system(size: 8))
                        .foregroundStyle(theme.orange)
                    Text(tool)
                        .font(.system(size: 10, design: .monospaced))
                        .foregroundStyle(theme.orange.opacity(0.8))
                        .lineLimit(1)
                }
            }

            HStack(spacing: 4) {
                Image(systemName: session.backend == "claude" ? "brain" : "chevron.left.forwardslash.chevron.right")
                    .font(.caption2)
                Text(session.shortDir)
                    .font(.caption2)
                    .lineLimit(1)

                if let model = session.model, !model.isEmpty {
                    Text("·")
                    Text(model)
                        .font(.caption2)
                        .lineLimit(1)
                }

                Spacer()

                // Conflict badge
                if hasConflict {
                    Label("conflict", systemImage: "arrow.triangle.merge")
                        .font(.system(size: 9, weight: .medium))
                        .foregroundStyle(theme.red)
                        .padding(.horizontal, 4)
                        .padding(.vertical, 1)
                        .background(theme.red.opacity(0.15))
                        .clipShape(RoundedRectangle(cornerRadius: 3))
                }

                // PR badge
                if let prUrl = session.prUrl, !prUrl.isEmpty {
                    Label("PR", systemImage: "arrow.triangle.branch")
                        .font(.system(size: 9, weight: .medium))
                        .foregroundStyle(theme.cyan)
                        .padding(.horizontal, 4)
                        .padding(.vertical, 1)
                        .background(theme.cyan.opacity(0.15))
                        .clipShape(RoundedRectangle(cornerRadius: 3))
                        .onTapGesture {
                            if let url = URL(string: prUrl) {
                                NSWorkspace.shared.open(url)
                            }
                        }
                }

                // Sub-agent count
                if let agents = session.subAgents, !agents.isEmpty {
                    let running = agents.filter { $0.status == "running" }.count
                    HStack(spacing: 2) {
                        Image(systemName: "person.2")
                            .font(.system(size: 8))
                        Text("\(running)/\(agents.count)")
                            .font(.system(size: 9).monospacedDigit())
                    }
                    .foregroundStyle(running > 0 ? theme.violet : theme.muted)
                }

                // Last-activity (session age)
                Text(relativeTime(session.createdAt))
                    .font(.caption2)
                    .foregroundStyle(theme.muted.opacity(0.7))
            }
            .foregroundStyle(theme.muted)
        }
        .padding(.vertical, 4)
        .scaleEffect(isHovered ? 1.01 : 1.0)
        .brightness(isHovered ? 0.05 : 0)
        .animation(.easeOut(duration: 0.15), value: isHovered)
        .onHover { isHovered = $0 }
        .onReceive(timer) { now in
            guard session.isRunning else { elapsed = 0; return }
            let start = runStart ?? now
            elapsed = now.timeIntervalSince(start)
        }
        .onChange(of: session.isRunning) { _, running in
            if running {
                runStart = Date()
                elapsed = 0
            } else {
                runStart = nil
                elapsed = 0
            }
        }
    }

    private func attentionColor(_ kind: AttentionKind) -> Color {
        switch kind {
        case .permission: return theme.yellow
        case .error: return theme.red
        case .conflict: return theme.red
        case .unread: return theme.accent
        }
    }

    private func formatElapsed(_ t: TimeInterval) -> String {
        let m = Int(t) / 60
        let s = Int(t) % 60
        return String(format: "%d:%02d", m, s)
    }

    private func relativeTime(_ date: Date) -> String {
        let seconds = Int(Date().timeIntervalSince(date))
        if seconds < 60 { return "just now" }
        if seconds < 3600 { return "\(seconds / 60)m ago" }
        if seconds < 86400 { return "\(seconds / 3600)h ago" }
        return "\(seconds / 86400)d ago"
    }
}
