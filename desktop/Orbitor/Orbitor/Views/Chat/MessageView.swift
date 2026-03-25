import SwiftUI

struct MessageView: View {
    let message: ChatMessage
    @Environment(\.theme) private var theme

    var body: some View {
        switch message {
        case .promptSent(_, let text, let ts):
            UserBubble(text: text, timestamp: ts)

        case .agentText(_, let text, let ts):
            AgentBubble(text: text, timestamp: ts)

        case .toolCall(_, let call, let ts):
            ToolCallView(call: call, timestamp: ts)

        case .toolResult(_, let result, _):
            ToolResultView(result: result)

        case .permissionRequest(_, let req, _):
            PermissionRequestRow(request: req)

        case .permissionResolved(_, _, let optionId, _):
            HStack(spacing: 6) {
                Image(systemName: "checkmark.shield")
                    .foregroundStyle(theme.green)
                Text("Permission \(optionId == "allow" ? "approved" : "denied")")
                    .font(.caption)
                    .foregroundStyle(theme.muted)
            }
            .padding(.vertical, 2)

        case .runComplete(_, let reason, _):
            HStack(spacing: 6) {
                Image(systemName: "checkmark.circle")
                    .foregroundStyle(theme.green)
                Text("Run complete")
                    .font(.caption)
                    .foregroundStyle(theme.muted)
                if reason != "end_turn" {
                    Text("(\(reason))")
                        .font(.caption)
                        .foregroundStyle(theme.muted)
                }
            }
            .padding(.vertical, 4)

        case .interrupted:
            HStack(spacing: 6) {
                Image(systemName: "stop.circle")
                    .foregroundStyle(theme.orange)
                Text("Interrupted")
                    .font(.caption)
                    .foregroundStyle(theme.muted)
            }
            .padding(.vertical, 4)

        case .error(_, let msg, _):
            HStack(spacing: 6) {
                Image(systemName: "exclamationmark.triangle")
                    .foregroundStyle(theme.red)
                Text(msg)
                    .font(.caption)
                    .foregroundStyle(theme.red)
            }
            .padding(.vertical, 4)
        }
    }
}

// MARK: - User prompt bubble

struct UserBubble: View {
    let text: String
    let timestamp: Date
    @Environment(\.theme) private var theme

    var body: some View {
        HStack {
            Spacer(minLength: 60)
            VStack(alignment: .trailing, spacing: 4) {
                Text(timestamp, style: .time)
                    .font(.caption2)
                    .foregroundStyle(theme.muted)
                Text(text)
                    .font(.body)
                    .foregroundStyle(theme.text)
                    .padding(.horizontal, 12)
                    .padding(.vertical, 8)
                    .background(theme.accent.opacity(0.15))
                    .clipShape(RoundedRectangle(cornerRadius: 12))
                    .overlay(
                        RoundedRectangle(cornerRadius: 12)
                            .strokeBorder(theme.accent.opacity(0.3), lineWidth: 1)
                    )
            }
        }
    }
}

// MARK: - Agent text bubble

struct AgentBubble: View {
    let text: String
    let timestamp: Date
    @Environment(\.theme) private var theme

    var body: some View {
        HStack {
            VStack(alignment: .leading, spacing: 4) {
                Text(timestamp, style: .time)
                    .font(.caption2)
                    .foregroundStyle(theme.muted)
                MarkdownTextView(text: text)
                    .padding(.horizontal, 12)
                    .padding(.vertical, 8)
                    .background(theme.selBg)
                    .clipShape(RoundedRectangle(cornerRadius: 12))
                    .overlay(
                        RoundedRectangle(cornerRadius: 12)
                            .strokeBorder(theme.border, lineWidth: 1)
                    )
            }
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

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            // Header
            Button {
                withAnimation(.easeInOut(duration: 0.15)) {
                    isExpanded.toggle()
                }
            } label: {
                HStack(spacing: 8) {
                    Image(systemName: isExpanded ? "chevron.down" : "chevron.right")
                        .font(.caption2)
                        .foregroundStyle(theme.muted)

                    StatusBadge(state: call.status)

                    Text(call.title.isEmpty ? call.kind : call.title)
                        .font(.system(.caption, design: .monospaced))
                        .foregroundStyle(theme.cyan)

                    if !call.kind.isEmpty && !call.title.isEmpty {
                        Text(call.kind)
                            .font(.caption2)
                            .foregroundStyle(theme.muted)
                    }

                    Spacer()

                    Text(timestamp, style: .time)
                        .font(.caption2)
                        .foregroundStyle(theme.muted)
                }
            }
            .buttonStyle(.plain)

            // Content (expandable)
            if isExpanded, let content = call.content, !content.isEmpty {
                CodeBlockView(code: content, language: call.kind)
                    .transition(.opacity.combined(with: .scale(scale: 0.98, anchor: .top)))
            }
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 6)
        .background(theme.selBg.opacity(0.5))
        .clipShape(RoundedRectangle(cornerRadius: 8))
        .overlay(
            RoundedRectangle(cornerRadius: 8)
                .strokeBorder(theme.border, lineWidth: 1)
        )
    }
}

// MARK: - Tool result

struct ToolResultView: View {
    let result: ToolResultData
    @Environment(\.theme) private var theme
    @State private var isExpanded = false

    var body: some View {
        if !result.content.isEmpty {
            DisclosureGroup(isExpanded: $isExpanded) {
                CodeBlockView(code: result.content, language: "")
            } label: {
                HStack(spacing: 6) {
                    Image(systemName: "arrow.turn.down.right")
                        .font(.caption2)
                        .foregroundStyle(theme.muted)
                    Text("Result")
                        .font(.caption)
                        .foregroundStyle(theme.muted)
                    Text("\(result.content.count) chars")
                        .font(.caption2)
                        .foregroundStyle(theme.gray)
                }
            }
            .padding(.leading, 20)
        }
    }
}

// MARK: - Permission request row

struct PermissionRequestRow: View {
    let request: PermissionRequestData
    @Environment(\.theme) private var theme

    var body: some View {
        HStack(spacing: 6) {
            Image(systemName: "exclamationmark.shield")
                .foregroundStyle(theme.yellow)
            Text("Permission: \(request.title)")
                .font(.caption)
                .foregroundStyle(theme.yellow)
            if let cmd = request.command {
                Text(cmd)
                    .font(.system(.caption2, design: .monospaced))
                    .foregroundStyle(theme.muted)
                    .lineLimit(1)
            }
        }
        .padding(.vertical, 2)
    }
}
