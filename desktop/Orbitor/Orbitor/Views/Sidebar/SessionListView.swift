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
                SessionRowView(session: session)
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
    @Environment(\.theme) private var theme
    @State private var isHovered = false

    var body: some View {
        VStack(alignment: .leading, spacing: 3) {
            HStack {
                Text(session.displayTitle)
                    .font(.subheadline.weight(.medium))
                    .foregroundStyle(theme.text)
                    .lineLimit(1)

                Spacer()

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
            }
            .foregroundStyle(theme.muted)
        }
        .padding(.vertical, 4)
        .scaleEffect(isHovered ? 1.01 : 1.0)
        .brightness(isHovered ? 0.05 : 0)
        .animation(.easeOut(duration: 0.15), value: isHovered)
        .onHover { isHovered = $0 }
    }
}
