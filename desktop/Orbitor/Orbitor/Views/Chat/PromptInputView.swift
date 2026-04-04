import AppKit
import SwiftUI
import UniformTypeIdentifiers

// MARK: - Attachment model

struct PromptAttachment: Identifiable {
    let id = UUID()
    let name: String
    /// For images: path to the temp file. For text: the file content to embed.
    let content: String
    let isImage: Bool
    let thumbnail: NSImage?
}

// MARK: - Prompt input view

struct PromptInputView: View {
    @Binding var text: String
    @Binding var attachments: [PromptAttachment]
    /// Called with the fully-built prompt text (includes attachment content).
    var onSubmit: (String) -> Void
    var onForkSubmit: ((String) -> Void)?
    @Environment(AppState.self) private var appState
    @Environment(\.theme) private var theme
    @FocusState private var isFocused: Bool
    @State private var historyIndex = -1
    @State private var suggestions: [String] = []
    @State private var isEnhancing = false
    @State private var routedSessionIDs: [String] = []
    @State private var routeTask: Task<Void, Never>? = nil

    private var estimatedTokens: Int { max(1, text.count / 4) }

    var body: some View {
        VStack(spacing: 0) {
            // Prompt routing suggestion (shown when AI suggests a different session)
            if let targetID = routedSessionIDs.first,
               targetID != appState.sessionList.selectedSessionID,
               let targetSession = appState.sessionList.sessions.first(where: { $0.id == targetID }) {
                HStack(spacing: 8) {
                    Image(systemName: "sparkles")
                        .font(.caption2)
                        .foregroundStyle(theme.violet)
                    Text("This might belong in:")
                        .font(.caption)
                        .foregroundStyle(theme.muted)
                    Text(targetSession.displayTitle)
                        .font(.caption.weight(.medium))
                        .foregroundStyle(theme.text)
                        .lineLimit(1)
                    Spacer()
                    Button {
                        appState.sessionList.selectedSessionID = targetID
                        routedSessionIDs = []
                    } label: {
                        Text("Switch")
                            .font(.caption.weight(.medium))
                            .foregroundStyle(theme.violet)
                            .padding(.horizontal, 8)
                            .padding(.vertical, 3)
                            .background(theme.violet.opacity(0.15))
                            .clipShape(RoundedRectangle(cornerRadius: 4))
                    }
                    .buttonStyle(.plain)
                    Button {
                        routedSessionIDs = []
                    } label: {
                        Image(systemName: "xmark")
                            .font(.system(size: 9))
                            .foregroundStyle(theme.muted)
                    }
                    .buttonStyle(.plain)
                }
                .padding(.horizontal, 12)
                .padding(.vertical, 5)
                .background(theme.violet.opacity(0.06))
            }

            // Suggestion chips (shown when input is empty and suggestions are loaded)
            if text.isEmpty && !suggestions.isEmpty {
                SuggestionChipsView(suggestions: suggestions, theme: theme) { chip in
                    text = chip
                    suggestions = []
                    isFocused = true
                }
                .padding(.horizontal, 12)
                .padding(.top, 6)
            }

            // Attachment chips (shown when attachments are present)
            if !attachments.isEmpty {
                AttachmentChipsView(attachments: $attachments, theme: theme)
                    .padding(.horizontal, 12)
                    .padding(.top, 6)
            }

            HStack(alignment: .bottom, spacing: 8) {
                ZStack(alignment: .topLeading) {
                    if text.isEmpty && !appState.dictation.isRecording {
                        Text("Type a prompt and press ⌘Enter…")
                            .foregroundStyle(theme.muted)
                            .padding(.horizontal, 8)
                            .padding(.top, 8)
                    }
                    if appState.dictation.isRecording {
                        HStack(spacing: 8) {
                            Image(systemName: "mic.fill")
                                .foregroundStyle(theme.red)
                                .symbolEffect(.pulse)
                            Text(appState.dictation.transcribedText.isEmpty
                                 ? "Listening… (release Space to stop)"
                                 : appState.dictation.transcribedText)
                                .foregroundStyle(theme.text)
                                .italic()
                        }
                        .padding(.horizontal, 8)
                        .padding(.vertical, 10)
                        .frame(maxWidth: .infinity, minHeight: 36, alignment: .leading)
                    } else {
                        TextEditor(text: $text)
                            .font(.system(size: appState.fontSize, design: .monospaced))
                            .foregroundStyle(theme.text)
                            .scrollContentBackground(.hidden)
                            .focused($isFocused)
                            .frame(minHeight: 36, maxHeight: 120)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                }
                .padding(4)
                .background(appState.dictation.isRecording ? theme.red.opacity(0.1) : theme.selBg)
                .clipShape(RoundedRectangle(cornerRadius: 8))
                .overlay(
                    RoundedRectangle(cornerRadius: 8)
                        .strokeBorder(
                            appState.dictation.isRecording ? theme.red :
                                (isFocused ? theme.accent : theme.border),
                            lineWidth: 1
                        )
                )

                HStack(spacing: 6) {
                    // File attachment button
                    Button {
                        openFilePicker()
                    } label: {
                        Image(systemName: "paperclip")
                            .font(.body)
                            .foregroundStyle(attachments.isEmpty ? theme.muted : theme.accent)
                    }
                    .buttonStyle(.plain)
                    .hoverScale(1.15)
                    .help("Attach file")

                    // Dictation button
                    if appState.dictation.isAvailable {
                        Button {
                            if appState.dictation.isRecording {
                                let result = appState.dictation.stopRecording()
                                if !result.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                                    text += result
                                }
                            } else {
                                appState.dictation.startRecording()
                            }
                        } label: {
                            Image(systemName: appState.dictation.isRecording ? "mic.fill" : "mic")
                                .font(.body)
                                .foregroundStyle(appState.dictation.isRecording ? theme.red : theme.muted)
                        }
                        .buttonStyle(.plain)
                        .hoverScale(1.15)
                        .help(appState.dictation.isRecording ? "Stop dictation" : "Start dictation (or hold Space)")
                    }

                    // Prompt enhancer button
                    if !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                        Button {
                            enhancePrompt()
                        } label: {
                            if isEnhancing {
                                ProgressView().controlSize(.mini).tint(theme.violet)
                            } else {
                                Image(systemName: "sparkles")
                                    .font(.body)
                                    .foregroundStyle(theme.violet)
                            }
                        }
                        .buttonStyle(.plain)
                        .hoverScale(1.15)
                        .disabled(isEnhancing)
                        .help("Enhance prompt with AI (⌘E)")
                        .keyboardShortcut("e", modifiers: .command)
                    }

                    // Char/token counter (shown when text is non-empty)
                    if !text.isEmpty {
                        Text("~\(estimatedTokens)t")
                            .font(.system(size: 10, design: .monospaced))
                            .foregroundStyle(estimatedTokens > 2000 ? theme.orange : theme.muted.opacity(0.6))
                            .help("\(text.count) characters · ~\(estimatedTokens) tokens")
                    }

                    // Fork button (Option+Enter)
                    if appState.sessionList.selectedSessionID != nil {
                        Button {
                            doForkSubmit()
                        } label: {
                            Image(systemName: "arrow.triangle.branch")
                                .font(.body)
                                .foregroundStyle(text.isEmpty ? theme.muted : theme.cyan)
                        }
                        .buttonStyle(.plain)
                        .hoverScale(1.15)
                        .disabled(text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && attachments.isEmpty)
                        .keyboardShortcut(.return, modifiers: .option)
                        .help("Fork & Send (⌥Enter)")
                    }

                    // Interrupt button
                    if appState.chat.isRunning {
                        Button {
                            Task { await appState.chat.interrupt() }
                        } label: {
                            Image(systemName: "stop.circle.fill")
                                .font(.body)
                                .foregroundStyle(theme.orange)
                        }
                        .buttonStyle(.plain)
                        .hoverScale(1.15)
                        .help("Interrupt (⌘.)")
                    }

                    // Send button (Cmd+Enter)
                    Button {
                        doSubmit()
                    } label: {
                        Image(systemName: "arrow.up.circle.fill")
                            .font(.title3)
                            .foregroundStyle((text.isEmpty && attachments.isEmpty) ? theme.muted : theme.accent)
                    }
                    .buttonStyle(.plain)
                    .hoverScale()
                    .disabled(text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && attachments.isEmpty)
                    .keyboardShortcut(.return, modifiers: .command)
                    .help("Send (⌘Enter)")
                }
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 8)
        }
        .background(theme.panel)
        // Handle image paste (text is handled natively by TextEditor)
        .onPasteCommand(of: [UTType.image, UTType.png, UTType.jpeg, UTType.tiff]) { providers in
            handleImagePaste(providers)
        }
        // Prompt history navigation: ⌘↑ / ⌘↓
        .onKeyPress(.upArrow, phases: .down) { event in
            guard event.modifiers.contains(.command) else { return .ignored }
            navigateHistory(direction: -1)
            return .handled
        }
        .onKeyPress(.downArrow, phases: .down) { event in
            guard event.modifiers.contains(.command) else { return .ignored }
            navigateHistory(direction: 1)
            return .handled
        }
        .onAppear {
            isFocused = true
            appState.dictation.promptIsEmpty = text.isEmpty
            appState.dictation.onDictationComplete = { result in
                text += result
            }
            appState.dictation.onInsertSpace = {
                text += " "
            }
            appState.dictation.installEventMonitor()
        }
        .onDisappear {
            appState.dictation.removeEventMonitor()
        }
        .onChange(of: text) { _, newValue in
            appState.dictation.promptIsEmpty = newValue.isEmpty
            if newValue.isEmpty {
                historyIndex = -1
                routedSessionIDs = []
                routeTask?.cancel()
            } else {
                triggerRoutePrompt(newValue)
            }
        }
        .onChange(of: appState.chat.activeSessionID) { _, _ in
            isFocused = true
            suggestions = []
            historyIndex = -1
        }
        .onChange(of: appState.chat.isRunning) { _, running in
            // Fetch suggestions after a run completes
            if !running, let id = appState.chat.activeSessionID {
                Task {
                    let chips = try? await appState.api.sessionSuggestions(id: id)
                    await MainActor.run { suggestions = chips ?? [] }
                }
            }
        }
    }

    // MARK: - Prompt history navigation

    private func navigateHistory(direction: Int) {
        let history = appState.chat.promptHistory
        guard !history.isEmpty else { return }
        let newIndex = historyIndex + direction
        if direction == -1 {
            // Go back in history
            let clamped = min(max(newIndex, 0), history.count - 1)
            historyIndex = clamped
            text = history[history.count - 1 - clamped]
        } else {
            // Go forward
            if newIndex < 0 {
                historyIndex = -1
                text = ""
            } else {
                historyIndex = -1
                text = ""
            }
        }
    }

    // MARK: - Prompt enhancer

    private func enhancePrompt() {
        let original = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !original.isEmpty else { return }
        isEnhancing = true
        Task {
            let enhanced = try? await appState.api.enhancePrompt(original)
            await MainActor.run {
                if let e = enhanced, !e.isEmpty { text = e }
                isEnhancing = false
            }
        }
    }

    // MARK: - Prompt routing

    private func triggerRoutePrompt(_ value: String) {
        routeTask?.cancel()
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.count >= 10, appState.sessionList.sessions.count >= 2 else {
            routedSessionIDs = []
            return
        }
        routeTask = Task {
            // Debounce 500ms
            try? await Task.sleep(for: .milliseconds(500))
            guard !Task.isCancelled else { return }
            let ids = try? await appState.api.routePrompt(trimmed)
            guard !Task.isCancelled else { return }
            await MainActor.run { routedSessionIDs = ids ?? [] }
        }
    }

    // MARK: - Submit helpers

    private func doSubmit() {
        let full = buildFullPrompt()
        guard !full.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else { return }
        text = ""
        attachments = []
        suggestions = []
        historyIndex = -1
        onSubmit(full)
    }

    private func doForkSubmit() {
        let full = buildFullPrompt()
        guard !full.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else { return }
        text = ""
        attachments = []
        suggestions = []
        historyIndex = -1
        onForkSubmit?(full)
    }

    /// Combines attachment content with the user's typed text into a single prompt string.
    private func buildFullPrompt() -> String {
        var parts: [String] = []
        for att in attachments {
            if att.isImage {
                parts.append("[Attached image: \(att.content)]")
            } else {
                let lang = att.name.components(separatedBy: ".").last ?? ""
                parts.append("[Attached file: \(att.name)]\n```\(lang)\n\(att.content)\n```")
            }
        }
        if !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            parts.append(text)
        }
        return parts.joined(separator: "\n\n")
    }

    // MARK: - File picker

    private func openFilePicker() {
        let panel = NSOpenPanel()
        panel.allowsMultipleSelection = true
        panel.canChooseDirectories = false
        panel.canChooseFiles = true
        panel.begin { response in
            guard response == .OK else { return }
            for url in panel.urls {
                handleFileURL(url)
            }
        }
    }

    // MARK: - File handling

    func handleFileURL(_ url: URL) {
        let ext = url.pathExtension.lowercased()
        let imageExts: Set<String> = ["png", "jpg", "jpeg", "gif", "webp", "heic", "bmp", "tiff", "tif"]

        if imageExts.contains(ext) {
            let tempURL = URL(fileURLWithPath: NSTemporaryDirectory())
                .appendingPathComponent("orbitor_\(UUID().uuidString).\(ext)")
            do {
                try FileManager.default.copyItem(at: url, to: tempURL)
                let thumbnail = NSImage(contentsOf: url)
                DispatchQueue.main.async {
                    attachments.append(PromptAttachment(
                        name: url.lastPathComponent,
                        content: tempURL.path,
                        isImage: true,
                        thumbnail: thumbnail
                    ))
                }
            } catch {
                print("[Attachment] Failed to copy image: \(error)")
            }
        } else {
            // Read as text (skip if too large: >500 KB)
            guard let attrs = try? FileManager.default.attributesOfItem(atPath: url.path),
                  let size = attrs[.size] as? Int, size < 500_000 else {
                print("[Attachment] File too large or unreadable: \(url.lastPathComponent)")
                return
            }
            guard let content = try? String(contentsOf: url, encoding: .utf8) else { return }
            DispatchQueue.main.async {
                attachments.append(PromptAttachment(
                    name: url.lastPathComponent,
                    content: content,
                    isImage: false,
                    thumbnail: nil
                ))
            }
        }
    }

    // MARK: - Image paste

    private func handleImagePaste(_ providers: [NSItemProvider]) {
        for provider in providers {
            let pngType = UTType.png.identifier
            let jpegType = UTType.jpeg.identifier
            let tiffType = UTType.tiff.identifier
            let preferred = [pngType, jpegType, tiffType].first { provider.hasItemConformingToTypeIdentifier($0) }
            guard let typeID = preferred else { continue }
            let ext = typeID == jpegType ? "jpg" : (typeID == tiffType ? "tiff" : "png")

            provider.loadDataRepresentation(forTypeIdentifier: typeID) { data, error in
                guard let data, let image = NSImage(data: data) else { return }
                let tempURL = URL(fileURLWithPath: NSTemporaryDirectory())
                    .appendingPathComponent("orbitor_paste_\(UUID().uuidString).\(ext)")
                do {
                    try data.write(to: tempURL)
                    DispatchQueue.main.async {
                        attachments.append(PromptAttachment(
                            name: "pasted_image.\(ext)",
                            content: tempURL.path,
                            isImage: true,
                            thumbnail: image
                        ))
                    }
                } catch {
                    print("[Attachment] Failed to write pasted image: \(error)")
                }
            }
        }
    }
}

// MARK: - Suggestion chips

private struct SuggestionChipsView: View {
    let suggestions: [String]
    let theme: OrbitorTheme
    let onSelect: (String) -> Void

    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 6) {
                ForEach(suggestions, id: \.self) { chip in
                    Button {
                        onSelect(chip)
                    } label: {
                        HStack(spacing: 4) {
                            Image(systemName: "sparkle")
                                .font(.system(size: 9))
                                .foregroundStyle(theme.violet)
                            Text(chip)
                                .font(.caption)
                                .foregroundStyle(theme.text)
                                .lineLimit(1)
                        }
                        .padding(.horizontal, 8)
                        .padding(.vertical, 4)
                        .background(theme.violet.opacity(0.12))
                        .clipShape(RoundedRectangle(cornerRadius: 6))
                        .overlay(
                            RoundedRectangle(cornerRadius: 6)
                                .strokeBorder(theme.violet.opacity(0.3), lineWidth: 1)
                        )
                    }
                    .buttonStyle(.plain)
                }
            }
        }
    }
}

// MARK: - Attachment chips

private struct AttachmentChipsView: View {
    @Binding var attachments: [PromptAttachment]
    let theme: OrbitorTheme

    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 6) {
                ForEach(attachments) { att in
                    AttachmentChip(attachment: att, theme: theme) {
                        attachments.removeAll { $0.id == att.id }
                    }
                }
            }
        }
    }
}

private struct AttachmentChip: View {
    let attachment: PromptAttachment
    let theme: OrbitorTheme
    let onRemove: () -> Void
    @State private var isHovered = false

    var body: some View {
        HStack(spacing: 5) {
            if attachment.isImage, let thumb = attachment.thumbnail {
                Image(nsImage: thumb)
                    .resizable()
                    .scaledToFill()
                    .frame(width: 18, height: 18)
                    .clipShape(RoundedRectangle(cornerRadius: 3))
            } else {
                Image(systemName: fileIcon)
                    .font(.caption2)
                    .foregroundStyle(theme.accent)
            }

            Text(attachment.name)
                .font(.caption)
                .foregroundStyle(theme.text)
                .lineLimit(1)
                .truncationMode(.middle)
                .frame(maxWidth: 120)

            Button {
                onRemove()
            } label: {
                Image(systemName: "xmark")
                    .font(.system(size: 8, weight: .semibold))
                    .foregroundStyle(theme.muted)
            }
            .buttonStyle(.plain)
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 4)
        .background(theme.selBg)
        .clipShape(RoundedRectangle(cornerRadius: 6))
        .overlay(
            RoundedRectangle(cornerRadius: 6)
                .strokeBorder(theme.border, lineWidth: 1)
        )
        .onHover { isHovered = $0 }
        .scaleEffect(isHovered ? 1.02 : 1.0)
        .animation(.easeOut(duration: 0.1), value: isHovered)
    }

    private var fileIcon: String {
        let ext = attachment.name.components(separatedBy: ".").last?.lowercased() ?? ""
        switch ext {
        case "swift", "py", "go", "ts", "js", "rs": return "doc.text"
        case "json", "yaml", "yml", "toml": return "doc.badge.gearshape"
        case "md", "txt": return "doc.plaintext"
        default: return "doc"
        }
    }
}
