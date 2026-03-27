import SwiftUI
import UniformTypeIdentifiers

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
    @State private var pendingAttachments: [PromptAttachment] = []
    @State private var scrollToBottom = true
    @State private var suppressNextScroll = false
    @State private var isAtBottom = true
    @State private var showRunCard = false

    var body: some View {
        let displayItems = buildDisplayItems(from: appState.chat.messages)

        VStack(spacing: 0) {
            // Permission banner
            if let perm = appState.chat.pendingPermission {
                PermissionBannerView(permission: perm)
            }

            // Message list
            ScrollViewReader { proxy in
                ZStack(alignment: .bottomTrailing) {
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
                            .onAppear { isAtBottom = true }
                            .onDisappear { isAtBottom = false }
                    }
                    .padding()
                }
                .background(theme.panel)
                .onDrop(of: [UTType.fileURL, UTType.image, UTType.png, UTType.jpeg], isTargeted: nil) { providers in
                    handleDrop(providers)
                    return true
                }
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
                .onChange(of: appState.chat.activeSessionID) {
                    DispatchQueue.main.asyncAfter(deadline: .now() + 0.1) {
                        proxy.scrollTo("bottom", anchor: .bottom)
                    }
                }

                // Jump-to-bottom floating button
                if !isAtBottom {
                    Button {
                        withAnimation(.easeOut(duration: 0.2)) {
                            proxy.scrollTo("bottom", anchor: .bottom)
                        }
                    } label: {
                        Image(systemName: "arrow.down.circle.fill")
                            .font(.title2)
                            .foregroundStyle(theme.accent)
                            .shadow(color: .black.opacity(0.3), radius: 4, x: 0, y: 2)
                    }
                    .buttonStyle(.plain)
                    .padding(12)
                    .transition(.scale.combined(with: .opacity))
                }
                } // ZStack
            }

            Divider().background(theme.sep)

            // Post-run card (shown after a run completes)
            if showRunCard && !appState.chat.isRunning {
                RunCompleteCard(
                    duration: appState.chat.lastRunDuration,
                    filesTouched: appState.chat.filesTouched,
                    sessionID: appState.chat.activeSessionID,
                    onDismiss: { showRunCard = false }
                )
            }

            // Working indicator
            if appState.chat.isRunning || appState.chat.isConnecting || appState.chat.isLoadingHistory || appState.chat.isReconnecting {
                WorkingIndicator(
                    isConnecting: appState.chat.isConnecting || appState.chat.isLoadingHistory,
                    isReconnecting: appState.chat.isReconnecting,
                    queuedPrompts: appState.chat.queuedPrompts,
                    onRemoveQueued: { index in
                        appState.chat.removeQueuedPrompt(at: index)
                    }
                )
            }

            // Input area
            PromptInputView(
                text: $promptText,
                attachments: $pendingAttachments,
                onSubmit: { fullText in
                    showRunCard = false
                    Task { await appState.chat.sendPrompt(fullText) }
                },
                onForkSubmit: { fullText in
                    guard let sourceID = appState.sessionList.selectedSessionID else { return }
                    showRunCard = false
                    Task { await appState.sessionList.forkSession(sourceID: sourceID, prompt: fullText) }
                }
            )
        }
        .background(theme.panel)
        .onChange(of: appState.chat.isRunning) { _, running in
            if !running && appState.chat.lastRunDuration != nil {
                showRunCard = true
            }
        }
    }

    // MARK: - Drop handling

    private func handleDrop(_ providers: [NSItemProvider]) {
        // Use a temporary PromptInputView-style helper via a local closure
        let imageExts: Set<String> = ["png", "jpg", "jpeg", "gif", "webp", "heic", "bmp", "tiff", "tif"]

        for provider in providers {
            if provider.hasItemConformingToTypeIdentifier(UTType.fileURL.identifier) {
                provider.loadItem(forTypeIdentifier: UTType.fileURL.identifier, options: nil) { item, _ in
                    guard let data = item as? Data,
                          let urlStr = String(data: data, encoding: .utf8),
                          let url = URL(string: urlStr) else { return }
                    let ext = url.pathExtension.lowercased()
                    if imageExts.contains(ext) {
                        let tempURL = URL(fileURLWithPath: NSTemporaryDirectory())
                            .appendingPathComponent("orbitor_\(UUID().uuidString).\(ext)")
                        try? FileManager.default.copyItem(at: url, to: tempURL)
                        let thumb = NSImage(contentsOf: url)
                        DispatchQueue.main.async {
                            pendingAttachments.append(PromptAttachment(
                                name: url.lastPathComponent,
                                content: tempURL.path,
                                isImage: true,
                                thumbnail: thumb
                            ))
                        }
                    } else {
                        guard let attrs = try? FileManager.default.attributesOfItem(atPath: url.path),
                              let size = attrs[.size] as? Int, size < 500_000,
                              let content = try? String(contentsOf: url, encoding: .utf8) else { return }
                        DispatchQueue.main.async {
                            pendingAttachments.append(PromptAttachment(
                                name: url.lastPathComponent,
                                content: content,
                                isImage: false,
                                thumbnail: nil
                            ))
                        }
                    }
                }
            } else {
                // Direct image drop (e.g. from browser or image viewer)
                let typeID = UTType.png.identifier
                provider.loadDataRepresentation(forTypeIdentifier: typeID) { data, _ in
                    guard let data, let image = NSImage(data: data) else { return }
                    let tempURL = URL(fileURLWithPath: NSTemporaryDirectory())
                        .appendingPathComponent("orbitor_drop_\(UUID().uuidString).png")
                    try? data.write(to: tempURL)
                    DispatchQueue.main.async {
                        pendingAttachments.append(PromptAttachment(
                            name: "dropped_image.png",
                            content: tempURL.path,
                            isImage: true,
                            thumbnail: image
                        ))
                    }
                }
            }
        }
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
    var isReconnecting: Bool = false
    var queuedPrompts: [String]
    var onRemoveQueued: (Int) -> Void
    @Environment(\.theme) private var theme
    @State private var phase: CGFloat = 0
    @State private var showQueue = false

    private var label: String {
        if isReconnecting {
            return "Reconnecting…"
        } else if isConnecting {
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

// MARK: - Run Complete Card

private struct RunCompleteCard: View {
    let duration: TimeInterval?
    let filesTouched: [String]
    let sessionID: String?
    let onDismiss: () -> Void
    @Environment(AppState.self) private var appState
    @Environment(\.theme) private var theme
    @State private var debriefText: String? = nil
    @State private var isLoadingDebrief = false
    @State private var expanded = false

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Divider().background(theme.border)
            HStack(spacing: 10) {
                Image(systemName: "checkmark.circle.fill")
                    .foregroundStyle(theme.green)
                    .font(.callout)

                if let dur = duration {
                    Text("Run complete · \(formatDuration(dur))")
                        .font(.caption.weight(.medium))
                        .foregroundStyle(theme.text)
                }

                if !filesTouched.isEmpty {
                    Text("· \(filesTouched.count) file\(filesTouched.count == 1 ? "" : "s") changed")
                        .font(.caption)
                        .foregroundStyle(theme.muted)
                }

                Spacer()

                if sessionID != nil {
                    Button {
                        // Check before toggling: if we're about to expand and haven't loaded yet, fetch
                        let willExpand = !expanded
                        withAnimation(.easeInOut(duration: 0.15)) {
                            expanded.toggle()
                        }
                        if willExpand && debriefText == nil {
                            loadDebrief()
                        }
                    } label: {
                        HStack(spacing: 3) {
                            Image(systemName: "sparkles")
                                .font(.caption2)
                            Text("Debrief")
                                .font(.caption)
                        }
                        .foregroundStyle(theme.accent)
                    }
                    .buttonStyle(.plain)
                }

                Button {
                    onDismiss()
                } label: {
                    Image(systemName: "xmark")
                        .font(.caption2)
                        .foregroundStyle(theme.muted)
                }
                .buttonStyle(.plain)
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 6)

            if expanded {
                VStack(alignment: .leading, spacing: 6) {
                    if !filesTouched.isEmpty {
                        Text("Files changed:")
                            .font(.caption.weight(.semibold))
                            .foregroundStyle(theme.muted)
                        ForEach(filesTouched.prefix(8), id: \.self) { path in
                            HStack(spacing: 4) {
                                Image(systemName: "pencil.circle.fill")
                                    .font(.system(size: 9))
                                    .foregroundStyle(theme.cyan)
                                Text(path)
                                    .font(.system(size: 11, design: .monospaced))
                                    .foregroundStyle(theme.text)
                                    .lineLimit(1)
                                    .truncationMode(.middle)
                            }
                        }
                        if filesTouched.count > 8 {
                            Text("…and \(filesTouched.count - 8) more")
                                .font(.caption2)
                                .foregroundStyle(theme.muted)
                        }
                    }

                    if isLoadingDebrief {
                        HStack(spacing: 6) {
                            ProgressView().controlSize(.mini)
                            Text("Generating summary…")
                                .font(.caption)
                                .foregroundStyle(theme.muted)
                        }
                    } else if let text = debriefText, !text.isEmpty {
                        Divider().background(theme.border)
                        ScrollView {
                            MarkdownTextView(text: text)
                                .frame(maxWidth: .infinity, alignment: .leading)
                        }
                        .frame(maxHeight: 180)
                    } else if !isLoadingDebrief {
                        Text("No summary available.")
                            .font(.caption)
                            .foregroundStyle(theme.muted)
                    }
                }
                .padding(.horizontal, 12)
                .padding(.bottom, 8)
            }
        }
        .background(theme.green.opacity(0.05))
        .animation(.easeInOut(duration: 0.15), value: expanded)
    }

    private func loadDebrief() {
        guard let id = sessionID else { return }
        isLoadingDebrief = true
        Task {
            let text = try? await appState.api.sessionDebrief(id: id)
            await MainActor.run {
                debriefText = text
                isLoadingDebrief = false
            }
        }
    }

    private func formatDuration(_ t: TimeInterval) -> String {
        let total = Int(t)
        if total < 60 { return "\(total)s" }
        return "\(total / 60)m \(total % 60)s"
    }
}
