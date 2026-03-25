import SwiftUI

// MARK: - Model lists (matching tui.go)

let claudeModels = [
    "(default)",
    "claude-opus-4-6",
    "claude-sonnet-4-6",
    "claude-haiku-4-5",
    "claude-opus-4-5",
    "claude-sonnet-4-5",
]

let copilotModels = [
    "(default)",
    "gpt-5", "gpt-5-mini", "gpt-5.1", "gpt-5.1-codex-mini",
    "gpt-5.3-codex", "gpt-5.4", "gpt-5.4-mini",
    "gpt-4o", "gpt-4o-mini",
    "o1", "o3-mini", "o4-mini",
    "claude-sonnet-4-6", "claude-opus-4-6",
]

func modelsForBackend(_ backend: String) -> [String] {
    backend == "copilot" ? copilotModels : claudeModels
}

func defaultModelForBackend(_ backend: String) -> String {
    backend == "copilot" ? "gpt-4o" : "claude-sonnet-4-6"
}

// MARK: - New Session Sheet

struct NewSessionSheet: View {
    @Environment(AppState.self) private var appState
    @Environment(\.theme) private var theme
    @Environment(\.dismiss) private var dismiss

    @State private var workingDir = ""
    @State private var backend = "claude"
    @State private var selectedModel = "(default)"
    @State private var skipPermissions = false
    @State private var planMode = false
    @State private var recentDirs: [String] = []

    var body: some View {
        VStack(spacing: 0) {
            // Header
            HStack {
                Image(systemName: "plus.circle.fill")
                    .font(.title2)
                    .foregroundStyle(theme.accent)
                VStack(alignment: .leading, spacing: 2) {
                    Text("New Session")
                        .font(.headline)
                        .foregroundStyle(theme.text)
                    Text("Start a new AI coding session")
                        .font(.caption)
                        .foregroundStyle(theme.muted)
                }
                Spacer()
                Button { dismiss() } label: {
                    Image(systemName: "xmark.circle.fill")
                        .font(.title3)
                        .foregroundStyle(theme.muted)
                }
                .buttonStyle(.plain)
            }
            .padding()

            Divider()

            Form {
                Section("Project") {
                    HStack {
                        TextField("Working directory", text: $workingDir)
                            .textFieldStyle(.roundedBorder)

                        Button("Browse") {
                            let panel = NSOpenPanel()
                            panel.canChooseFiles = false
                            panel.canChooseDirectories = true
                            panel.allowsMultipleSelection = false
                            if panel.runModal() == .OK, let url = panel.url {
                                workingDir = url.path
                            }
                        }
                    }

                    // Quick picks from existing sessions
                    if !recentDirs.isEmpty {
                        ScrollView(.horizontal, showsIndicators: false) {
                            HStack(spacing: 6) {
                                ForEach(recentDirs, id: \.self) { dir in
                                    Button {
                                        workingDir = dir
                                    } label: {
                                        Label(
                                            dir.components(separatedBy: "/").suffix(2).joined(separator: "/"),
                                            systemImage: "folder"
                                        )
                                        .font(.caption)
                                    }
                                    .buttonStyle(.bordered)
                                    .controlSize(.small)
                                }
                            }
                        }
                    }
                }

                Section("Backend") {
                    Picker("Backend", selection: $backend) {
                        Text("Claude Code").tag("claude")
                        Text("GitHub Copilot").tag("copilot")
                    }
                    .pickerStyle(.segmented)
                    .onChange(of: backend) { _, _ in
                        selectedModel = "(default)"
                    }

                    Picker("Model", selection: $selectedModel) {
                        ForEach(modelsForBackend(backend), id: \.self) { model in
                            Text(model).tag(model)
                        }
                    }
                    .pickerStyle(.menu)
                }

                Section("Options") {
                    Toggle("Skip permissions", isOn: $skipPermissions)
                    Toggle("Plan mode", isOn: $planMode)
                }
            }
            .formStyle(.grouped)
            .scrollContentBackground(.hidden)

            Divider()

            HStack {
                Button("Cancel") { dismiss() }
                    .keyboardShortcut(.cancelAction)
                Spacer()
                Button {
                    let dir = workingDir.isEmpty ? FileManager.default.currentDirectoryPath : workingDir
                    let m = selectedModel == "(default)" ? defaultModelForBackend(backend) : selectedModel
                    Task {
                        await appState.sessionList.createSession(
                            workingDir: dir, backend: backend,
                            model: m, skip: skipPermissions, plan: planMode
                        )
                        dismiss()
                    }
                } label: {
                    Label("Create Session", systemImage: "play.fill")
                }
                .keyboardShortcut(.defaultAction)
                .buttonStyle(.borderedProminent)
                .tint(theme.accent)
                .disabled(workingDir.isEmpty)
            }
            .padding()
        }
        .frame(width: 640, height: 460)
        .background(theme.panel)
        .onAppear {
            recentDirs = Array(Set(appState.sessionList.sessions.map(\.workingDir)))
            if workingDir.isEmpty {
                workingDir = recentDirs.first ?? FileManager.default.homeDirectoryForCurrentUser.path
            }
        }
    }
}

// MARK: - Fork Session Sheet

struct ForkSessionSheet: View {
    @Environment(AppState.self) private var appState
    @Environment(\.theme) private var theme
    @Environment(\.dismiss) private var dismiss

    @State private var prompt = ""

    var body: some View {
        VStack(spacing: 0) {
            // Header
            HStack {
                Image(systemName: "arrow.triangle.branch")
                    .font(.title2)
                    .foregroundStyle(theme.cyan)
                VStack(alignment: .leading, spacing: 2) {
                    Text("Fork Session")
                        .font(.headline)
                        .foregroundStyle(theme.text)
                    if let session = appState.sessionList.selectedSession {
                        Text("From: \(session.displayTitle)")
                            .font(.caption)
                            .foregroundStyle(theme.muted)
                            .lineLimit(1)
                    }
                }
                Spacer()
                Button { dismiss() } label: {
                    Image(systemName: "xmark.circle.fill")
                        .font(.title3)
                        .foregroundStyle(theme.muted)
                }
                .buttonStyle(.plain)
            }
            .padding()

            Divider()

            VStack(alignment: .leading, spacing: 12) {
                Text("Enter a prompt for the forked session:")
                    .font(.subheadline)
                    .foregroundStyle(theme.text)

                TextEditor(text: $prompt)
                    .font(.system(.body, design: .monospaced))
                    .foregroundStyle(theme.text)
                    .scrollContentBackground(.hidden)
                    .frame(minHeight: 100)
                    .padding(8)
                    .background(theme.selBg)
                    .clipShape(RoundedRectangle(cornerRadius: 8))
                    .overlay(
                        RoundedRectangle(cornerRadius: 8)
                            .strokeBorder(theme.accent, lineWidth: 1)
                    )

                Text("The forked session will inherit the full conversation history and start with your new prompt.")
                    .font(.caption)
                    .foregroundStyle(theme.muted)
            }
            .padding()

            Divider()

            HStack {
                Button("Cancel") { dismiss() }
                    .keyboardShortcut(.cancelAction)
                Spacer()
                Button {
                    guard let sourceID = appState.sessionList.selectedSessionID else { return }
                    Task {
                        await appState.sessionList.forkSession(sourceID: sourceID, prompt: prompt)
                        dismiss()
                    }
                } label: {
                    Label("Fork", systemImage: "arrow.triangle.branch")
                }
                .keyboardShortcut(.defaultAction)
                .buttonStyle(.borderedProminent)
                .tint(theme.cyan)
                .disabled(prompt.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            }
            .padding()
        }
        .frame(width: 480, height: 340)
        .background(theme.panel)
    }
}
