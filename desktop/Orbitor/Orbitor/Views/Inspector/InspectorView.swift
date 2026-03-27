import SwiftUI

struct InspectorView: View {
    @Environment(AppState.self) private var appState
    @Environment(\.theme) private var theme
    @State private var gitBranch: String? = nil

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
                        if appState.chat.errorCount > 0 {
                            Label("\(appState.chat.errorCount) error\(appState.chat.errorCount == 1 ? "" : "s")", systemImage: "exclamationmark.triangle.fill")
                                .font(.caption)
                                .foregroundStyle(theme.red)
                        }
                    }

                    // Session info grid
                    DetailSection(title: "Session") {
                        DetailRow(label: "ID", value: session.id, theme: theme, mono: true)
                        DetailRow(label: "Status", theme: theme) {
                            StatusBadge(state: session.stateLabel)
                        }
                        DetailRow(label: "Backend", value: session.backend, theme: theme)
                        ModelPickerRow(session: session)
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
                        if let dur = appState.chat.lastRunDuration {
                            DetailRow(label: "Last run", value: formatDuration(dur), theme: theme)
                        }
                    }

                    DetailSection(title: "Project") {
                        DetailRow(label: "Dir", value: session.shortDir, theme: theme)
                        if let branch = gitBranch {
                            DetailRow(label: "Branch", theme: theme) {
                                Image(systemName: "arrow.triangle.branch")
                                    .font(.caption2)
                                    .foregroundStyle(theme.cyan)
                                Text(branch)
                                    .font(.system(.caption, design: .monospaced))
                                    .foregroundStyle(theme.cyan)
                            }
                        }
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

                    // Files touched this session
                    if !appState.chat.filesTouched.isEmpty {
                        DetailSection(title: "Files Changed (\(appState.chat.filesTouched.count))") {
                            ForEach(appState.chat.filesTouched.prefix(10), id: \.self) { path in
                                HStack(spacing: 4) {
                                    Image(systemName: "pencil.circle.fill")
                                        .font(.system(size: 9))
                                        .foregroundStyle(theme.cyan)
                                    Text(path)
                                        .font(.system(size: 10, design: .monospaced))
                                        .foregroundStyle(theme.text)
                                        .lineLimit(1)
                                        .truncationMode(.head)
                                }
                            }
                            if appState.chat.filesTouched.count > 10 {
                                Text("…and \(appState.chat.filesTouched.count - 10) more")
                                    .font(.caption2)
                                    .foregroundStyle(theme.muted)
                            }
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

                    // Delete
                    DetailSection(title: "Danger Zone") {
                        Button(role: .destructive) {
                            Task { await appState.sessionList.deleteSession(session.id) }
                        } label: {
                            HStack(spacing: 4) {
                                Image(systemName: "trash")
                                Text("Delete Session")
                            }
                            .font(.caption)
                            .foregroundStyle(theme.red)
                        }
                        .buttonStyle(.plain)
                        .hoverScale(1.05)
                    }

                    Spacer()
                }
                .padding()
            }
            .background(theme.panel)
            .onAppear { loadGitBranch(for: session) }
            .onChange(of: session.id) { _, _ in loadGitBranch(for: session) }
        } else {
            VStack {
                Text("No session")
                    .foregroundStyle(theme.muted)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .background(theme.panel)
        }
    }

    private func loadGitBranch(for session: SessionInfo) {
        let headPath = session.workingDir + "/.git/HEAD"
        Task {
            let branch = await Task.detached(priority: .background) {
                guard let content = try? String(contentsOfFile: headPath, encoding: .utf8) else { return nil as String? }
                let trimmed = content.trimmingCharacters(in: .whitespacesAndNewlines)
                if trimmed.hasPrefix("ref: refs/heads/") {
                    return String(trimmed.dropFirst("ref: refs/heads/".count))
                }
                return String(trimmed.prefix(8)) // detached HEAD
            }.value
            await MainActor.run { gitBranch = branch }
        }
    }

    private func formatDuration(_ t: TimeInterval) -> String {
        let total = Int(t)
        if total < 60 { return "\(total)s" }
        return "\(total / 60)m \(total % 60)s"
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

// MARK: - Model picker for existing sessions

struct ModelPickerRow: View {
    let session: SessionInfo
    @Environment(AppState.self) private var appState
    @Environment(\.theme) private var theme
    @State private var selectedModel: String = ""
    @State private var isUserChange = false

    private func syncModel() {
        let current = session.model ?? ""
        let resolved = modelsForBackend(session.backend).contains(current) ? current : "(default)"
        if selectedModel != resolved {
            selectedModel = resolved
        }
    }

    var body: some View {
        HStack {
            Text("Model")
                .font(.caption)
                .foregroundStyle(theme.muted)
                .frame(width: 70, alignment: .trailing)
            Picker("", selection: $selectedModel) {
                ForEach(modelsForBackend(session.backend), id: \.self) { model in
                    Text(model).tag(model)
                }
            }
            .pickerStyle(.menu)
            .controlSize(.small)
            .onChange(of: selectedModel) { _, newValue in
                guard isUserChange, !newValue.isEmpty else { return }
                let modelToSend = newValue == "(default)" ? defaultModelForBackend(session.backend) : newValue
                Task {
                    try? await appState.api.updateSession(id: session.id, model: modelToSend)
                    await appState.sessionList.refresh()
                }
            }
        }
        .onAppear {
            syncModel()
            // Delay enabling user changes to avoid triggering on initial sync
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.1) {
                isUserChange = true
            }
        }
        .onChange(of: session.model) { _, _ in
            isUserChange = false
            syncModel()
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.1) {
                isUserChange = true
            }
        }
    }
}
