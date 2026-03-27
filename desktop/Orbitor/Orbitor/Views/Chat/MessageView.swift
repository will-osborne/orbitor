import SwiftUI

struct MessageView: View {
    let message: ChatMessage
    @Environment(\.theme) private var theme

    var body: some View {
        switch message {
        case .promptSent(_, let text, let ts):
            TurnDivider(role: "you", timestamp: ts, isUser: true)
            UserBubble(text: text, timestamp: ts)

        case .agentText(_, let text, let ts):
            AgentBubble(text: text, timestamp: ts)

        case .toolCall(_, let call, let ts):
            ToolCallView(call: call, timestamp: ts)

        case .toolResult:
            EmptyView()

        case .permissionRequest(_, let req, _):
            PermissionRequestRow(request: req)

        case .permissionResolved(_, _, let optionId, _):
            StatusPill(
                icon: "checkmark.shield",
                text: "Permission \(optionId == "allow" ? "approved" : "denied")",
                color: optionId == "allow" ? theme.green : theme.red
            )

        case .runComplete(_, let reason, _):
            StatusPill(
                icon: "checkmark.circle.fill",
                text: reason == "end_turn" ? "Done" : "Done — \(reason)",
                color: theme.green
            )

        case .interrupted:
            StatusPill(icon: "stop.circle.fill", text: "Interrupted", color: theme.orange)

        case .error(_, let msg, _):
            StatusPill(icon: "exclamationmark.triangle.fill", text: msg, color: theme.red)

        case .historyBatch:
            EmptyView()

        case .sessionStatus:
            EmptyView()
        }
    }
}

// MARK: - Turn divider

struct TurnDivider: View {
    let role: String
    let timestamp: Date
    var isUser: Bool = false
    @Environment(\.theme) private var theme

    var body: some View {
        HStack(spacing: 8) {
            Rectangle()
                .fill(isUser ? theme.accent.opacity(0.3) : theme.border)
                .frame(height: 1)

            HStack(spacing: 4) {
                Text(role)
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(isUser ? theme.accent : theme.cyan)
                Text("·")
                    .foregroundStyle(theme.muted)
                Text(timestamp, style: .time)
                    .font(.caption)
                    .foregroundStyle(theme.muted)
            }
            .fixedSize()

            Rectangle()
                .fill(isUser ? theme.accent.opacity(0.3) : theme.border)
                .frame(height: 1)
        }
        .padding(.top, 12)
        .padding(.bottom, 4)
    }
}

// MARK: - Status pill

struct StatusPill: View {
    let icon: String
    let text: String
    let color: Color
    @Environment(\.theme) private var theme

    var body: some View {
        HStack {
            Spacer()
            HStack(spacing: 6) {
                Image(systemName: icon)
                    .font(.caption2)
                Text(text)
                    .font(.caption)
            }
            .foregroundStyle(color)
            .padding(.horizontal, 12)
            .padding(.vertical, 4)
            .background(color.opacity(0.1))
            .clipShape(Capsule())
            .overlay(
                Capsule().strokeBorder(color.opacity(0.2), lineWidth: 1)
            )
            Spacer()
        }
        .padding(.vertical, 4)
    }
}

// MARK: - User prompt bubble

struct UserBubble: View {
    let text: String
    let timestamp: Date
    @Environment(\.theme) private var theme
    @Environment(AppState.self) private var appState

    var body: some View {
        HStack {
            Spacer(minLength: 60)
            Text(text)
                .font(.system(size: appState.fontSize))
                .foregroundStyle(theme.text)
                .textSelection(.enabled)
                .padding(.horizontal, 14)
                .padding(.vertical, 10)
                .background(theme.accent.opacity(0.12))
                .clipShape(RoundedRectangle(cornerRadius: 12))
                .overlay(
                    RoundedRectangle(cornerRadius: 12)
                        .strokeBorder(theme.accent.opacity(0.25), lineWidth: 1)
                )
        }
    }
}

// MARK: - Agent text bubble

struct AgentBubble: View {
    let text: String
    let timestamp: Date
    @Environment(\.theme) private var theme
    @Environment(AppState.self) private var appState

    var body: some View {
        HStack {
            MarkdownTextView(text: text)
                .padding(.horizontal, 14)
                .padding(.vertical, 10)
                .background(theme.selBg)
                .clipShape(RoundedRectangle(cornerRadius: 12))
                .overlay(
                    RoundedRectangle(cornerRadius: 12)
                        .strokeBorder(theme.border, lineWidth: 1)
                )
            Spacer(minLength: 60)
        }
    }
}

// MARK: - Tool call view

struct ToolCallView: View {
    let call: ToolCallData
    let timestamp: Date
    @Environment(\.theme) private var theme
    @State private var isExpanded = false

    private var statusIcon: (String, Color) {
        switch call.status {
        case "done", "completed": return ("checkmark.circle.fill", theme.green)
        case "error", "failed": return ("xmark.circle.fill", theme.red)
        case "running", "in_progress": return ("arrow.triangle.2.circlepath", theme.orange)
        default: return ("circle.dashed", theme.muted)
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            // Header row
            Button {
                withAnimation(.easeInOut(duration: 0.15)) {
                    isExpanded.toggle()
                }
            } label: {
                HStack(spacing: 6) {
                    Image(systemName: isExpanded ? "chevron.down" : "chevron.right")
                        .font(.caption2)
                        .foregroundStyle(theme.muted)
                        .frame(width: 10)

                    Image(systemName: statusIcon.0)
                        .font(.caption2)
                        .foregroundStyle(statusIcon.1)

                    Text(call.title.isEmpty ? call.kind : call.title)
                        .font(.system(.caption, design: .monospaced))
                        .foregroundStyle(theme.cyan)
                        .lineLimit(1)

                    if !call.kind.isEmpty && !call.title.isEmpty {
                        Text(call.kind)
                            .font(.caption2)
                            .foregroundStyle(theme.muted)
                    }

                    Spacer()
                }
            }
            .buttonStyle(.plain)
            .padding(.horizontal, 10)
            .padding(.vertical, 5)

            // Content (expandable)
            if isExpanded, let content = call.content, !content.isEmpty {
                Divider().padding(.horizontal, 10)
                if looksLikeDiff(content) {
                    DiffView(diff: content)
                        .padding(.horizontal, 6)
                        .padding(.bottom, 6)
                        .transition(.opacity.combined(with: .scale(scale: 0.98, anchor: .top)))
                } else {
                    CodeBlockView(code: content, language: call.kind)
                        .padding(.horizontal, 6)
                        .padding(.bottom, 6)
                        .transition(.opacity.combined(with: .scale(scale: 0.98, anchor: .top)))
                }
            }
        }
        .background(theme.selBg.opacity(0.3))
        .clipShape(RoundedRectangle(cornerRadius: 6))
        .overlay(
            RoundedRectangle(cornerRadius: 6)
                .strokeBorder(theme.border.opacity(0.5), lineWidth: 1)
        )
        .hoverHighlight()
    }
}

// MARK: - Collapsed tool group

struct CollapsedToolGroup: View {
    let calls: [ToolCallData]
    let timestamp: Date
    @Environment(\.theme) private var theme
    @State private var isExpanded = false

    private var allDone: Bool {
        calls.allSatisfy { $0.status == "done" || $0.status == "completed" }
    }

    private var failCount: Int {
        calls.filter { $0.status == "error" || $0.status == "failed" }.count
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            // Summary header
            Button {
                withAnimation(.easeInOut(duration: 0.15)) {
                    isExpanded.toggle()
                }
            } label: {
                HStack(spacing: 6) {
                    Image(systemName: isExpanded ? "chevron.down" : "chevron.right")
                        .font(.caption2)
                        .foregroundStyle(theme.muted)
                        .frame(width: 10)

                    Image(systemName: failCount > 0 ? "exclamationmark.circle.fill" : "checkmark.circle.fill")
                        .font(.caption2)
                        .foregroundStyle(failCount > 0 ? theme.orange : theme.green)

                    Text("\(calls.count) tool calls")
                        .font(.system(.caption, design: .monospaced))
                        .foregroundStyle(theme.cyan)

                    // Show unique kinds
                    let kinds = Array(Set(calls.map(\.kind).filter { !$0.isEmpty })).prefix(3)
                    if !kinds.isEmpty {
                        Text(kinds.joined(separator: ", "))
                            .font(.caption2)
                            .foregroundStyle(theme.muted)
                            .lineLimit(1)
                    }

                    Spacer()
                }
            }
            .buttonStyle(.plain)
            .padding(.horizontal, 10)
            .padding(.vertical, 5)

            // Expanded: show individual tool calls
            if isExpanded {
                Divider().padding(.horizontal, 10)
                VStack(alignment: .leading, spacing: 2) {
                    ForEach(calls, id: \.toolCallId) { call in
                        ToolCallView(call: call, timestamp: timestamp)
                    }
                }
                .padding(6)
            }
        }
        .background(theme.selBg.opacity(0.3))
        .clipShape(RoundedRectangle(cornerRadius: 6))
        .overlay(
            RoundedRectangle(cornerRadius: 6)
                .strokeBorder(theme.border.opacity(0.5), lineWidth: 1)
        )
        .hoverHighlight()
    }
}

// MARK: - Tool result (hidden, kept for exhaustive switch)

struct ToolResultView: View {
    let result: ToolResultData
    var body: some View { EmptyView() }
}

// MARK: - Permission request row

struct PermissionRequestRow: View {
    let request: PermissionRequestData
    @Environment(\.theme) private var theme

    var body: some View {
        HStack(spacing: 8) {
            Image(systemName: "exclamationmark.shield.fill")
                .foregroundStyle(theme.yellow)
            VStack(alignment: .leading, spacing: 2) {
                Text(request.title)
                    .font(.caption.weight(.medium))
                    .foregroundStyle(theme.yellow)
                if let cmd = request.command {
                    Text(cmd)
                        .font(.system(.caption2, design: .monospaced))
                        .foregroundStyle(theme.muted)
                        .lineLimit(2)
                }
            }
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 6)
        .background(theme.yellow.opacity(0.08))
        .clipShape(RoundedRectangle(cornerRadius: 6))
        .overlay(
            RoundedRectangle(cornerRadius: 6)
                .strokeBorder(theme.yellow.opacity(0.2), lineWidth: 1)
        )
    }
}
