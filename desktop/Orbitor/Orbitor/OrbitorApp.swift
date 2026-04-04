import SwiftUI
import UserNotifications

final class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        let center = UNUserNotificationCenter.current()
        center.delegate = NotificationDelegate.shared
        center.requestAuthorization(options: [.alert, .sound, .badge]) { granted, error in
            if let error {
                print("[Notifications] authorization error: \(error)")
            } else if !granted {
                print("[Notifications] permission denied")
            }
        }
    }
}

@main
struct OrbitorApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) var appDelegate
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

        WindowGroup("File History", id: "file-history", for: String.self) { $sessionID in
            if let sessionID {
                RunHistoryView(sessionID: sessionID)
                    .environment(appState)
                    .environment(\.theme, appState.currentTheme)
            }
        }
        .defaultSize(width: 1100, height: 700)
        .windowStyle(.titleBar)

        // Split session view — two sessions side by side
        WindowGroup("Split View", id: "split-view") {
            SplitSessionLauncher()
                .environment(appState)
                .environment(\.theme, appState.currentTheme)
        }
        .defaultSize(width: 1400, height: 800)
        .windowStyle(.titleBar)

        Settings {
            SettingsView()
                .environment(appState)
                .environment(\.theme, appState.currentTheme)
        }
    }
}

/// Reads the split session IDs from AppState and launches the SplitSessionView.
private struct SplitSessionLauncher: View {
    @Environment(AppState.self) private var appState
    @Environment(\.theme) private var theme

    var body: some View {
        if let left = appState.splitLeftSessionID,
           let right = appState.splitRightSessionID {
            SplitSessionView(leftSessionID: left, rightSessionID: right)
        } else {
            VStack(spacing: 12) {
                Image(systemName: "rectangle.split.2x1")
                    .font(.system(size: 36))
                    .foregroundStyle(theme.muted)
                Text("Select two sessions to compare")
                    .foregroundStyle(theme.muted)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .background(theme.panel)
        }
    }
}

// MARK: - Menu commands

struct AppCommands: Commands {
    let appState: AppState

    var body: some Commands {
        CommandGroup(replacing: .newItem) {
            Button("New Session") {
                appState.showNewSession = true
            }
            .keyboardShortcut("n")

            Button("Fork Session") {
                appState.showForkSheet = true
            }
            .keyboardShortcut("n", modifiers: [.command, .shift])
            .disabled(appState.sessionList.selectedSessionID == nil)
        }

        CommandGroup(after: .newItem) {
            Button("Command Palette") {
                appState.showCommandPalette = true
            }
            .keyboardShortcut("k", modifiers: .command)
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

            // Pin/unpin current session
            if let id = appState.sessionList.selectedSessionID {
                if appState.pinnedSessionIDs.contains(id) {
                    Button("Unpin Session") {
                        appState.pinnedSessionIDs.remove(id)
                    }
                } else {
                    Button("Pin Session") {
                        appState.pinnedSessionIDs.insert(id)
                    }
                }
            }

            Divider()

            if let id = appState.sessionList.selectedSessionID {
                Button("Delete Session") {
                    Task { await appState.sessionList.deleteSession(id) }
                }
                .keyboardShortcut(.delete, modifiers: [.command])
            }
        }

        CommandMenu("View") {
            Button("Activity Dashboard") {
                appState.showActivityDashboard = true
            }
            .keyboardShortcut("d", modifiers: [.command, .shift])

            Button("Activity Feed") {
                appState.showActivityFeed = true
            }
            .keyboardShortcut("f", modifiers: [.command, .shift])

            Divider()

            Button("Increase Font Size") {
                appState.fontSize = min(appState.fontSize + 1, 28)
            }
            .keyboardShortcut("+", modifiers: .command)

            Button("Decrease Font Size") {
                appState.fontSize = max(appState.fontSize - 1, 9)
            }
            .keyboardShortcut("-", modifiers: .command)

            Button("Reset Font Size") {
                appState.fontSize = 13
            }
            .keyboardShortcut("0", modifiers: .command)
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
