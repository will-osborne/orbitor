import SwiftUI

struct ChatView: View {
    @Environment(AppState.self) private var appState
    @Environment(\.theme) private var theme
    @State private var promptText = ""
    @State private var scrollToBottom = true

    var body: some View {
        VStack(spacing: 0) {
            // Permission banner
            if let perm = appState.chat.pendingPermission {
                PermissionBannerView(permission: perm)
            }

            // Message list
            ScrollViewReader { proxy in
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 8) {
                        ForEach(appState.chat.messages) { message in
                            MessageView(message: message)
                                .id(message.id)
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
                    if scrollToBottom {
                        withAnimation(.easeOut(duration: 0.2)) {
                            proxy.scrollTo("bottom", anchor: .bottom)
                        }
                    }
                }
            }

            Divider().background(theme.sep)

            // Input area
            PromptInputView(text: $promptText) {
                Task {
                    let text = promptText
                    promptText = ""
                    await appState.chat.sendPrompt(text)
                }
            }
        }
        .background(theme.panel)
    }
}

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
