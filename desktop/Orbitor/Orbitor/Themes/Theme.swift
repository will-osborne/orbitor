import SwiftUI

struct OrbitorTheme: Identifiable, Hashable {
    let id: String
    let name: String
    let green: Color
    let orange: Color
    let yellow: Color
    let red: Color
    let cyan: Color
    let violet: Color
    let gray: Color
    let muted: Color
    let text: Color
    let sep: Color
    let border: Color
    let accent: Color
    let selBg: Color
    let panel: Color

    func hash(into hasher: inout Hasher) { hasher.combine(id) }
    static func == (lhs: Self, rhs: Self) -> Bool { lhs.id == rhs.id }

    func stateColor(_ state: String) -> Color {
        switch state {
        case "idle": return green
        case "working", "starting": return orange
        case "waiting-input": return yellow
        case "error": return red
        case "offline", "closed", "suspended": return gray
        case "killed": return red
        default: return muted
        }
    }
}

extension OrbitorTheme {
    static let dracula = OrbitorTheme(
        id: "dracula", name: "Dracula",
        green: Color(hex: "50FA7B"), orange: Color(hex: "FFB86C"),
        yellow: Color(hex: "F1FA8C"), red: Color(hex: "FF5555"),
        cyan: Color(hex: "8BE9FD"), violet: Color(hex: "BD93F9"),
        gray: Color(hex: "6272A4"), muted: Color(hex: "94A3C5"),
        text: Color(hex: "F8F8F2"), sep: Color(hex: "44475A"),
        border: Color(hex: "3B3E52"), accent: Color(hex: "BD93F9"),
        selBg: Color(hex: "343746"), panel: Color(hex: "282A36")
    )

    static let opencode = OrbitorTheme(
        id: "opencode", name: "OpenCode",
        green: Color(hex: "5FAF5F"), orange: Color(hex: "D7875F"),
        yellow: Color(hex: "D7D75F"), red: Color(hex: "D75F5F"),
        cyan: Color(hex: "5FAFAF"), violet: Color(hex: "AF87D7"),
        gray: Color(hex: "767676"), muted: Color(hex: "949494"),
        text: Color(hex: "C6C6C6"), sep: Color(hex: "3A3A3A"),
        border: Color(hex: "444444"), accent: Color(hex: "AF87D7"),
        selBg: Color(hex: "303030"), panel: Color(hex: "1C1C1C")
    )

    static let catppuccin = OrbitorTheme(
        id: "catppuccin", name: "Catppuccin",
        green: Color(hex: "A6E3A1"), orange: Color(hex: "FAB387"),
        yellow: Color(hex: "F9E2AF"), red: Color(hex: "F38BA8"),
        cyan: Color(hex: "94E2D5"), violet: Color(hex: "CBA6F7"),
        gray: Color(hex: "6C7086"), muted: Color(hex: "9399B2"),
        text: Color(hex: "CDD6F4"), sep: Color(hex: "45475A"),
        border: Color(hex: "585B70"), accent: Color(hex: "CBA6F7"),
        selBg: Color(hex: "313244"), panel: Color(hex: "1E1E2E")
    )

    static let tokyoNight = OrbitorTheme(
        id: "tokyonight", name: "Tokyo Night",
        green: Color(hex: "9ECE6A"), orange: Color(hex: "FF9E64"),
        yellow: Color(hex: "E0AF68"), red: Color(hex: "F7768E"),
        cyan: Color(hex: "7DCFFF"), violet: Color(hex: "BB9AF7"),
        gray: Color(hex: "565F89"), muted: Color(hex: "737AA2"),
        text: Color(hex: "C0CAF5"), sep: Color(hex: "3B4261"),
        border: Color(hex: "2F3549"), accent: Color(hex: "7AA2F7"),
        selBg: Color(hex: "283457"), panel: Color(hex: "1A1B26")
    )

    static let all: [OrbitorTheme] = [.dracula, .opencode, .catppuccin, .tokyoNight]
}

// MARK: - Environment key

private struct ThemeKey: EnvironmentKey {
    static let defaultValue: OrbitorTheme = .dracula
}

extension EnvironmentValues {
    var theme: OrbitorTheme {
        get { self[ThemeKey.self] }
        set { self[ThemeKey.self] = newValue }
    }
}

// MARK: - Color hex init

extension Color {
    init(hex: String) {
        let hex = hex.trimmingCharacters(in: CharacterSet(charactersIn: "#"))
        var int: UInt64 = 0
        Scanner(string: hex).scanHexInt64(&int)
        let r = Double((int >> 16) & 0xFF) / 255
        let g = Double((int >> 8) & 0xFF) / 255
        let b = Double(int & 0xFF) / 255
        self.init(red: r, green: g, blue: b)
    }
}
