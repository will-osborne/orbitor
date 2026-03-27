import SwiftUI

struct CommandPaletteView: View {
    @Binding var isPresented: Bool
    @Environment(AppState.self) private var appState
    @Environment(\.theme) private var theme
    @State private var query = ""
    @State private var selectedIndex = 0
    @FocusState private var searchFocused: Bool

    struct PaletteItem: Identifiable {
        let id = UUID()
        let icon: String
        let title: String
        let subtitle: String?
        let shortcut: String?
        let action: () -> Void
    }

    private func makeItems() -> [PaletteItem] {
        var items: [PaletteItem] = []

        // --- Actions ---
        items.append(PaletteItem(icon: "plus.circle", title: "New Session", subtitle: nil, shortcut: "⌘N") {
            appState.showNewSession = true
            isPresented = false
        })

        if appState.sessionList.selectedSessionID != nil {
            items.append(PaletteItem(icon: "arrow.triangle.branch", title: "Fork Session", subtitle: nil, shortcut: "⇧⌘N") {
                appState.showForkSheet = true
                isPresented = false
            })

            if appState.chat.isRunning {
                items.append(PaletteItem(icon: "stop.circle", title: "Interrupt Agent", subtitle: nil, shortcut: "⌘.") {
                    Task { await appState.chat.interrupt() }
                    isPresented = false
                })
            }

            if let id = appState.sessionList.selectedSessionID {
                items.append(PaletteItem(
                    icon: "trash",
                    title: "Delete Session",
                    subtitle: appState.sessionList.selectedSession?.displayTitle,
                    shortcut: "⌘⌫"
                ) {
                    Task { await appState.sessionList.deleteSession(id) }
                    isPresented = false
                })
            }
        }

        if appState.sessionList.sessions.count > 1 {
            items.append(PaletteItem(icon: "chevron.down", title: "Next Session", subtitle: nil, shortcut: "⌘]") {
                appState.sessionList.selectNext()
                isPresented = false
            })
            items.append(PaletteItem(icon: "chevron.up", title: "Previous Session", subtitle: nil, shortcut: "⌘[") {
                appState.sessionList.selectPrevious()
                isPresented = false
            })
        }

        // --- Sessions ---
        for session in appState.sessionList.sessions {
            let isCurrent = session.id == appState.sessionList.selectedSessionID
            items.append(PaletteItem(
                icon: isCurrent ? "terminal.fill" : "terminal",
                title: session.displayTitle,
                subtitle: "\(session.stateLabel)  \(session.shortDir)",
                shortcut: nil
            ) {
                appState.sessionList.selectedSessionID = session.id
                isPresented = false
            })
        }

        // --- Themes ---
        for t in OrbitorTheme.all {
            let isCurrent = t.id == appState.selectedThemeID
            items.append(PaletteItem(
                icon: isCurrent ? "checkmark.circle.fill" : "paintbrush",
                title: "Theme: \(t.name)",
                subtitle: nil,
                shortcut: nil
            ) {
                appState.selectedThemeID = t.id
                isPresented = false
            })
        }

        return items
    }

    private var filtered: [PaletteItem] {
        let all = makeItems()
        guard !query.trimmingCharacters(in: .whitespaces).isEmpty else { return all }
        let q = query.lowercased()
        return all.filter {
            $0.title.lowercased().contains(q) ||
            ($0.subtitle?.lowercased().contains(q) ?? false)
        }
    }

    var body: some View {
        ZStack {
            // Backdrop — tap to dismiss
            Color.black.opacity(0.45)
                .ignoresSafeArea()
                .onTapGesture { isPresented = false }

            // Floating panel
            VStack(spacing: 0) {
                // Search bar
                HStack(spacing: 10) {
                    Image(systemName: "magnifyingglass")
                        .foregroundStyle(theme.muted)
                    TextField("Search commands, sessions…", text: $query)
                        .textFieldStyle(.plain)
                        .font(.system(size: 15))
                        .foregroundStyle(theme.text)
                        .focused($searchFocused)
                        .onSubmit { executeSelected() }
                        .onChange(of: query) { _, _ in selectedIndex = 0 }
                }
                .padding(.horizontal, 16)
                .padding(.vertical, 13)

                Divider().background(theme.border)

                // Results list
                if filtered.isEmpty {
                    Text("No results")
                        .font(.caption)
                        .foregroundStyle(theme.muted)
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 24)
                } else {
                    ScrollViewReader { proxy in
                        ScrollView {
                            LazyVStack(spacing: 0) {
                                ForEach(Array(filtered.enumerated()), id: \.element.id) { idx, item in
                                    PaletteRow(item: item, isSelected: idx == selectedIndex)
                                        .id(idx)
                                        .contentShape(Rectangle())
                                        .onTapGesture { item.action() }
                                        .onHover { hovered in if hovered { selectedIndex = idx } }
                                }
                            }
                        }
                        .frame(maxHeight: 380)
                        .onChange(of: selectedIndex) { _, new in
                            withAnimation(.easeOut(duration: 0.1)) {
                                proxy.scrollTo(new, anchor: .center)
                            }
                        }
                    }
                }
            }
            .background(theme.panel)
            .clipShape(RoundedRectangle(cornerRadius: 12))
            .overlay(
                RoundedRectangle(cornerRadius: 12)
                    .strokeBorder(theme.border, lineWidth: 1)
            )
            .shadow(color: .black.opacity(0.5), radius: 30, x: 0, y: 10)
            .frame(width: 540)
            .padding(.horizontal, 40)
        }
        .onKeyPress(.escape) { isPresented = false; return .handled }
        .onKeyPress(.upArrow) { selectedIndex = max(0, selectedIndex - 1); return .handled }
        .onKeyPress(.downArrow) { selectedIndex = min(filtered.count - 1, selectedIndex + 1); return .handled }
        .onKeyPress(.return) { executeSelected(); return .handled }
        .onAppear {
            searchFocused = true
            selectedIndex = 0
        }
    }

    private func executeSelected() {
        guard selectedIndex < filtered.count else { return }
        filtered[selectedIndex].action()
    }
}

// MARK: - Palette row

private struct PaletteRow: View {
    let item: CommandPaletteView.PaletteItem
    let isSelected: Bool
    @Environment(\.theme) private var theme

    var body: some View {
        HStack(spacing: 10) {
            Image(systemName: item.icon)
                .font(.system(size: 14))
                .foregroundStyle(isSelected ? theme.accent : theme.muted)
                .frame(width: 22, alignment: .center)

            VStack(alignment: .leading, spacing: 1) {
                Text(item.title)
                    .font(.system(size: 13))
                    .foregroundStyle(theme.text)
                if let sub = item.subtitle {
                    Text(sub)
                        .font(.caption)
                        .foregroundStyle(theme.muted)
                        .lineLimit(1)
                }
            }

            Spacer()

            if let shortcut = item.shortcut {
                Text(shortcut)
                    .font(.system(size: 11, design: .monospaced))
                    .foregroundStyle(theme.muted)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 2)
                    .background(theme.selBg)
                    .clipShape(RoundedRectangle(cornerRadius: 4))
            }
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 8)
        .background(isSelected ? theme.accent.opacity(0.13) : Color.clear)
        .overlay(alignment: .leading) {
            if isSelected {
                RoundedRectangle(cornerRadius: 2)
                    .fill(theme.accent)
                    .frame(width: 3)
            }
        }
    }
}
