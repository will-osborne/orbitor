import SwiftUI

struct SessionListView: View {
    @Environment(AppState.self) private var appState
    @Environment(\.theme) private var theme
    @Binding var showNewSession: Bool
    @State private var newSessionHovered = false

    var body: some View {
        @Bindable var sessionList = appState.sessionList

        List(selection: $sessionList.selectedSessionID) {
            ForEach(appState.sessionList.sessions) { session in
                SessionRowView(session: session, isUnread: appState.sessionList.unreadSessionIDs.contains(session.id))
                    .tag(session.id)
                    .listRowBackground(
                        session.id == appState.sessionList.selectedSessionID
                            ? theme.selBg : Color.clear
                    )
                    .contextMenu {
                        Button("Delete Session", role: .destructive) {
                            Task { await appState.sessionList.deleteSession(session.id) }
                        }
                    }
            }
        }
        .listStyle(.sidebar)
        .scrollContentBackground(.hidden)
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
    }
}

struct SessionRowView: View {
    let session: SessionInfo
    var isUnread: Bool = false
    @Environment(\.theme) private var theme
    @State private var isHovered = false
    @State private var runStart: Date? = nil
    @State private var elapsed: TimeInterval = 0
    private let timer = Timer.publish(every: 1, on: .main, in: .common).autoconnect()

    var body: some View {
        VStack(alignment: .leading, spacing: 3) {
            HStack {
                Text(session.displayTitle)
                    .font(.subheadline.weight(.medium))
                    .foregroundStyle(theme.text)
                    .lineLimit(1)

                if isUnread {
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
