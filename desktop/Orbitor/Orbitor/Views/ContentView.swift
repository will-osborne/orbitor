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
