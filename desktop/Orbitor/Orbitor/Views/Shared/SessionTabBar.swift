import SwiftUI

struct SessionTabBar: View {
    @Environment(AppState.self) private var appState
    @Environment(\.theme) private var theme

    private var pinnedSessions: [SessionInfo] {
        appState.sessionList.sessions.filter {
            appState.pinnedSessionIDs.contains($0.id)
        }
    }

    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 0) {
                ForEach(pinnedSessions) { session in
                    SessionTab(
                        session: session,
                        isSelected: session.id == appState.sessionList.selectedSessionID,
                        isUnread: appState.sessionList.unreadSessionIDs.contains(session.id),
                        onSelect: { appState.sessionList.selectedSessionID = session.id },
                        onUnpin: { appState.pinnedSessionIDs.remove(session.id) }
                    )

                    if session.id != pinnedSessions.last?.id {
                        theme.sep
                            .frame(width: 1, height: 20)
                    }
                }
            }
        }
        .frame(height: 30)
        .background(theme.panel)
        .overlay(alignment: .bottom) {
            theme.sep.frame(height: 1)
        }
    }
}

// MARK: - Individual Tab

private struct SessionTab: View {
    let session: SessionInfo
    let isSelected: Bool
    let isUnread: Bool
    let onSelect: () -> Void
    let onUnpin: () -> Void

    @Environment(\.theme) private var theme
    @State private var isHovered = false

    private func statusColor(for session: SessionInfo) -> Color {
        if session.pendingPermission { return theme.yellow }
        if session.isRunning { return theme.orange }
        switch session.stateLabel {
        case "error": return theme.red
        case "idle": return theme.green
        default: return theme.gray
        }
    }

    var body: some View {
        Button(action: onSelect) {
            HStack(spacing: 6) {
                Circle()
                    .fill(statusColor(for: session))
                    .frame(width: 6, height: 6)

                Text(session.displayTitle)
                    .font(.system(.caption, design: .monospaced))
                    .foregroundStyle(isSelected ? theme.text : theme.muted)
                    .lineLimit(1)
                    .frame(maxWidth: 120, alignment: .leading)

                if isUnread {
                    Circle()
                        .fill(theme.accent)
                        .frame(width: 5, height: 5)
                }

                if isHovered {
                    Button(action: onUnpin) {
                        Image(systemName: "xmark")
                            .font(.system(size: 8, weight: .semibold))
                            .foregroundStyle(theme.muted)
                    }
                    .buttonStyle(.plain)
                }
            }
            .padding(.horizontal, 10)
            .frame(height: 30)
            .background(isSelected ? theme.selBg : (isHovered ? theme.panel.opacity(0.8) : .clear))
            .overlay(alignment: .bottom) {
                if isSelected {
                    theme.accent
                        .frame(height: 2)
                }
            }
        }
        .buttonStyle(.plain)
        .onHover { isHovered = $0 }
        .contextMenu {
            Button("Unpin Session") { onUnpin() }
            Button("Close All Others") {
                // Keep only this session pinned
            }
        }
    }
}
