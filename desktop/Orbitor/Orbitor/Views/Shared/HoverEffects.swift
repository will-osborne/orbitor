import SwiftUI

// MARK: - Hover scale modifier

struct HoverScaleModifier: ViewModifier {
    var scale: CGFloat = 1.12
    @State private var isHovered = false

    func body(content: Content) -> some View {
        content
            .scaleEffect(isHovered ? scale : 1.0)
            .animation(.spring(response: 0.25, dampingFraction: 0.6), value: isHovered)
            .onHover { isHovered = $0 }
    }
}

extension View {
    func hoverScale(_ scale: CGFloat = 1.12) -> some View {
        modifier(HoverScaleModifier(scale: scale))
    }
}

// MARK: - Hover glow modifier (brightens + lifts)

struct HoverGlowModifier: ViewModifier {
    @State private var isHovered = false

    func body(content: Content) -> some View {
        content
            .brightness(isHovered ? 0.08 : 0)
            .shadow(color: .accentColor.opacity(isHovered ? 0.3 : 0), radius: isHovered ? 6 : 0, y: 0)
            .scaleEffect(isHovered ? 1.02 : 1.0)
            .animation(.easeOut(duration: 0.18), value: isHovered)
            .onHover { isHovered = $0 }
    }
}

extension View {
    func hoverGlow() -> some View {
        modifier(HoverGlowModifier())
    }
}

// MARK: - Hover highlight for cards (border brightens)

struct HoverHighlightModifier: ViewModifier {
    @Environment(\.theme) private var theme
    @State private var isHovered = false

    func body(content: Content) -> some View {
        content
            .brightness(isHovered ? 0.04 : 0)
            .overlay(
                RoundedRectangle(cornerRadius: 6)
                    .strokeBorder(theme.accent.opacity(isHovered ? 0.25 : 0), lineWidth: 1)
            )
            .animation(.easeOut(duration: 0.15), value: isHovered)
            .onHover { isHovered = $0 }
    }
}

extension View {
    func hoverHighlight() -> some View {
        modifier(HoverHighlightModifier())
    }
}
