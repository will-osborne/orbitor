import SwiftUI

@main
struct OrbitorApp: App {
    @State private var appState = AppState()

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environment(appState)
                .environment(\.theme, appState.currentTheme)
                .onAppear {
                    appState.sessionList.startPolling()
                    appState.reconnect()
                }
        }
        .windowStyle(.titleBar)
        .windowToolbarStyle(.unified(showsTitle: true))
        .defaultSize(width: 1200, height: 800)
        .commands {
            AppCommands(appState: appState)
        }

        Settings {
            SettingsView()
                .environment(appState)
                .environment(\.theme, appState.currentTheme)
        }
    }
}

// MARK: - Menu commands

struct AppCommands: Commands {
    let appState: AppState

    var body: some Commands {
        CommandGroup(replacing: .newItem) {
            Button("New Session") {
                appState.sessionList.selectedSessionID = nil // triggers sheet
            }
            .keyboardShortcut("n")
        }

        CommandMenu("Session") {
            Button("Interrupt") {
                Task { await appState.chat.interrupt() }
            }
            .keyboardShortcut(".", modifiers: [.command])

            Divider()

            Button("Next Session") {
                appState.sessionList.selectNext()
            }
            .keyboardShortcut("]", modifiers: [.command])

            Button("Previous Session") {
                appState.sessionList.selectPrevious()
            }
            .keyboardShortcut("[", modifiers: [.command])

            Divider()

            if let id = appState.sessionList.selectedSessionID {
                Button("Delete Session") {
                    Task { await appState.sessionList.deleteSession(id) }
                }
                .keyboardShortcut(.delete, modifiers: [.command])
            }
        }

        CommandMenu("Theme") {
            ForEach(OrbitorTheme.all) { theme in
                Button(theme.name) {
                    appState.selectedThemeID = theme.id
                }
            }
        }
    }
}

// MARK: - Settings

struct SettingsView: View {
    @Environment(AppState.self) private var appState

    var body: some View {
        @Bindable var state = appState
        Form {
            TextField("Server URL", text: $state.serverURL)
                .onSubmit { appState.reconnect() }

            Picker("Theme", selection: $state.selectedThemeID) {
                ForEach(OrbitorTheme.all) { theme in
                    Text(theme.name).tag(theme.id)
                }
            }
        }
        .padding()
        .frame(width: 400)
    }
}
