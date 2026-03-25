import SwiftUI

struct InspectorView: View {
    @Environment(AppState.self) private var appState
    @Environment(\.theme) private var theme

    var body: some View {
        if let session = appState.sessionList.selectedSession {
            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
                    // Header
                    HStack {
                        Text("Details")
                            .font(.headline)
                            .foregroundStyle(theme.text)
                        Spacer()
                    }

                    // Session info grid
                    DetailSection(title: "Session") {
                        DetailRow(label: "ID", value: session.id, theme: theme, mono: true)
                        DetailRow(label: "Status", theme: theme) {
                            StatusBadge(state: session.stateLabel)
                        }
                        DetailRow(label: "Backend", value: session.backend, theme: theme)
                        DetailRow(label: "Model", value: session.model ?? "default", theme: theme)
                    }

                    DetailSection(title: "State") {
                        DetailRow(label: "Running", theme: theme) {
                            Circle()
                                .fill(session.isRunning ? theme.orange : theme.gray)
                                .frame(width: 8, height: 8)
                            Text(session.isRunning ? "yes" : "no")
                                .font(.caption)
                                .foregroundStyle(theme.text)
                        }
                        DetailRow(label: "Permission", theme: theme) {
                            Circle()
                                .fill(session.pendingPermission ? theme.yellow : theme.gray)
                                .frame(width: 8, height: 8)
                            Text(session.pendingPermission ? "pending" : "none")
                                .font(.caption)
                                .foregroundStyle(theme.text)
                        }
                        if session.queueDepth > 0 {
                            DetailRow(label: "Queue", value: "\(session.queueDepth) pending", theme: theme)
                        }
                    }

                    DetailSection(title: "Project") {
                        DetailRow(label: "Dir", value: session.shortDir, theme: theme)
                        if let tool = session.currentTool, !tool.isEmpty {
                            DetailRow(label: "Tool", value: tool, theme: theme)
                        }
                    }

                    // Toggles
                    DetailSection(title: "Options") {
                        HStack {
                            Text("Skip permissions")
                                .font(.caption)
                                .foregroundStyle(theme.muted)
                            Spacer()
                            Toggle("", isOn: Binding(
                                get: { session.skipPermissions },
                                set: { newValue in
                                    Task {
                                        try? await appState.api.updateSession(
                                            id: session.id, skipPermissions: newValue
                                        )
                                        await appState.sessionList.refresh()
                                    }
                                }
                            ))
                            .toggleStyle(.switch)
                            .controlSize(.small)
                        }

                        HStack {
                            Text("Plan mode")
                                .font(.caption)
                                .foregroundStyle(theme.muted)
                            Spacer()
                            Toggle("", isOn: Binding(
                                get: { session.planMode },
                                set: { newValue in
                                    Task {
                                        try? await appState.api.updateSession(
                                            id: session.id, planMode: newValue
                                        )
                                        await appState.sessionList.refresh()
                                    }
                                }
                            ))
                            .toggleStyle(.switch)
                            .controlSize(.small)
                        }
                    }

                    // Sub-agents
                    if let agents = session.subAgents, !agents.isEmpty {
                        DetailSection(title: "Sub-Agents (\(agents.count))") {
                            ForEach(agents) { agent in
                                HStack(spacing: 6) {
                                    StatusBadge(state: agent.status)
                                    Text(agent.title)
                                        .font(.caption)
                                        .foregroundStyle(theme.text)
                                        .lineLimit(1)
                                }
                            }
                        }
                    }

                    // PR URL
                    if let prUrl = session.prUrl, !prUrl.isEmpty {
                        DetailSection(title: "Pull Request") {
                            Link(prUrl, destination: URL(string: prUrl)!)
                                .font(.caption)
                                .foregroundStyle(theme.cyan)
                        }
                    }

                    Spacer()
                }
                .padding()
            }
            .background(theme.panel)
        } else {
            VStack {
                Text("No session")
                    .foregroundStyle(theme.muted)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .background(theme.panel)
        }
    }
}

// MARK: - Detail helpers

struct DetailSection<Content: View>: View {
    let title: String
    @ViewBuilder let content: Content
    @Environment(\.theme) private var theme

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(title.uppercased())
                .font(.caption2.bold())
                .foregroundStyle(theme.muted)
            content
        }
    }
}

struct DetailRow: View {
    let label: String
    var value: String? = nil
    let theme: OrbitorTheme
    var mono: Bool = false
    var trailing: AnyView? = nil

    init(label: String, value: String, theme: OrbitorTheme, mono: Bool = false) {
        self.label = label
        self.value = value
        self.theme = theme
        self.mono = mono
    }

    init<V: View>(label: String, theme: OrbitorTheme, @ViewBuilder trailing: () -> V) {
        self.label = label
        self.theme = theme
        self.trailing = AnyView(trailing())
    }

    var body: some View {
        HStack {
            Text(label)
                .font(.caption)
                .foregroundStyle(theme.muted)
                .frame(width: 70, alignment: .trailing)
            if let value {
                Text(value)
                    .font(mono ? .system(.caption, design: .monospaced) : .caption)
                    .foregroundStyle(theme.text)
                    .lineLimit(1)
            }
            if let trailing {
                trailing
            }
            Spacer()
        }
    }
}
