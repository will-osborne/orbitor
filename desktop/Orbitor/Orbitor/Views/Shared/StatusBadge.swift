import SwiftUI

struct StatusBadge: View {
    let state: String
    @Environment(\.theme) private var theme

    var body: some View {
        HStack(spacing: 4) {
            Circle()
                .fill(theme.stateColor(state))
                .frame(width: 6, height: 6)
            Text(state)
                .font(.system(.caption2, design: .monospaced))
                .foregroundStyle(theme.stateColor(state))
        }
        .padding(.horizontal, 6)
        .padding(.vertical, 2)
        .background(theme.stateColor(state).opacity(0.1))
        .clipShape(Capsule())
    }
}
