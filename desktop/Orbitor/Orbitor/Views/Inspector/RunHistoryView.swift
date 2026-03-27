import SwiftUI

struct RunHistoryView: View {
    let sessionID: String
    @Environment(AppState.self) private var appState
    @Environment(\.theme) private var theme
    @Environment(\.dismiss) private var dismiss

    @State private var runs: [RunRecord] = []
    @State private var isLoading = false
    @State private var selectedRunID: String?
    @State private var selectedFile: FileChange?

    private var selectedRun: RunRecord? {
        runs.first { $0.id == selectedRunID }
    }

    var body: some View {
        VStack(spacing: 0) {
            // Title bar
            HStack {
                Image(systemName: "clock.arrow.trianglehead.counterclockwise.rotate.90")
                    .foregroundStyle(theme.accent)
                Text("File History")
                    .font(.headline)
                    .foregroundStyle(theme.text)
                Spacer()
                if isLoading {
                    ProgressView().controlSize(.small)
                }
                Button {
                    dismiss()
                } label: {
                    Image(systemName: "xmark.circle.fill")
                        .foregroundStyle(theme.muted)
                }
                .buttonStyle(.plain)
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 12)
            .background(theme.panel)

            Divider().background(theme.sep)

            if runs.isEmpty && !isLoading {
                emptyState
            } else {
                HSplitView {
                    // Left: run list
                    runList
                        .frame(minWidth: 200, idealWidth: 240, maxWidth: 300)

                    // Right: file list + diff
                    rightPanel
                        .frame(minWidth: 400)
                }
            }
        }
        .background(theme.panel)
        .frame(minWidth: 800, minHeight: 500)
        .task { await load() }
    }

    // MARK: - Subviews

    private var emptyState: some View {
        VStack(spacing: 12) {
            Image(systemName: "clock.badge.xmark")
                .font(.system(size: 36))
                .foregroundStyle(theme.muted)
            Text("No file changes recorded yet")
                .font(.headline)
                .foregroundStyle(theme.muted)
            Text("File changes will appear here after the agent writes files.")
                .font(.caption)
                .foregroundStyle(theme.muted.opacity(0.7))
                .multilineTextAlignment(.center)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private var runList: some View {
        VStack(spacing: 0) {
            Text("RUNS")
                .font(.caption2.bold())
                .foregroundStyle(theme.muted)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal, 12)
                .padding(.vertical, 8)

            Divider().background(theme.sep)

            ScrollView {
                LazyVStack(spacing: 0) {
                    ForEach(runs) { run in
                        RunRowView(run: run, isSelected: run.id == selectedRunID)
                            .contentShape(Rectangle())
                            .onTapGesture {
                                selectedRunID = run.id
                                selectedFile = run.files.first
                            }
                    }
                }
            }
        }
        .background(theme.panel)
    }

    private var rightPanel: some View {
        Group {
            if let run = selectedRun {
                VSplitView {
                    // File list
                    fileList(for: run)
                        .frame(minHeight: 80, idealHeight: 120, maxHeight: 200)

                    // Diff viewer
                    diffPanel
                }
            } else {
                VStack {
                    Text("Select a run to view changes")
                        .foregroundStyle(theme.muted)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
    }

    private func fileList(for run: RunRecord) -> some View {
        VStack(spacing: 0) {
            HStack {
                Text("FILES CHANGED")
                    .font(.caption2.bold())
                    .foregroundStyle(theme.muted)
                Spacer()
                Text("\(run.files.count) file\(run.files.count == 1 ? "" : "s")")
                    .font(.caption2)
                    .foregroundStyle(theme.muted)
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 8)

            Divider().background(theme.sep)

            ScrollView {
                LazyVStack(spacing: 0) {
                    ForEach(run.files) { file in
                        FileRowView(file: file, isSelected: file.id == selectedFile?.id)
                            .contentShape(Rectangle())
                            .onTapGesture { selectedFile = file }
                    }
                }
            }
        }
        .background(theme.panel)
    }

    private var diffPanel: some View {
        VStack(spacing: 0) {
            if let file = selectedFile {
                HStack(spacing: 8) {
                    Image(systemName: file.before.isEmpty ? "plus.circle.fill" : "pencil.circle.fill")
                        .font(.caption)
                        .foregroundStyle(file.before.isEmpty ? Color.green : theme.cyan)
                    Text(file.relativePath)
                        .font(.system(.caption, design: .monospaced))
                        .foregroundStyle(theme.text)
                    Spacer()
                    let added = countAdded(file)
                    let removed = countRemoved(file)
                    if added > 0 {
                        Text("+\(added)")
                            .font(.caption2.monospacedDigit())
                            .foregroundStyle(Color.green)
                    }
                    if removed > 0 {
                        Text("-\(removed)")
                            .font(.caption2.monospacedDigit())
                            .foregroundStyle(Color.red)
                    }
                }
                .padding(.horizontal, 12)
                .padding(.vertical, 8)

                Divider().background(theme.sep)

                FileDiffView(before: file.before, after: file.after)
            } else {
                VStack {
                    Text("Select a file to view diff")
                        .foregroundStyle(theme.muted)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
    }

    // MARK: - Helpers

    private func load() async {
        isLoading = true
        defer { isLoading = false }
        if let records = try? await appState.api.sessionRunHistory(id: sessionID) {
            runs = records
            selectedRunID = runs.first?.id
            selectedFile = runs.first?.files.first
        }
    }

    private func countAdded(_ file: FileChange) -> Int {
        computeDiff(before: file.before, after: file.after).filter { $0.kind == .added }.count
    }

    private func countRemoved(_ file: FileChange) -> Int {
        computeDiff(before: file.before, after: file.after).filter { $0.kind == .removed }.count
    }
}

// MARK: - Row views

private struct RunRowView: View {
    let run: RunRecord
    let isSelected: Bool
    @Environment(\.theme) private var theme

    var body: some View {
        VStack(alignment: .leading, spacing: 3) {
            HStack(spacing: 6) {
                Circle()
                    .fill(isSelected ? theme.accent : theme.muted.opacity(0.5))
                    .frame(width: 6, height: 6)
                Text(run.startedAt, style: .time)
                    .font(.caption2)
                    .foregroundStyle(theme.muted)
                Spacer()
                Text("\(run.files.count) file\(run.files.count == 1 ? "" : "s")")
                    .font(.caption2)
                    .foregroundStyle(theme.muted)
            }

            Text(run.prompt.isEmpty ? "(continuation)" : run.prompt)
                .font(.caption)
                .foregroundStyle(theme.text)
                .lineLimit(2)
                .padding(.leading, 12)

            if let dur = runDuration {
                Text(dur)
                    .font(.caption2)
                    .foregroundStyle(theme.muted.opacity(0.7))
                    .padding(.leading, 12)
            }
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 8)
        .background(isSelected ? theme.accent.opacity(0.12) : Color.clear)
        .overlay(alignment: .leading) {
            if isSelected {
                Rectangle()
                    .fill(theme.accent)
                    .frame(width: 2)
            }
        }
    }

    private var runDuration: String? {
        guard let end = run.completedAt else { return nil }
        let secs = Int(end.timeIntervalSince(run.startedAt))
        if secs < 60 { return "\(secs)s" }
        return "\(secs / 60)m \(secs % 60)s"
    }
}

private struct FileRowView: View {
    let file: FileChange
    let isSelected: Bool
    @Environment(\.theme) private var theme

    var body: some View {
        HStack(spacing: 8) {
            Image(systemName: file.before.isEmpty ? "plus.circle.fill" : "pencil.circle.fill")
                .font(.system(size: 9))
                .foregroundStyle(file.before.isEmpty ? Color.green : theme.cyan)
            Text(file.relativePath)
                .font(.system(size: 11, design: .monospaced))
                .foregroundStyle(theme.text)
                .lineLimit(1)
                .truncationMode(.head)
            Spacer()
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 5)
        .background(isSelected ? theme.accent.opacity(0.12) : Color.clear)
    }
}
