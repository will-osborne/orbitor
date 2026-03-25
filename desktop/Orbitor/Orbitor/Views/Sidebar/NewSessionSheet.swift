import SwiftUI

struct NewSessionSheet: View {
    @Environment(AppState.self) private var appState
    @Environment(\.theme) private var theme
    @Environment(\.dismiss) private var dismiss

    @State private var workingDir = ""
    @State private var backend = "claude"
    @State private var model = ""
    @State private var skipPermissions = false
    @State private var planMode = false
    @State private var recentDirs: [String] = []

    var body: some View {
        VStack(spacing: 0) {
            // Header
            HStack {
                Text("New Session")
                    .font(.headline)
                    .foregroundStyle(theme.text)
                Spacer()
                Button { dismiss() } label: {
                    Image(systemName: "xmark.circle.fill")
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
                            HStack {
                                ForEach(recentDirs, id: \.self) { dir in
                                    Button(dir.components(separatedBy: "/").suffix(2).joined(separator: "/")) {
                                        workingDir = dir
                                    }
                                    .buttonStyle(.bordered)
                                    .font(.caption)
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

                    TextField("Model (optional)", text: $model)
                        .textFieldStyle(.roundedBorder)

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
                Button("Create") {
                    let dir = workingDir.isEmpty ? FileManager.default.currentDirectoryPath : workingDir
                    let m = model.isEmpty ? (backend == "claude" ? "claude-sonnet-4-6" : "gpt-4o") : model
                    Task {
                        await appState.sessionList.createSession(
                            workingDir: dir, backend: backend,
                            model: m, skip: skipPermissions, plan: planMode
                        )
                        dismiss()
                    }
                }
                .keyboardShortcut(.defaultAction)
                .buttonStyle(.borderedProminent)
                .tint(theme.accent)
                .disabled(workingDir.isEmpty)
            }
            .padding()
        }
        .frame(width: 500, height: 420)
        .background(theme.panel)
        .onAppear {
            recentDirs = Array(Set(appState.sessionList.sessions.map(\.workingDir)))
            if workingDir.isEmpty {
                workingDir = recentDirs.first ?? FileManager.default.homeDirectoryForCurrentUser.path
            }
        }
    }
}
