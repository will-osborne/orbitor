import Foundation
import SwiftUI

@Observable
final class AppState {
    var serverURL: String {
        didSet { UserDefaults.standard.set(serverURL, forKey: "serverURL") }
    }
    var connectionStatus: ConnectionStatus = .disconnected
    var selectedThemeID: String {
        didSet { UserDefaults.standard.set(selectedThemeID, forKey: "themeID") }
    }

    var showNewSession = false
    var showForkSheet = false
    var showCommandPalette = false
    var fontSize: CGFloat {
        didSet { UserDefaults.standard.set(fontSize, forKey: "fontSize") }
    }

    private(set) var api: APIClient
    let sessionList: SessionListState
    let chat: ChatState
    let dictation = DictationState()

    var currentTheme: OrbitorTheme {
        OrbitorTheme.all.first { $0.id == selectedThemeID } ?? .dracula
    }

    init() {
        let config = Self.loadConfig()
        let url = UserDefaults.standard.string(forKey: "serverURL") ?? config.serverURL
        let themeID = UserDefaults.standard.string(forKey: "themeID") ?? "dracula"

        self.serverURL = url
        self.selectedThemeID = themeID
        let savedFontSize = UserDefaults.standard.double(forKey: "fontSize")
        self.fontSize = savedFontSize > 0 ? savedFontSize : 13

        let baseURL = URL(string: url) ?? URL(string: "http://127.0.0.1:8080")!
        let client = APIClient(baseURL: baseURL)
        self.api = client
        self.sessionList = SessionListState(api: client)
        self.chat = ChatState(baseURL: baseURL)
    }

    func reconnect() {
        let baseURL = URL(string: serverURL) ?? URL(string: "http://127.0.0.1:8080")!
        let client = APIClient(baseURL: baseURL)
        self.api = client
        sessionList.updateAPI(client)
        chat.updateBaseURL(baseURL)
        connectionStatus = .connecting
        Task {
            await sessionList.refresh()
            connectionStatus = .connected
        }
    }

    // MARK: - Config loading

    private static func loadConfig() -> ClientConfig {
        let home = FileManager.default.homeDirectoryForCurrentUser
        let configPath = home.appendingPathComponent(".orbitor/config.json")
        guard let data = try? Data(contentsOf: configPath),
              let config = try? JSONDecoder().decode(ClientConfig.self, from: data) else {
            return ClientConfig.default
        }
        return config
    }
}

struct ClientConfig: Codable {
    let serverURL: String
    let listenAddr: String?
    let defaultBackend: String?
    let defaultModel: String?
    let skipPermissions: Bool?
    let planMode: Bool?

    static let `default` = ClientConfig(
        serverURL: "http://127.0.0.1:8080",
        listenAddr: nil, defaultBackend: "claude",
        defaultModel: nil, skipPermissions: false, planMode: false
    )
}

enum ConnectionStatus {
    case connected, connecting, disconnected, error(String)
}
