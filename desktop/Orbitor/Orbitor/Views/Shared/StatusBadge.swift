import SwiftUI

struct StatusBadge: View {
    let state: String
    @Environment(\.theme) private var theme
    @State private var pulse = false

    private var isAnimated: Bool {
        state == "working" || state == "waiting-input" || state == "starting"
    }

    var body: some View {
        HStack(spacing: 4) {
            Circle()
                .fill(theme.stateColor(state))
                .frame(width: 6, height: 6)
                .opacity(isAnimated && pulse ? 0.3 : 1.0)
            Text(state)
                .font(.system(.caption2, design: .monospaced))
                .foregroundStyle(theme.stateColor(state))
        }
        .padding(.horizontal, 6)
        .padding(.vertical, 2)
        .background(theme.stateColor(state).opacity(0.1))
        .clipShape(Capsule())
        .hoverScale(1.08)
        .onAppear {
            if isAnimated {
                withAnimation(.easeInOut(duration: 0.8).repeatForever(autoreverses: true)) {
                    pulse = true
                }
            }
        }
        .onChange(of: state) { _, newState in
            let animated = newState == "working" || newState == "waiting-input" || newState == "starting"
            if animated {
                pulse = false
                withAnimation(.easeInOut(duration: 0.8).repeatForever(autoreverses: true)) {
                    pulse = true
                }
            } else {
                withAnimation(.default) {
                    pulse = false
                }
            }
        }
    }
}
