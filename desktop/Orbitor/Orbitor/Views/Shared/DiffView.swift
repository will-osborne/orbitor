import SwiftUI

/// Returns true if `text` looks like a unified diff (has file headers or hunk markers).
func looksLikeDiff(_ text: String) -> Bool {
    let lines = text.components(separatedBy: "\n")
    let fileHeaders = lines.filter { $0.hasPrefix("--- ") || $0.hasPrefix("+++ ") }.count
    let hunkHeaders = lines.filter { $0.hasPrefix("@@ ") || ($0.hasPrefix("@@") && $0.count > 2) }.count
    return fileHeaders >= 2 || hunkHeaders >= 1
}

/// Renders a unified diff with colored lines and a copy button.
struct DiffView: View {
    let diff: String
    @Environment(\.theme) private var theme
    @Environment(AppState.self) private var appState
    @State private var isCopied = false

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            // Header bar
            HStack {
                Text("diff")
                    .font(.caption2)
                    .foregroundStyle(theme.muted)
                Spacer()
                Button {
                    NSPasteboard.general.clearContents()
                    NSPasteboard.general.setString(diff, forType: .string)
                    isCopied = true
                    Task {
                        try? await Task.sleep(for: .seconds(2))
                        isCopied = false
                    }
                } label: {
                    Image(systemName: isCopied ? "checkmark" : "doc.on.doc")
                        .font(.caption2)
                        .foregroundStyle(isCopied ? theme.green : theme.muted)
                }
                .buttonStyle(.plain)
            }
            .padding(.horizontal, 10)
            .padding(.top, 6)
            .padding(.bottom, 4)

            Divider()

            // Diff lines
            ScrollView(.horizontal, showsIndicators: false) {
                VStack(alignment: .leading, spacing: 0) {
                    ForEach(Array(diff.components(separatedBy: "\n").enumerated()), id: \.offset) { _, line in
                        DiffLineView(line: line)
                    }
                }
                .padding(.vertical, 4)
            }
        }
        .background(Color(hex: "1E1E1E").opacity(0.5))
        .clipShape(RoundedRectangle(cornerRadius: 6))
        .overlay(
            RoundedRectangle(cornerRadius: 6)
                .strokeBorder(theme.border, lineWidth: 1)
        )
    }
}

// MARK: - Single diff line

private struct DiffLineView: View {
    let line: String
    @Environment(\.theme) private var theme
    @Environment(AppState.self) private var appState

    private enum Kind { case added, removed, hunkHeader, fileHeader, context }

    private var kind: Kind {
        if line.hasPrefix("+++ ") || line.hasPrefix("--- ") { return .fileHeader }
        if line.hasPrefix("@@") { return .hunkHeader }
        if line.hasPrefix("+") { return .added }
        if line.hasPrefix("-") { return .removed }
        return .context
    }

    var body: some View {
        HStack(spacing: 0) {
            // Gutter (+/-)
            Text(gutterText)
                .font(.system(size: max(appState.fontSize - 2, 8), design: .monospaced))
                .foregroundStyle(gutterColor)
                .frame(width: 14, alignment: .center)
                .padding(.leading, 8)

            // Line content (strip leading +/- for added/removed)
            Text(displayText)
                .font(.system(size: max(appState.fontSize - 1, 9), design: .monospaced))
                .foregroundStyle(textColor)
                .textSelection(.enabled)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal, 6)
                .padding(.vertical, 1)
        }
        .background(bgColor)
    }

    private var gutterText: String {
        switch kind {
        case .added: return "+"
        case .removed: return "-"
        default: return ""
        }
    }

    private var displayText: String {
        switch kind {
        case .added, .removed: return String(line.dropFirst(1))
        default: return line
        }
    }

    private var bgColor: Color {
        switch kind {
        case .added: return Color.green.opacity(0.10)
        case .removed: return Color.red.opacity(0.10)
        case .hunkHeader: return Color.blue.opacity(0.07)
        default: return Color.clear
        }
    }

    private var textColor: Color {
        switch kind {
        case .added: return theme.green
        case .removed: return theme.red
        case .hunkHeader: return theme.cyan
        case .fileHeader: return theme.muted
        case .context: return theme.text
        }
    }

    private var gutterColor: Color {
        switch kind {
        case .added: return theme.green
        case .removed: return theme.red
        default: return theme.muted
        }
    }
}
