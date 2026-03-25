import SwiftUI

struct CodeBlockView: View {
    let code: String
    var language: String = ""
    @Environment(\.theme) private var theme
    @State private var isCopied = false

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            // Language label + copy button
            if !language.isEmpty || true {
                HStack {
                    if !language.isEmpty {
                        Text(language)
                            .font(.caption2)
                            .foregroundStyle(theme.muted)
                    }
                    Spacer()
                    Button {
                        NSPasteboard.general.clearContents()
                        NSPasteboard.general.setString(code, forType: .string)
                        isCopied = true
                        Task {
                            try? await Task.sleep(for: .seconds(2))
                            isCopied = false
                        }
                    } label: {
                        Image(systemName: isCopied ? "checkmark" : "doc.on.doc")
                            .font(.caption2)
                            .foregroundStyle(isCopied ? theme.green : theme.muted)
                    }
                    .buttonStyle(.plain)
                }
                .padding(.horizontal, 10)
                .padding(.top, 6)
                .padding(.bottom, 2)
            }

            // Code content
            ScrollView(.horizontal, showsIndicators: false) {
                Text(code)
                    .font(.system(.caption, design: .monospaced))
                    .foregroundStyle(theme.text)
                    .textSelection(.enabled)
                    .padding(.horizontal, 10)
                    .padding(.bottom, 8)
            }
        }
        .background(Color(hex: "1E1E1E").opacity(0.5))
        .clipShape(RoundedRectangle(cornerRadius: 6))
        .overlay(
            RoundedRectangle(cornerRadius: 6)
                .strokeBorder(theme.border, lineWidth: 1)
        )
    }
}

struct MarkdownTextView: View {
    let text: String
    @Environment(\.theme) private var theme

    var body: some View {
        let segments = parseMarkdown(text)
        VStack(alignment: .leading, spacing: 4) {
            ForEach(Array(segments.enumerated()), id: \.offset) { _, segment in
                switch segment {
                case .text(let content):
                    if let attributed = try? AttributedString(markdown: content) {
                        Text(attributed)
                            .font(.body)
                            .foregroundStyle(theme.text)
                            .textSelection(.enabled)
                    } else {
                        Text(content)
                            .font(.body)
                            .foregroundStyle(theme.text)
                            .textSelection(.enabled)
                    }
                case .code(let content, let lang):
                    CodeBlockView(code: content, language: lang)
                }
            }
        }
    }

    // Split text at ``` fences into text and code segments
    private func parseMarkdown(_ input: String) -> [MarkdownSegment] {
        var segments: [MarkdownSegment] = []
        let lines = input.split(separator: "\n", omittingEmptySubsequences: false).map(String.init)
        var currentText = ""
        var inCode = false
        var codeLang = ""
        var codeContent = ""

        for line in lines {
            let trimmed = line.trimmingCharacters(in: .whitespaces)
            if trimmed.hasPrefix("```") {
                if inCode {
                    // End code block
                    segments.append(.code(codeContent, codeLang))
                    codeContent = ""
                    codeLang = ""
                    inCode = false
                } else {
                    // Start code block
                    if !currentText.isEmpty {
                        segments.append(.text(currentText))
                        currentText = ""
                    }
                    codeLang = String(trimmed.dropFirst(3)).trimmingCharacters(in: .whitespaces)
                    inCode = true
                }
            } else if inCode {
                if !codeContent.isEmpty { codeContent += "\n" }
                codeContent += line
            } else {
                if !currentText.isEmpty { currentText += "\n" }
                currentText += line
            }
        }
        if !currentText.isEmpty { segments.append(.text(currentText)) }
        if !codeContent.isEmpty { segments.append(.code(codeContent, codeLang)) }
        return segments
    }
}

private enum MarkdownSegment {
    case text(String)
    case code(String, String)
}
