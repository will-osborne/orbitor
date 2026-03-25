import SwiftUI

struct PromptInputView: View {
    @Binding var text: String
    var onSubmit: () -> Void
    @Environment(AppState.self) private var appState
    @Environment(\.theme) private var theme
    @FocusState private var isFocused: Bool

    var body: some View {
        HStack(alignment: .bottom, spacing: 8) {
            ZStack(alignment: .topLeading) {
                if text.isEmpty {
                    Text("Type a prompt and press ⌘Enter...")
                        .foregroundStyle(theme.muted)
                        .padding(.horizontal, 8)
                        .padding(.vertical, 10)
                }
                TextEditor(text: $text)
                    .font(.system(.body, design: .monospaced))
                    .foregroundStyle(theme.text)
                    .scrollContentBackground(.hidden)
                    .focused($isFocused)
                    .frame(minHeight: 36, maxHeight: 120)
                    .fixedSize(horizontal: false, vertical: true)
            }
            .padding(4)
            .background(theme.selBg)
            .clipShape(RoundedRectangle(cornerRadius: 8))
            .overlay(
                RoundedRectangle(cornerRadius: 8)
                    .strokeBorder(isFocused ? theme.accent : theme.border, lineWidth: 1)
            )

            VStack(spacing: 4) {
                Button {
                    onSubmit()
                } label: {
                    Image(systemName: "arrow.up.circle.fill")
                        .font(.title2)
                        .foregroundStyle(text.isEmpty ? theme.muted : theme.accent)
                }
                .buttonStyle(.plain)
                .disabled(text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                .keyboardShortcut(.return, modifiers: .command)
                .help("Send (⌘Enter)")

                if appState.chat.isRunning {
                    Button {
                        Task { await appState.chat.interrupt() }
                    } label: {
                        Image(systemName: "stop.circle.fill")
                            .font(.title2)
                            .foregroundStyle(theme.orange)
                    }
                    .buttonStyle(.plain)
                    .help("Interrupt (⌘.)")
                }
            }
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 8)
        .background(theme.panel)
        .onAppear { isFocused = true }
    }
}
