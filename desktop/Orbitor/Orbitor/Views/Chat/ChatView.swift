import SwiftUI

// MARK: - Turn grouping

/// A display item in the chat — either a single message or a collapsed group of tool calls.
enum DisplayItem: Identifiable {
    case single(ChatMessage)
    case toolGroup(id: UUID, calls: [ToolCallData], timestamp: Date)

    var id: UUID {
        switch self {
        case .single(let msg): return msg.id
        case .toolGroup(let id, _, _): return id
        }
    }
}

/// A turn is a user prompt followed by the agent's response (text, tools, status).
struct Turn: Identifiable {
    let id: UUID
    let items: [DisplayItem]
}

/// Group flat messages into turns with collapsed tool call groups and coalesced agent text.
func buildDisplayItems(from messages: [ChatMessage]) -> [DisplayItem] {
    var items: [DisplayItem] = []
    var pendingTools: [(ToolCallData, Date)] = []
    var pendingText: (id: UUID, text: String, timestamp: Date)?

    func flushTools() {
        guard !pendingTools.isEmpty else { return }
        if pendingTools.count >= 2 {
            items.append(.toolGroup(
                id: UUID(),
                calls: pendingTools.map(\.0),
                timestamp: pendingTools.first!.1
            ))
        } else {
            for (call, ts) in pendingTools {
                items.append(.single(.toolCall(id: UUID(), call: call, timestamp: ts)))
            }
        }
        pendingTools = []
    }

    func flushText() {
        guard let pt = pendingText else { return }
        items.append(.single(.agentText(id: pt.id, text: pt.text, timestamp: pt.timestamp)))
        pendingText = nil
    }

    for msg in messages {
        switch msg {
        case .agentText(let id, let text, let ts):
            flushTools()
            if pendingText != nil {
                pendingText!.text += text
            } else {
                pendingText = (id: id, text: text, timestamp: ts)
            }

        case .toolCall(_, let call, let ts):
            flushText()
            let isDone = call.status == "done" || call.status == "completed" ||
                         call.status == "error" || call.status == "failed"
            if isDone {
                pendingTools.append((call, ts))
            } else {
                flushTools()
                items.append(.single(msg))
            }

        case .toolResult:
            // Hidden — tool call card shows status
            continue

        case .historyBatch:
            continue

        default:
            flushText()
            flushTools()
            items.append(.single(msg))
        }
    }
    flushText()
    flushTools()
    return items
}

// MARK: - Chat View

struct ChatView: View {
    @Environment(AppState.self) private var appState
    @Environment(\.theme) private var theme
    @State private var promptText = ""
    @State private var scrollToBottom = true
    @State private var suppressNextScroll = false

    var body: some View {
        let displayItems = buildDisplayItems(from: appState.chat.messages)

        VStack(spacing: 0) {
            // Permission banner
            if let perm = appState.chat.pendingPermission {
                PermissionBannerView(permission: perm)
            }

            // Message list
            ScrollViewReader { proxy in
                ZStack {
                // Connecting overlay
                if appState.chat.isConnecting && appState.chat.messages.isEmpty {
                    VStack(spacing: 16) {
                        Spacer()
                        ProgressView()
                            .controlSize(.large)
                            .tint(theme.accent)
                        Text("Connecting to session…")
                            .font(.headline)
                            .foregroundStyle(theme.muted)
                        Spacer()
                    }
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                    .background(theme.panel)
                } else if appState.chat.isLoadingHistory && appState.chat.messages.isEmpty {
                    VStack(spacing: 16) {
                        Spacer()
                        ProgressView()
                            .controlSize(.large)
                            .tint(theme.accent)
                        Text("Loading chat history…")
                            .font(.headline)
                            .foregroundStyle(theme.muted)
                        Spacer()
                    }
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                    .background(theme.panel)
                }

                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 4) {
                        // Load-more trigger at the top
                        if appState.chat.hasMoreHistory {
                            Button {
                                suppressNextScroll = true
                                let firstVisibleID = displayItems.first?.id
                                appState.chat.loadMoreHistory()
                                if let id = firstVisibleID {
                                    DispatchQueue.main.asyncAfter(deadline: .now() + 0.05) {
                                        proxy.scrollTo(id, anchor: .top)
                                    }
                                }
                            } label: {
                                HStack(spacing: 6) {
                                    Image(systemName: "arrow.up.circle")
                                    Text("Load earlier messages")
                                }
                                .font(.caption)
                                .foregroundStyle(theme.muted)
                                .frame(maxWidth: .infinity)
                                .padding(.vertical, 8)
                            }
                            .buttonStyle(.plain)
                            .id("load-more")
                        }

                        ForEach(displayItems) { item in
                            DisplayItemView(item: item)
                                .id(item.id)
                        }

                        // Anchor for scroll-to-bottom
                        Color.clear
                            .frame(height: 1)
                            .id("bottom")
                    }
                    .padding()
                }
                .background(theme.panel)
                .onChange(of: appState.chat.messages.count) {
                    if suppressNextScroll {
                        suppressNextScroll = false
                        return
                    }
                    if scrollToBottom {
                        withAnimation(.easeOut(duration: 0.2)) {
                            proxy.scrollTo("bottom", anchor: .bottom)
                        }
                    }
                }
                } // ZStack
            }

            Divider().background(theme.sep)

            // Working indicator
            if appState.chat.isRunning || appState.chat.isConnecting || appState.chat.isLoadingHistory {
                WorkingIndicator(
                    isConnecting: appState.chat.isConnecting || appState.chat.isLoadingHistory,
                    queuedPrompts: appState.chat.queuedPrompts,
                    onRemoveQueued: { index in
                        appState.chat.removeQueuedPrompt(at: index)
                    }
                )
            }

            // Input area
            PromptInputView(text: $promptText, onSubmit: {
                Task {
                    let text = promptText
                    promptText = ""
                    await appState.chat.sendPrompt(text)
                }
            }, onForkSubmit: {
                guard let sourceID = appState.sessionList.selectedSessionID else { return }
                let text = promptText
                promptText = ""
                Task {
                    await appState.sessionList.forkSession(sourceID: sourceID, prompt: text)
                }
            })
        }
        .background(theme.panel)
    }
}

// MARK: - Display Item View

struct DisplayItemView: View {
    let item: DisplayItem
    @Environment(\.theme) private var theme

    var body: some View {
        switch item {
        case .single(let msg):
            MessageView(message: msg)
        case .toolGroup(_, let calls, let timestamp):
            CollapsedToolGroup(calls: calls, timestamp: timestamp)
        }
    }
}

// MARK: - Permission Banner

struct PermissionBannerView: View {
    let permission: PermissionRequestData
    @Environment(AppState.self) private var appState
    @Environment(\.theme) private var theme

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: "exclamationmark.shield")
                .font(.title3)
                .foregroundStyle(theme.yellow)

            VStack(alignment: .leading, spacing: 2) {
                Text(permission.title)
                    .font(.headline)
                    .foregroundStyle(theme.text)
                if let cmd = permission.command {
                    Text(cmd)
                        .font(.system(.caption, design: .monospaced))
                        .foregroundStyle(theme.muted)
                        .lineLimit(2)
                }
            }

            Spacer()

            ForEach(permission.options) { option in
                Button(option.name) {
                    Task {
                        await appState.chat.respondToPermission(
                            requestId: permission.requestId,
                            optionId: option.optionId
                        )
                    }
                }
                .buttonStyle(.bordered)
                .tint(option.kind == "allow" ? theme.green : theme.red)
            }
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
        .background(theme.yellow.opacity(0.1))
        .overlay(alignment: .bottom) {
            Divider().background(theme.yellow)
        }
    }
}

// MARK: - Working Indicator

private struct SlidingGradientBar: View {
    let phase: CGFloat
    @Environment(\.theme) private var theme

    var body: some View {
        GeometryReader { geo in
            let w = geo.size.width
            Capsule()
                .fill(
                    LinearGradient(
                        colors: [
                            theme.accent.opacity(0),
                            theme.accent.opacity(0.6),
                            theme.cyan.opacity(0.8),
                            theme.accent.opacity(0.6),
                            theme.accent.opacity(0),
                        ],
                        startPoint: .leading,
                        endPoint: .trailing
                    )
                )
                .frame(width: w * 0.4, height: 2)
                .offset(x: -w * 0.2 + phase * (w * 1.2))
        }
        .frame(height: 2)
        .clipped()
    }
}

private struct WorkingStatusLabel: View {
    let isConnecting: Bool
    let label: String
    let phase: CGFloat
    let hasQueue: Bool
    let showQueue: Bool
    @Environment(\.theme) private var theme

    var body: some View {
        HStack(spacing: 6) {
            if isConnecting {
                ProgressView()
                    .controlSize(.small)
                    .tint(theme.accent)
            } else {
                Circle()
                    .fill(theme.accent)
                    .frame(width: 6, height: 6)
                    .opacity(pulseOpacity)
            }
            Text(label)
                .font(.caption)
                .foregroundStyle(theme.muted)

            if hasQueue {
                Image(systemName: showQueue ? "chevron.down" : "chevron.up")
                    .font(.caption2)
                    .foregroundStyle(theme.muted)
            }
        }
        .padding(.vertical, 4)
        .frame(maxWidth: .infinity)
        .contentShape(Rectangle())
    }

    private var pulseOpacity: Double {
        let sinVal = sin(Double(phase) * .pi * 2)
        return 0.4 + 0.6 * ((sinVal + 1.0) / 2.0)
    }
}

struct WorkingIndicator: View {
    var isConnecting: Bool
    var queuedPrompts: [String]
    var onRemoveQueued: (Int) -> Void
    @Environment(\.theme) private var theme
    @State private var phase: CGFloat = 0
    @State private var showQueue = false

    private var label: String {
        if isConnecting {
            return "Connecting…"
        } else if !queuedPrompts.isEmpty {
            return "Working… (\(queuedPrompts.count) queued)"
        } else {
            return "Working…"
        }
    }

    var body: some View {
        VStack(spacing: 0) {
            SlidingGradientBar(phase: phase)

            Button {
                if !queuedPrompts.isEmpty {
                    withAnimation(.easeInOut(duration: 0.15)) {
                        showQueue.toggle()
                    }
                }
            } label: {
                WorkingStatusLabel(
                    isConnecting: isConnecting,
                    label: label,
                    phase: phase,
                    hasQueue: !queuedPrompts.isEmpty,
                    showQueue: showQueue
                )
            }
            .buttonStyle(.plain)

            if showQueue && !queuedPrompts.isEmpty {
                QueuedPromptsList(prompts: queuedPrompts, onRemove: onRemoveQueued)
                    .transition(.opacity.combined(with: .move(edge: .bottom)))
            }
        }
        .background(theme.panel)
        .onAppear {
            withAnimation(.linear(duration: 1.8).repeatForever(autoreverses: false)) {
                phase = 1
            }
        }
        .onChange(of: queuedPrompts.count) { _, newCount in
            if newCount == 0 { showQueue = false }
        }
    }
}

// MARK: - Queued Prompts List

struct QueuedPromptsList: View {
    let prompts: [String]
    let onRemove: (Int) -> Void
    @Environment(\.theme) private var theme

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Divider().background(theme.border)

            ForEach(Array(prompts.enumerated()), id: \.offset) { index, prompt in
                QueuedPromptRow(index: index, prompt: prompt, isLast: index == prompts.count - 1, onRemove: onRemove)
            }
        }
        .background(theme.selBg.opacity(0.5))
    }
}

struct QueuedPromptRow: View {
    let index: Int
    let prompt: String
    let isLast: Bool
    let onRemove: (Int) -> Void
    @Environment(\.theme) private var theme

    var body: some View {
        VStack(spacing: 0) {
            HStack(spacing: 8) {
                Text("#\(index + 1)")
                    .font(.system(.caption2, design: .monospaced))
                    .foregroundStyle(theme.accent)
                    .frame(width: 20, alignment: .trailing)

                Text(prompt)
                    .font(.system(.caption, design: .monospaced))
                    .foregroundStyle(theme.text)
                    .lineLimit(2)
                    .truncationMode(.tail)

                Spacer()

                Button {
                    withAnimation(.easeOut(duration: 0.15)) {
                        onRemove(index)
                    }
                } label: {
                    Image(systemName: "xmark.circle.fill")
                        .font(.caption)
                        .foregroundStyle(theme.red.opacity(0.7))
                }
                .buttonStyle(.plain)
                .help("Remove from queue")
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 4)

            if !isLast {
                Divider()
                    .padding(.horizontal, 12)
            }
        }
    }
}
