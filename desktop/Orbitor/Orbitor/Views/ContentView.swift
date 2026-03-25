import SwiftUI

struct ContentView: View {
    @Environment(AppState.self) private var appState
    @Environment(\.theme) private var theme
    @State private var columnVisibility: NavigationSplitViewVisibility = .all
    @State private var showNewSession = false
    @State private var inspectorPresented = true

    var body: some View {
        @Bindable var sessionList = appState.sessionList

        NavigationSplitView(columnVisibility: $columnVisibility) {
            SessionListView(showNewSession: $showNewSession)
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
                EmptyStateView(showNewSession: $showNewSession)
            }
        }
        .background(theme.panel)
        .toolbar {
            ToolbarItemGroup(placement: .primaryAction) {
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
        .sheet(isPresented: $showNewSession) {
            NewSessionSheet()
        }
        .onChange(of: appState.sessionList.selectedSessionID) { _, newID in
            if let id = newID {
                appState.chat.connectToSession(id)
            }
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
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(theme.panel)
    }
}
