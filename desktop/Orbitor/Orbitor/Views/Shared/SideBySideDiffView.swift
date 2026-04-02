import SwiftUI

// MARK: - Side-by-side diff data model

struct SideBySideLine: Identifiable {
    let id = UUID()
    let leftNumber: Int?
    let leftText: String?
    let leftKind: DiffLine.Kind?
    let rightNumber: Int?
    let rightText: String?
    let rightKind: DiffLine.Kind?
    /// True for separator rows (context gaps).
    var isSeparator: Bool { leftNumber == nil && rightNumber == nil && leftText == nil && rightText == nil }
}

/// Converts a flat LCS edit script into paired side-by-side lines.
/// Consecutive removed/added lines are zipped together; the shorter side gets blank padding.
func computeSideBySideDiff(before: String, after: String) -> [SideBySideLine] {
    let oldLines = before.components(separatedBy: "\n")
    let newLines = after.components(separatedBy: "\n")

    let m = oldLines.count
    let n = newLines.count

    // LCS table
    var dp = Array(repeating: Array(repeating: 0, count: n + 1), count: m + 1)
    for i in 1...max(m, 1) {
        guard i <= m else { break }
        for j in 1...max(n, 1) {
            guard j <= n else { break }
            if oldLines[i - 1] == newLines[j - 1] {
                dp[i][j] = dp[i - 1][j - 1] + 1
            } else {
                dp[i][j] = max(dp[i - 1][j], dp[i][j - 1])
            }
        }
    }

    // Backtrack to get edit script
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

    // Walk edits, collecting consecutive removed/added runs and flushing them as paired lines.
    var result: [SideBySideLine] = []
    var removedBuf: [(Int, String)] = []  // (lineNumber, text)
    var addedBuf: [(Int, String)] = []

    func flush() {
        let count = max(removedBuf.count, addedBuf.count)
        for k in 0..<count {
            let left = k < removedBuf.count ? removedBuf[k] : nil
            let right = k < addedBuf.count ? addedBuf[k] : nil
            result.append(SideBySideLine(
                leftNumber: left?.0,
                leftText: left?.1,
                leftKind: left != nil ? .removed : nil,
                rightNumber: right?.0,
                rightText: right?.1,
                rightKind: right != nil ? .added : nil
            ))
        }
        removedBuf.removeAll()
        addedBuf.removeAll()
    }

    for edit in edits {
        switch edit {
        case .unchanged(let oi, let ni):
            flush()
            result.append(SideBySideLine(
                leftNumber: oi + 1,
                leftText: oldLines[oi],
                leftKind: .unchanged,
                rightNumber: ni + 1,
                rightText: newLines[ni],
                rightKind: .unchanged
            ))
        case .removed(let oi):
            removedBuf.append((oi + 1, oldLines[oi]))
        case .added(let ni):
            addedBuf.append((ni + 1, newLines[ni]))
        }
    }
    flush()

    return result
}

// MARK: - Side-by-side diff view

struct SideBySideDiffView: View {
    let before: String
    let after: String
    let leftTitle: String
    let rightTitle: String
    @Environment(\.theme) private var theme

    init(before: String, after: String, leftTitle: String = "Original", rightTitle: String = "Modified") {
        self.before = before
        self.after = after
        self.leftTitle = leftTitle
        self.rightTitle = rightTitle
    }

    private var lines: [SideBySideLine] {
        computeSideBySideDiff(before: before, after: after)
    }

    private let rowHeight: CGFloat = 18
    private let gutterWidth: CGFloat = 44
    private let fontSize: CGFloat = 11

    var body: some View {
        VStack(spacing: 0) {
            // Column headers
            HStack(spacing: 0) {
                Text(leftTitle)
                    .font(.caption2.bold())
                    .foregroundStyle(theme.muted)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.horizontal, 12)
                    .padding(.vertical, 6)
                    .background(theme.panel)

                Divider().frame(height: 24)

                Text(rightTitle)
                    .font(.caption2.bold())
                    .foregroundStyle(theme.muted)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.horizontal, 12)
                    .padding(.vertical, 6)
                    .background(theme.panel)
            }

            Divider().background(theme.sep)

            // Diff content — single scroll view keeps both sides in sync
            ScrollView([.vertical, .horizontal]) {
                let computed = lines
                HStack(alignment: .top, spacing: 0) {
                    // Left column (old)
                    VStack(alignment: .leading, spacing: 0) {
                        ForEach(computed) { line in
                            leftRow(line)
                                .frame(height: rowHeight)
                        }
                    }
                    .frame(minWidth: 400)

                    // Center divider
                    Rectangle()
                        .fill(theme.sep)
                        .frame(width: 1)

                    // Right column (new)
                    VStack(alignment: .leading, spacing: 0) {
                        ForEach(computed) { line in
                            rightRow(line)
                                .frame(height: rowHeight)
                        }
                    }
                    .frame(minWidth: 400)
                }
            }
        }
        .background(theme.panel)
    }

    // MARK: - Row builders

    @ViewBuilder
    private func leftRow(_ line: SideBySideLine) -> some View {
        if line.isSeparator {
            separatorRow
        } else {
            HStack(spacing: 0) {
                Text(line.leftNumber.map { "\($0)" } ?? "")
                    .font(.system(size: 10, design: .monospaced))
                    .foregroundStyle(Color.gray.opacity(0.5))
                    .frame(width: gutterWidth, alignment: .trailing)
                    .padding(.trailing, 6)

                Text(line.leftText ?? "")
                    .font(.system(size: fontSize, design: .monospaced))
                    .foregroundStyle(textColor(line.leftKind))
                    .textSelection(.enabled)

                Spacer(minLength: 0)
            }
            .padding(.vertical, 1)
            .background(bgColor(line.leftKind, side: .left))
        }
    }

    @ViewBuilder
    private func rightRow(_ line: SideBySideLine) -> some View {
        if line.isSeparator {
            separatorRow
        } else {
            HStack(spacing: 0) {
                Text(line.rightNumber.map { "\($0)" } ?? "")
                    .font(.system(size: 10, design: .monospaced))
                    .foregroundStyle(Color.gray.opacity(0.5))
                    .frame(width: gutterWidth, alignment: .trailing)
                    .padding(.trailing, 6)

                Text(line.rightText ?? "")
                    .font(.system(size: fontSize, design: .monospaced))
                    .foregroundStyle(textColor(line.rightKind))
                    .textSelection(.enabled)

                Spacer(minLength: 0)
            }
            .padding(.vertical, 1)
            .background(bgColor(line.rightKind, side: .right))
        }
    }

    private var separatorRow: some View {
        HStack {
            Text("···")
                .font(.system(size: 10, design: .monospaced))
                .foregroundStyle(Color.gray.opacity(0.4))
                .padding(.leading, gutterWidth + 6)
            Spacer()
        }
        .background(Color.gray.opacity(0.05))
    }

    // MARK: - Colors

    private enum Side { case left, right }

    private func bgColor(_ kind: DiffLine.Kind?, side: Side) -> Color {
        switch kind {
        case .removed:   return Color.red.opacity(0.15)
        case .added:     return Color.green.opacity(0.15)
        case .unchanged: return Color.clear
        case nil:        return Color.gray.opacity(0.05) // blank padding
        case .some:      return Color.clear
        }
    }

    private func textColor(_ kind: DiffLine.Kind?) -> Color {
        switch kind {
        case .removed:   return Color.primary
        case .added:     return Color.primary
        case .unchanged: return Color.primary.opacity(0.7)
        case nil:        return Color.clear
        case .some:      return Color.primary
        }
    }
}
