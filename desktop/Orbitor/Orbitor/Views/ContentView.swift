import SwiftUI
import Combine

struct ContentView: View {
    @Environment(AppState.self) private var appState
    @Environment(\.theme) private var theme
    @Environment(\.openWindow) private var openWindow
    @State private var columnVisibility: NavigationSplitViewVisibility = .all
    @State private var inspectorPresented = true
    @State private var switchSummary: SessionSwitchSummary? = nil
    @State private var previousSessionID: String? = nil

    var body: some View {
        @Bindable var state = appState

        NavigationSplitView(columnVisibility: $columnVisibility) {
            SessionListView(showNewSession: $state.showNewSession)
                .navigationSplitViewColumnWidth(min: 200, ideal: 260, max: 360)
        } detail: {
            VStack(spacing: 0) {
                // Pinned session tab bar
                if !appState.pinnedSessionIDs.isEmpty {
                    SessionTabBar()
                }

                if appState.sessionList.selectedSessionID != nil {
                    // Session switch summary banner
                    if let summary = switchSummary {
                        SessionSwitchBanner(summary: summary, onDismiss: {
                            withAnimation(.easeOut(duration: 0.15)) {
                                switchSummary = nil
                            }
                        })
                    }

                    HStack(spacing: 0) {
                        ChatView()

                        if inspectorPresented {
                            Divider()
                            InspectorView()
                                .frame(width: 280)
                        }
                    }
                } else {
                    EmptyStateView(showNewSession: $state.showNewSession)
                }

                StatusBar()
            }
        }
        .background(theme.panel)
        .toolbar {
            ToolbarItemGroup(placement: .primaryAction) {
                // Activity Dashboard
                Button {
                    appState.showActivityDashboard = true
                } label: {
                    Image(systemName: "square.grid.2x2")
                }
                .help("Activity Dashboard")

                // Activity Feed
                Button {
                    appState.showActivityFeed = true
                } label: {
                    Image(systemName: "list.bullet.rectangle")
                }
                .help("Activity Feed")

                // Fork button
                if appState.sessionList.selectedSessionID != nil {
                    Button {
                        appState.showForkSheet = true
                    } label: {
                        Image(systemName: "arrow.triangle.branch")
                    }
                    .help("Fork Session (⇧⌘N)")
                }

                // Split view
                if appState.sessionList.sessions.count >= 2 {
                    Button {
                        let sessions = appState.sessionList.sessions
                        let selected = appState.sessionList.selectedSessionID ?? sessions.first?.id ?? ""
                        let other = sessions.first { $0.id != selected }?.id ?? selected
                        appState.splitLeftSessionID = selected
                        appState.splitRightSessionID = other
                    } label: {
                        Image(systemName: "rectangle.split.2x1")
                    }
                    .help("Split View — two sessions side by side")
                }

                Button {
                    inspectorPresented.toggle()
                } label: {
                    Image(systemName: "sidebar.right")
                }
                .help("Toggle Inspector")

                Picker("Theme", selection: Binding(
                    get: { appState.selectedThemeID },
                    set: { appState.selectedThemeID = $0 }
                )) {
                    ForEach(OrbitorTheme.all) { theme in
                        Text(theme.name).tag(theme.id)
                    }
                }
                .pickerStyle(.menu)
                .frame(width: 120)
            }
        }
        .sheet(isPresented: $state.showNewSession) {
            NewSessionSheet()
        }
        .sheet(isPresented: $state.showForkSheet) {
            ForkSessionSheet()
        }
        .sheet(isPresented: $state.showActivityDashboard) {
            ActivityDashboardView()
                .frame(minWidth: 700, minHeight: 500)
        }
        .sheet(isPresented: $state.showActivityFeed) {
            ActivityFeedView()
                .frame(minWidth: 500, minHeight: 400)
        }
        .overlay {
            if appState.showCommandPalette {
                CommandPaletteView(isPresented: $state.showCommandPalette)
                    .transition(.opacity.combined(with: .scale(scale: 0.97)))
            }
        }
        .animation(.easeOut(duration: 0.12), value: appState.showCommandPalette)
        .onChange(of: appState.sessionList.selectedSessionID) { oldID, newID in
            previousSessionID = oldID
            if let id = newID {
                appState.chat.connectToSession(id)
                appState.sessionList.markRead(id)

                // Auto-pin when selected
                appState.pinnedSessionIDs.insert(id)

                // Show switch summary if coming from a different session
                if oldID != nil && oldID != id {
                    loadSwitchSummary(for: id)
                }
            }
        }
        .onReceive(NotificationCenter.default.publisher(for: NSApplication.didBecomeActiveNotification)) { _ in
            appState.chat.isAppFocused = true
        }
        .onReceive(NotificationCenter.default.publisher(for: NSApplication.didResignActiveNotification)) { _ in
            appState.chat.isAppFocused = false
        }
    }

    // MARK: - Session switch summary

    private func loadSwitchSummary(for sessionID: String) {
        let session = appState.sessionList.sessions.first { $0.id == sessionID }
        guard let session else { return }

        let summary = SessionSwitchSummary(
            sessionTitle: session.displayTitle,
            isRunning: session.isRunning,
            currentTool: session.currentTool,
            pendingPermission: session.pendingPermission,
            queueDepth: session.queueDepth,
            subAgentCount: session.subAgents?.count ?? 0,
            runningSubAgents: session.subAgents?.filter { $0.status == "running" }.count ?? 0
        )
        withAnimation(.easeOut(duration: 0.15)) {
            switchSummary = summary
        }

        // Auto-dismiss after 4 seconds
        Task {
            try? await Task.sleep(for: .seconds(4))
            await MainActor.run {
                withAnimation(.easeOut(duration: 0.15)) {
                    switchSummary = nil
                }
            }
        }
    }
}

// MARK: - Session Switch Summary

struct SessionSwitchSummary {
    let sessionTitle: String
    let isRunning: Bool
    let currentTool: String?
    let pendingPermission: Bool
    let queueDepth: Int
    let subAgentCount: Int
    let runningSubAgents: Int
}

private struct SessionSwitchBanner: View {
    let summary: SessionSwitchSummary
    let onDismiss: () -> Void
    @Environment(\.theme) private var theme

    var body: some View {
        HStack(spacing: 10) {
            Image(systemName: "arrow.right.circle.fill")
                .font(.callout)
                .foregroundStyle(theme.accent)

            VStack(alignment: .leading, spacing: 2) {
                Text(summary.sessionTitle)
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(theme.text)
                    .lineLimit(1)

                HStack(spacing: 8) {
                    if summary.isRunning {
                        Label(summary.currentTool ?? "Working…", systemImage: "gearshape")
                            .font(.caption2)
                            .foregroundStyle(theme.orange)
                    } else if summary.pendingPermission {
                        Label("Needs permission", systemImage: "exclamationmark.shield")
                            .font(.caption2)
                            .foregroundStyle(theme.yellow)
                    } else {
                        Label("Idle", systemImage: "checkmark.circle")
                            .font(.caption2)
                            .foregroundStyle(theme.green)
                    }

                    if summary.queueDepth > 0 {
                        Text("\(summary.queueDepth) queued")
                            .font(.caption2)
                            .foregroundStyle(theme.muted)
                    }

                    if summary.subAgentCount > 0 {
                        Text("\(summary.runningSubAgents)/\(summary.subAgentCount) sub-agents")
                            .font(.caption2)
                            .foregroundStyle(theme.violet)
                    }
                }
            }

            Spacer()

            Button {
                onDismiss()
            } label: {
                Image(systemName: "xmark")
                    .font(.caption2)
                    .foregroundStyle(theme.muted)
            }
            .buttonStyle(.plain)
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 6)
        .background(theme.accent.opacity(0.08))
        .overlay(alignment: .bottom) {
            Divider().background(theme.sep)
        }
    }
}

// MARK: - Status Bar

private struct StatusBar: View {
    @Environment(AppState.self) private var appState
    @Environment(\.theme) private var theme

    private var runningSessions: Int {
        appState.sessionList.sessions.filter { $0.isRunning }.count
    }

    private var permissionSessions: Int {
        appState.sessionList.sessions.filter { $0.pendingPermission }.count
    }

    private var serverLabel: String {
        switch appState.connectionStatus {
        case .connected: return "Connected"
        case .connecting: return "Connecting…"
        case .disconnected: return "Disconnected"
        case .error(let msg): return "Error: \(msg)"
        }
    }

    private var serverColor: Color {
        switch appState.connectionStatus {
        case .connected: return theme.green
        case .connecting: return theme.yellow
        case .disconnected, .error: return theme.red
        }
    }

    var body: some View {
        HStack(spacing: 10) {
            // Server status
            HStack(spacing: 4) {
                Circle()
                    .fill(serverColor)
                    .frame(width: 6, height: 6)
                Text(serverLabel)
                    .font(.system(size: 10))
                    .foregroundStyle(theme.muted)
            }

            Divider().frame(height: 12)

            // Session count
            HStack(spacing: 4) {
                Image(systemName: "terminal")
                    .font(.system(size: 9))
                    .foregroundStyle(theme.muted)
                Text("\(appState.sessionList.sessions.count) session\(appState.sessionList.sessions.count == 1 ? "" : "s")")
                    .font(.system(size: 10))
                    .foregroundStyle(theme.muted)
            }

            // Running count (shown when any are running)
            if runningSessions > 0 {
                Divider().frame(height: 12)
                HStack(spacing: 4) {
                    Circle()
                        .fill(theme.orange)
                        .frame(width: 6, height: 6)
                    Text("\(runningSessions) running")
                        .font(.system(size: 10))
                        .foregroundStyle(theme.orange)
                }
            }

            // Permission count
            if permissionSessions > 0 {
                Divider().frame(height: 12)
                HStack(spacing: 4) {
                    Image(systemName: "exclamationmark.shield.fill")
                        .font(.system(size: 9))
                    Text("\(permissionSessions) need attention")
                        .font(.system(size: 10))
                }
                .foregroundStyle(theme.yellow)
            }

            // Conflict indicator
            if !appState.sessionList.conflictingSessionIDs.isEmpty {
                Divider().frame(height: 12)
                HStack(spacing: 4) {
                    Image(systemName: "arrow.triangle.merge")
                        .font(.system(size: 9))
                    Text("\(appState.sessionList.fileConflicts.count) conflict\(appState.sessionList.fileConflicts.count == 1 ? "" : "s")")
                        .font(.system(size: 10))
                }
                .foregroundStyle(theme.red)
            }

            Spacer()

            // Server URL (right-aligned)
            Text(appState.serverURL)
                .font(.system(size: 9, design: .monospaced))
                .foregroundStyle(theme.muted.opacity(0.5))
                .lineLimit(1)
                .truncationMode(.middle)
                .frame(maxWidth: 200)
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 4)
        .background(theme.panel)
        .overlay(alignment: .top) {
            Divider().background(theme.sep)
        }
    }
}

struct EmptyStateView: View {
    @Binding var showNewSession: Bool
    @Environment(\.theme) private var theme

    var body: some View {
        VStack(spacing: 16) {
            Image(systemName: "terminal")
                .font(.system(size: 48))
                .foregroundStyle(theme.muted)
            Text("No session selected")
                .font(.title2)
                .foregroundStyle(theme.muted)
            Button("New Session") {
                showNewSession = true
            }
            .keyboardShortcut("n")
            .buttonStyle(.borderedProminent)
            .tint(theme.accent)
            .hoverGlow()
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(theme.panel)
    }
}
