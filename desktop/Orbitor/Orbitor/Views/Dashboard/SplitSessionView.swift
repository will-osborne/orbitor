import SwiftUI

// MARK: - Split Session View

struct SplitSessionView: View {
    let leftSessionID: String
    let rightSessionID: String
    @Environment(AppState.self) private var appState
    @Environment(\.theme) private var theme
    @State private var leftChat: ChatState?
    @State private var rightChat: ChatState?

    var body: some View {
        HSplitView {
            SplitPaneView(
                sessionID: leftSessionID,
                chatState: leftChat,
                label: "Left"
            )

            Divider().background(theme.border)

            SplitPaneView(
                sessionID: rightSessionID,
                chatState: rightChat,
                label: "Right"
            )
        }
        .frame(minWidth: 900, minHeight: 500)
        .background(theme.panel)
        .onAppear {
            guard let baseURL = URL(string: appState.serverURL) else { return }

            let left = ChatState(baseURL: baseURL)
            left.connectToSession(leftSessionID)
            leftChat = left

            let right = ChatState(baseURL: baseURL)
            right.connectToSession(rightSessionID)
            rightChat = right
        }
    }
}

// MARK: - Single Pane

private struct SplitPaneView: View {
    let sessionID: String
    let chatState: ChatState?
    let label: String
    @Environment(\.theme) private var theme
    @State private var promptText = ""

    var body: some View {
        VStack(spacing: 0) {
            // Header bar
            SplitPaneHeader(sessionID: sessionID, chatState: chatState)

            Divider().background(theme.sep)

            // Permission banner
            if let perm = chatState?.pendingPermission {
                SplitPanePermissionBanner(permission: perm, chatState: chatState)
            }

            // Message list
            if let chat = chatState, !chat.messages.isEmpty {
                ScrollViewReader { proxy in
                    ScrollView {
                        LazyVStack(alignment: .leading, spacing: 4) {
                            ForEach(chat.messages) { msg in
                                SplitPaneMessageView(message: msg)
                                    .id(msg.id)
                            }
                            Color.clear
                                .frame(height: 1)
                                .id("pane-bottom-\(label)")
                        }
                        .padding(8)
                    }
                    .background(theme.panel)
                    .onChange(of: chat.messages.count) {
                        withAnimation(.easeOut(duration: 0.2)) {
                            proxy.scrollTo("pane-bottom-\(label)", anchor: .bottom)
                        }
                    }
                }
            } else {
                Spacer()
                VStack(spacing: 8) {
                    if chatState?.isConnecting == true {
                        ProgressView()
                            .controlSize(.small)
                            .tint(theme.accent)
                        Text("Connecting...")
                            .font(.caption)
                            .foregroundStyle(theme.muted)
                    } else {
                        Image(systemName: "bubble.left.and.bubble.right")
                            .font(.title2)
                            .foregroundStyle(theme.muted)
                        Text("Waiting for messages")
                            .font(.caption)
                            .foregroundStyle(theme.muted)
                    }
                }
                Spacer()
            }

            Divider().background(theme.sep)

            // Working indicator
            if chatState?.isRunning == true {
                HStack(spacing: 6) {
                    Circle()
                        .fill(theme.accent)
                        .frame(width: 6, height: 6)
                    Text("Working...")
                        .font(.caption)
                        .foregroundStyle(theme.muted)
                }
                .padding(.vertical, 4)
                .frame(maxWidth: .infinity)
            }

            // Input area
            HStack(spacing: 8) {
                TextField("Send prompt...", text: $promptText)
                    .textFieldStyle(.plain)
                    .font(.system(size: 12))
                    .padding(.horizontal, 8)
                    .padding(.vertical, 6)
                    .background(theme.selBg)
                    .clipShape(RoundedRectangle(cornerRadius: 6))
                    .overlay(
                        RoundedRectangle(cornerRadius: 6)
                            .strokeBorder(theme.border, lineWidth: 1)
                    )
                    .onSubmit {
                        sendPrompt()
                    }

                Button {
                    sendPrompt()
                } label: {
                    Image(systemName: "arrow.up.circle.fill")
                        .font(.title3)
                        .foregroundStyle(promptText.isEmpty ? theme.muted : theme.accent)
                }
                .buttonStyle(.plain)
                .disabled(promptText.isEmpty)
            }
            .padding(8)
        }
        .background(theme.panel)
    }

    private func sendPrompt() {
        let text = promptText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty, let chat = chatState else { return }
        promptText = ""
        Task { await chat.sendPrompt(text) }
    }
}

// MARK: - Pane Header

private struct SplitPaneHeader: View {
    let sessionID: String
    let chatState: ChatState?
    @Environment(\.theme) private var theme

    private var statusColor: Color {
        guard let chat = chatState else { return theme.muted }
        if chat.isRunning { return theme.orange }
        if chat.isConnecting || chat.isReconnecting { return theme.yellow }
        if chat.activeSessionID != nil { return theme.green }
        return theme.muted
    }

    private var currentTool: String? {
        guard let chat = chatState else { return nil }
        // Find the last in-progress tool call
        for msg in chat.messages.reversed() {
            if case .toolCall(_, let call, _) = msg,
               call.status == "running" || call.status == "in_progress" {
                return call.title.isEmpty ? call.kind : call.title
            }
        }
        return nil
    }

    var body: some View {
        HStack(spacing: 8) {
            Circle()
                .fill(statusColor)
                .frame(width: 8, height: 8)

            Text(sessionID)
                .font(.system(size: 11, weight: .medium, design: .monospaced))
                .foregroundStyle(theme.text)
                .lineLimit(1)
                .truncationMode(.middle)

            if let tool = currentTool {
                Text("·")
                    .foregroundStyle(theme.muted)
                Text(tool)
                    .font(.system(size: 10, design: .monospaced))
                    .foregroundStyle(theme.cyan)
                    .lineLimit(1)
            }

            Spacer()
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 6)
        .background(theme.selBg.opacity(0.5))
    }
}

// MARK: - Compact Message View

private struct SplitPaneMessageView: View {
    let message: ChatMessage
    @Environment(\.theme) private var theme

    var body: some View {
        switch message {
        case .agentText(_, let text, _):
            Text(text)
                .font(.system(size: 12))
                .foregroundStyle(theme.text)
                .textSelection(.enabled)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal, 6)
                .padding(.vertical, 2)

        case .toolCall(_, let call, _):
            SplitPaneToolCallRow(call: call)

        case .promptSent(_, let text, _):
            HStack {
                Spacer()
                Text(text)
                    .font(.system(size: 12))
                    .foregroundStyle(theme.text)
                    .lineLimit(4)
                    .padding(.horizontal, 10)
                    .padding(.vertical, 4)
                    .background(theme.accent.opacity(0.12))
                    .clipShape(Capsule())
                    .overlay(
                        Capsule().strokeBorder(theme.accent.opacity(0.25), lineWidth: 1)
                    )
            }

        case .runComplete(_, let reason, _):
            HStack(spacing: 4) {
                Image(systemName: "checkmark.circle.fill")
                    .font(.caption2)
                    .foregroundStyle(theme.green)
                Text(reason == "end_turn" ? "Run complete" : "Done — \(reason)")
                    .font(.system(size: 11))
                    .foregroundStyle(theme.green)
            }
            .frame(maxWidth: .infinity, alignment: .center)
            .padding(.vertical, 2)

        case .error(_, let msg, _):
            HStack(spacing: 4) {
                Image(systemName: "exclamationmark.triangle.fill")
                    .font(.caption2)
                    .foregroundStyle(theme.red)
                Text(msg)
                    .font(.system(size: 11))
                    .foregroundStyle(theme.red)
                    .lineLimit(2)
            }
            .padding(.horizontal, 6)
            .padding(.vertical, 2)

        case .interrupted:
            HStack(spacing: 4) {
                Image(systemName: "stop.circle.fill")
                    .font(.caption2)
                    .foregroundStyle(theme.orange)
                Text("Interrupted")
                    .font(.system(size: 11))
                    .foregroundStyle(theme.orange)
            }
            .frame(maxWidth: .infinity, alignment: .center)
            .padding(.vertical, 2)

        default:
            EmptyView()
        }
    }
}

// MARK: - Tool Call Row

private struct SplitPaneToolCallRow: View {
    let call: ToolCallData
    @Environment(\.theme) private var theme

    private var statusIcon: (String, Color) {
        switch call.status {
        case "done", "completed": return ("checkmark.circle.fill", theme.green)
        case "error", "failed": return ("xmark.circle.fill", theme.red)
        case "running", "in_progress": return ("arrow.triangle.2.circlepath", theme.orange)
        default: return ("circle.dashed", theme.muted)
        }
    }

    var body: some View {
        HStack(spacing: 4) {
            Image(systemName: statusIcon.0)
                .font(.system(size: 9))
                .foregroundStyle(statusIcon.1)

            Text(call.title.isEmpty ? call.kind : call.title)
                .font(.system(size: 11, design: .monospaced))
                .foregroundStyle(theme.cyan)
                .lineLimit(1)

            if !call.kind.isEmpty && !call.title.isEmpty {
                Text(call.kind)
                    .font(.system(size: 10))
                    .foregroundStyle(theme.muted)
            }

            Spacer()
        }
        .padding(.horizontal, 6)
        .padding(.vertical, 2)
        .background(theme.selBg.opacity(0.3))
        .clipShape(RoundedRectangle(cornerRadius: 4))
    }
}

// MARK: - Compact Permission Banner

private struct SplitPanePermissionBanner: View {
    let permission: PermissionRequestData
    let chatState: ChatState?
    @Environment(\.theme) private var theme

    var body: some View {
        HStack(spacing: 8) {
            Image(systemName: "exclamationmark.shield")
                .font(.caption)
                .foregroundStyle(theme.yellow)

            VStack(alignment: .leading, spacing: 1) {
                Text(permission.title)
                    .font(.system(size: 11, weight: .medium))
                    .foregroundStyle(theme.text)
                    .lineLimit(1)
                if let cmd = permission.command {
                    Text(cmd)
                        .font(.system(size: 10, design: .monospaced))
                        .foregroundStyle(theme.muted)
                        .lineLimit(1)
                }
            }

            Spacer()

            ForEach(permission.options) { option in
                Button(option.name) {
                    guard let chat = chatState else { return }
                    Task {
                        await chat.respondToPermission(
                            requestId: permission.requestId,
                            optionId: option.optionId
                        )
                    }
                }
                .controlSize(.small)
                .buttonStyle(.bordered)
                .tint(option.kind == "allow" ? theme.green : theme.red)
            }
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 6)
        .background(theme.yellow.opacity(0.1))
    }
}
