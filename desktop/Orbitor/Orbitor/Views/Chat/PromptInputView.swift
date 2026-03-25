import SwiftUI

struct PromptInputView: View {
    @Binding var text: String
    var onSubmit: () -> Void
    var onForkSubmit: (() -> Void)?
    @Environment(AppState.self) private var appState
    @Environment(\.theme) private var theme
    @FocusState private var isFocused: Bool

    var body: some View {
        HStack(alignment: .bottom, spacing: 8) {
            ZStack(alignment: .topLeading) {
                if text.isEmpty && !appState.dictation.isRecording {
                    Text("Type a prompt and press ⌘Enter...")
                        .foregroundStyle(theme.muted)
                        .padding(.horizontal, 8)
                        .padding(.top, 8)
                }
                if appState.dictation.isRecording {
                    HStack(spacing: 8) {
                        Image(systemName: "mic.fill")
                            .foregroundStyle(theme.red)
                            .symbolEffect(.pulse)
                        Text(appState.dictation.transcribedText.isEmpty ? "Listening... (release Space to stop)" : appState.dictation.transcribedText)
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

                // Fork button (Option+Enter)
                if appState.sessionList.selectedSessionID != nil {
                    Button {
                        onForkSubmit?()
                    } label: {
                        Image(systemName: "arrow.triangle.branch")
                            .font(.body)
                            .foregroundStyle(text.isEmpty ? theme.muted : theme.cyan)
                    }
                    .buttonStyle(.plain)
                    .hoverScale(1.15)
                    .disabled(text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
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
                    onSubmit()
                } label: {
                    Image(systemName: "arrow.up.circle.fill")
                        .font(.title3)
                        .foregroundStyle(text.isEmpty ? theme.muted : theme.accent)
                }
                .buttonStyle(.plain)
                .hoverScale()
                .disabled(text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                .keyboardShortcut(.return, modifiers: .command)
                .help("Send (⌘Enter)")
            }
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 8)
        .background(theme.panel)
        .onAppear {
            isFocused = true
            appState.dictation.promptIsEmpty = text.isEmpty
            appState.dictation.onDictationComplete = { [weak appState] result in
                // Remove any spaces that leaked into the text field during hold
                if let appState {
                    // Trim leading spaces that accumulated while holding
                    let cleaned = text.replacingOccurrences(of: #"^\s+"#, with: "", options: .regularExpression)
                    text = cleaned + result
                    _ = appState // keep reference alive
                }
            }
            appState.dictation.installEventMonitor()
        }
        .onDisappear {
            appState.dictation.removeEventMonitor()
        }
        .onChange(of: text) { _, newValue in
            appState.dictation.promptIsEmpty = newValue.isEmpty
        }
    }
}
