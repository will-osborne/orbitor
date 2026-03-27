import SwiftUI
import Combine

struct ContentView: View {
    @Environment(AppState.self) private var appState
    @Environment(\.theme) private var theme
    @State private var columnVisibility: NavigationSplitViewVisibility = .all
    @State private var inspectorPresented = true

    var body: some View {
        @Bindable var state = appState

        NavigationSplitView(columnVisibility: $columnVisibility) {
            SessionListView(showNewSession: $state.showNewSession)
                .navigationSplitViewColumnWidth(min: 200, ideal: 260, max: 360)
        } detail: {
            if appState.sessionList.selectedSessionID != nil {
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
        }
        .background(theme.panel)
        .toolbar {
            ToolbarItemGroup(placement: .primaryAction) {
                // Fork button
                if appState.sessionList.selectedSessionID != nil {
                    Button {
                        appState.showForkSheet = true
                    } label: {
                        Image(systemName: "arrow.triangle.branch")
                    }
                    .help("Fork Session (⇧⌘N)")
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
        .overlay {
            if appState.showCommandPalette {
                CommandPaletteView(isPresented: $state.showCommandPalette)
                    .transition(.opacity.combined(with: .scale(scale: 0.97)))
            }
        }
        .animation(.easeOut(duration: 0.12), value: appState.showCommandPalette)
        .safeAreaInset(edge: .bottom, spacing: 0) {
            StatusBar()
        }
        .onChange(of: appState.sessionList.selectedSessionID) { _, newID in
            if let id = newID {
                appState.chat.connectToSession(id)
                appState.sessionList.markRead(id)
            }
        }
        .onReceive(NotificationCenter.default.publisher(for: NSApplication.didBecomeActiveNotification)) { _ in
            appState.chat.isAppFocused = true
        }
        .onReceive(NotificationCenter.default.publisher(for: NSApplication.didResignActiveNotification)) { _ in
            appState.chat.isAppFocused = false
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
