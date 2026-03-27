import SwiftUI

// MARK: - LCS Diff Engine (before/after text)

struct DiffLine: Identifiable {
    enum Kind { case added, removed, unchanged }
    let id = UUID()
    let kind: Kind
    let oldNumber: Int?
    let newNumber: Int?
    let text: String
}

/// Computes a line-level unified diff between `before` and `after`.
/// Returns DiffLine entries with ±context lines of context around each change.
func computeDiff(before: String, after: String, context: Int = 3) -> [DiffLine] {
    let oldLines = before.components(separatedBy: "\n")
    let newLines = after.components(separatedBy: "\n")

    let m = oldLines.count
    let n = newLines.count
    var dp = Array(repeating: Array(repeating: 0, count: n + 1), count: m + 1)
    for i in 1...m {
        for j in 1...n {
            if oldLines[i - 1] == newLines[j - 1] {
                dp[i][j] = dp[i - 1][j - 1] + 1
            } else {
                dp[i][j] = max(dp[i - 1][j], dp[i][j - 1])
            }
        }
    }

    enum Edit { case unchanged(Int, Int), removed(Int), added(Int) }
    var edits: [Edit] = []
    var i = m, j = n
    while i > 0 || j > 0 {
        if i > 0 && j > 0 && oldLines[i - 1] == newLines[j - 1] {
            edits.append(.unchanged(i - 1, j - 1))
            i -= 1; j -= 1
        } else if j > 0 && (i == 0 || dp[i][j - 1] >= dp[i - 1][j]) {
            edits.append(.added(j - 1))
            j -= 1
        } else {
            edits.append(.removed(i - 1))
            i -= 1
        }
    }
    edits.reverse()

    var changedOld = Set<Int>()
    var changedNew = Set<Int>()
    for edit in edits {
        switch edit {
        case .removed(let oi): changedOld.insert(oi)
        case .added(let ni): changedNew.insert(ni)
        default: break
        }
    }

    var showOld = Set<Int>()
    var showNew = Set<Int>()
    for oi in changedOld {
        for k in max(0, oi - context)...min(m - 1, oi + context) { showOld.insert(k) }
    }
    for ni in changedNew {
        for k in max(0, ni - context)...min(n - 1, ni + context) { showNew.insert(k) }
    }

    var result: [DiffLine] = []
    var prevOld = -1
    var prevNew = -1

    for edit in edits {
        switch edit {
        case .unchanged(let oi, let ni):
            let show = showOld.contains(oi) || showNew.contains(ni)
            if show {
                if prevOld >= 0 && (oi > prevOld + 1 || ni > prevNew + 1) {
                    result.append(DiffLine(kind: .unchanged, oldNumber: nil, newNumber: nil, text: ""))
                }
                result.append(DiffLine(kind: .unchanged, oldNumber: oi + 1, newNumber: ni + 1, text: oldLines[oi]))
                prevOld = oi; prevNew = ni
            }
        case .removed(let oi):
            result.append(DiffLine(kind: .removed, oldNumber: oi + 1, newNumber: nil, text: oldLines[oi]))
            prevOld = oi
        case .added(let ni):
            result.append(DiffLine(kind: .added, oldNumber: nil, newNumber: ni + 1, text: newLines[ni]))
            prevNew = ni
        }
    }

    return result
}

// MARK: - FileDiffView (before/after strings → computed diff)

struct FileDiffView: View {
    let before: String
    let after: String
    @Environment(\.theme) private var theme

    private var lines: [DiffLine] { computeDiff(before: before, after: after) }

    var body: some View {
        ScrollView([.vertical, .horizontal]) {
            LazyVStack(alignment: .leading, spacing: 0) {
                ForEach(lines) { line in
                    FileDiffLineRow(line: line)
                }
            }
            .padding(.vertical, 4)
        }
        .background(theme.panel)
    }
}

private struct FileDiffLineRow: View {
    let line: DiffLine
    @Environment(\.theme) private var theme

    private var bg: Color {
        switch line.kind {
        case .added:     return Color.green.opacity(0.15)
        case .removed:   return Color.red.opacity(0.15)
        case .unchanged: return Color.clear
        }
    }

    private var prefix: String {
        switch line.kind {
        case .added:     return "+"
        case .removed:   return "-"
        case .unchanged: return line.oldNumber == nil ? "…" : " "
        }
    }

    private var prefixColor: Color {
        switch line.kind {
        case .added:     return Color.green
        case .removed:   return Color.red
        case .unchanged: return line.oldNumber == nil ? Color.gray : Color.gray.opacity(0.5)
        }
    }

    var body: some View {
        if line.oldNumber == nil && line.newNumber == nil && line.text.isEmpty {
            HStack(spacing: 0) {
                Text("  ···")
                    .font(.system(size: 11, design: .monospaced))
                    .foregroundStyle(Color.gray.opacity(0.5))
                    .padding(.horizontal, 8)
                    .padding(.vertical, 2)
                Spacer()
            }
            .background(Color.gray.opacity(0.05))
        } else {
            HStack(spacing: 0) {
                Text(line.oldNumber.map { "\($0)" } ?? "")
                    .font(.system(size: 10, design: .monospaced))
                    .foregroundStyle(Color.gray.opacity(0.5))
                    .frame(width: 36, alignment: .trailing)
                    .padding(.trailing, 4)

                Text(line.newNumber.map { "\($0)" } ?? "")
                    .font(.system(size: 10, design: .monospaced))
                    .foregroundStyle(Color.gray.opacity(0.5))
                    .frame(width: 36, alignment: .trailing)
                    .padding(.trailing, 6)

                Text(prefix)
                    .font(.system(size: 11, design: .monospaced))
                    .foregroundStyle(prefixColor)
                    .frame(width: 14, alignment: .center)

                Text(line.text)
                    .font(.system(size: 11, design: .monospaced))
                    .foregroundStyle(line.kind == .unchanged ? Color.primary.opacity(0.7) : Color.primary)
                    .textSelection(.enabled)

                Spacer(minLength: 0)
            }
            .padding(.vertical, 1)
            .background(bg)
        }
    }
}

// MARK: - Pre-formatted unified diff (used by chat messages)

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
