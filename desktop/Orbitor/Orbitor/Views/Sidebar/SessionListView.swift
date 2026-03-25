import SwiftUI

struct SessionListView: View {
    @Environment(AppState.self) private var appState
    @Environment(\.theme) private var theme
    @Binding var showNewSession: Bool

    var body: some View {
        @Bindable var sessionList = appState.sessionList

        List(selection: $sessionList.selectedSessionID) {
            ForEach(appState.sessionList.sessions) { session in
                SessionRowView(session: session)
                    .tag(session.id)
                    .listRowBackground(
                        session.id == appState.sessionList.selectedSessionID
                            ? theme.selBg : Color.clear
                    )
            }
        }
        .listStyle(.sidebar)
        .scrollContentBackground(.hidden)
        .background(theme.panel)
        .safeAreaInset(edge: .bottom) {
            HStack {
                Button {
                    showNewSession = true
                } label: {
                    Label("New Session", systemImage: "plus")
                }
                .buttonStyle(.borderless)
                .foregroundStyle(theme.accent)

                Spacer()

                if let error = appState.sessionList.error {
                    Image(systemName: "exclamationmark.triangle")
                        .foregroundStyle(theme.yellow)
                        .help(error)
                }
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 8)
            .background(theme.panel)
        }
    }
}

struct SessionRowView: View {
    let session: SessionInfo
    @Environment(\.theme) private var theme

    var body: some View {
        VStack(alignment: .leading, spacing: 3) {
            HStack {
                Text(session.id)
                    .font(.system(.caption, design: .monospaced))
                    .foregroundStyle(theme.accent)

                Spacer()

                StatusBadge(state: session.stateLabel)
            }

            Text(session.displayTitle)
                .font(.subheadline)
                .foregroundStyle(theme.text)
                .lineLimit(1)

            HStack(spacing: 4) {
                Image(systemName: session.backend == "claude" ? "brain" : "chevron.left.forwardslash.chevron.right")
                    .font(.caption2)
                Text(session.shortDir)
                    .font(.caption2)
                    .lineLimit(1)
            }
            .foregroundStyle(theme.muted)
        }
        .padding(.vertical, 4)
    }
}
