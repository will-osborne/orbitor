import 'dart:async';
import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;
import 'package:shared_preferences/shared_preferences.dart';
import 'package:web_socket_channel/web_socket_channel.dart';
import '../models/session.dart';
import '../models/message.dart';

class ApiService extends ChangeNotifier {
  static const String defaultBaseUrl = 'http://ff00030.tail8466fb.ts.net:8080';
  String _baseUrl;
  String get baseUrl => _baseUrl;
  WebSocketChannel? _eventsChannel;
  StreamSubscription? _eventsSub;
  Timer? _eventsReconnect;

  ApiService({String baseUrl = defaultBaseUrl}) : _baseUrl = baseUrl;

  void updateBaseUrl(String url) {
    _baseUrl = url.replaceAll(RegExp(r'/+$'), '');
    SharedPreferences.getInstance().then(
      (prefs) => prefs.setString('api_base_url', _baseUrl),
    );
    _connectEvents(); // reconnect to new host
    notifyListeners();
  }

  /// Global event callback for notification-worthy events (permission requests,
  /// run completion, etc.) across all sessions.
  void Function(GlobalNotificationEvent event)? onGlobalNotification;

  void _connectEvents() {
    _eventsSub?.cancel();
    _eventsReconnect?.cancel();
    try {
      _eventsChannel?.sink.close();
    } catch (_) {}

    final uri = Uri.parse('$_wsBase/ws/events');
    _eventsChannel = WebSocketChannel.connect(uri);
    _eventsSub = _eventsChannel!.stream.listen(
      (data) {
        try {
          final json = jsonDecode(data as String) as Map<String, dynamic>;
          final type = json['type'] as String? ?? '';
          if (type == 'permission_request' || type == 'run_complete') {
            final d = json['data'] as Map<String, dynamic>? ?? {};
            final event = GlobalNotificationEvent.fromWS(type, d);
            onGlobalNotification?.call(event);
          }
        } catch (_) {}
      },
      onError: (_) => _scheduleEventsReconnect(),
      onDone: () => _scheduleEventsReconnect(),
    );
  }

  bool _eventsBackgrounded = false;

  void _scheduleEventsReconnect() {
    if (_eventsBackgrounded) return;
    _eventsReconnect?.cancel();
    _eventsReconnect = Timer(const Duration(seconds: 3), _connectEvents);
  }

  /// Call once after construction to start the global event listener.
  void startEventListener() => _connectEvents();

  /// Call when the app moves to the background.
  void onBackground() {
    _eventsBackgrounded = true;
    _eventsReconnect?.cancel();
  }

  /// Call when the app returns to the foreground. Reconnects the global
  /// events WebSocket if it was dropped while backgrounded.
  void onResume() {
    _eventsBackgrounded = false;
    _connectEvents();
  }

  String get _wsBase {
    final uri = Uri.parse(_baseUrl);
    final scheme = uri.scheme == 'https' ? 'wss' : 'ws';
    return '$scheme://${uri.host}:${uri.port}';
  }

  Future<List<Session>> listSessions() async {
    final resp = await http.get(Uri.parse('$_baseUrl/api/sessions'));
    if (resp.statusCode != 200) {
      throw Exception('Failed to list sessions: ${resp.statusCode}');
    }
    final list = jsonDecode(resp.body) as List;
    return list.map((j) => Session.fromJson(j)).toList();
  }

  Future<Session> createSession(
    String workingDir, {
    String backend = 'copilot',
    String model = '',
    bool skipPermissions = false,
    bool planMode = false,
  }) async {
    final body = <String, dynamic>{
      'workingDir': workingDir,
      'backend': backend,
      'skipPermissions': skipPermissions,
      'planMode': planMode,
    };
    if (model.isNotEmpty) {
      body['model'] = model;
    }
    final resp = await http.post(
      Uri.parse('$_baseUrl/api/sessions'),
      headers: {'Content-Type': 'application/json'},
      body: jsonEncode(body),
    );
    if (resp.statusCode != 201) {
      throw Exception('Failed to create session: ${resp.body}');
    }
    return Session.fromJson(jsonDecode(resp.body));
  }

  Future<Map<String, dynamic>> selfUpdate({bool flutter = false}) async {
    final resp = await http.post(
      Uri.parse('$_baseUrl/api/self-update'),
      headers: {'Content-Type': 'application/json'},
      body: jsonEncode({'flutter': flutter}),
    );
    final body = jsonDecode(resp.body) as Map<String, dynamic>;
    if (resp.statusCode != 200) {
      throw Exception(
        body['error'] ?? 'Self-update failed (${resp.statusCode})',
      );
    }
    return body;
  }

  Future<void> deleteSession(String id) async {
    final resp = await http.delete(Uri.parse('$_baseUrl/api/sessions/$id'));
    if (resp.statusCode != 204) {
      throw Exception('Failed to delete session: ${resp.statusCode}');
    }
  }

  Future<Session> updateSession(
    String id, {
    required bool skipPermissions,
    required bool planMode,
  }) async {
    final resp = await http.patch(
      Uri.parse('$_baseUrl/api/sessions/$id'),
      headers: {'Content-Type': 'application/json'},
      body: jsonEncode({'skipPermissions': skipPermissions, 'planMode': planMode}),
    );
    if (resp.statusCode != 200) {
      throw Exception('Failed to update session: ${resp.body}');
    }
    return Session.fromJson(jsonDecode(resp.body));
  }

  Future<void> killSession(String id) async {
    final resp = await http.post(Uri.parse('$_baseUrl/api/sessions/$id/kill'));
    if (resp.statusCode != 204) {
      throw Exception('Failed to kill session: ${resp.statusCode}');
    }
  }

  Future<void> reviveSession(String id) async {
    final resp = await http.post(Uri.parse('$_baseUrl/api/sessions/$id/revive'));
    if (resp.statusCode != 204) {
      throw Exception('Failed to revive session: ${resp.body}');
    }
  }

  Future<void> releaseApk() async {
    final resp = await http.post(
      Uri.parse('$_baseUrl/api/release-apk'),
      headers: {'Content-Type': 'application/json'},
      body: jsonEncode({}),
    );
    // Server responds with 202 Accepted when the build is queued and run
    // asynchronously. Treat 200 and 202 as success here so the client
    // doesn't throw on an expected async response.
    if (resp.statusCode != 200 && resp.statusCode != 202) {
      final body = jsonDecode(resp.body) as Map<String, dynamic>;
      throw Exception(body['error'] ?? 'Release APK failed (${resp.statusCode})');
    }
  }

  Future<Map<String, String>> getMissionSummary() async {
    final resp = await http.get(Uri.parse('$_baseUrl/api/mission-summary'));
    if (resp.statusCode != 200) {
      throw Exception('Failed to fetch mission summary: ${resp.statusCode}');
    }
    final body = jsonDecode(resp.body) as Map<String, dynamic>;
    return {
      'title': body['title'] as String? ?? '',
      'summary': body['summary'] as String? ?? ''
    };
  }

  Future<BrowseResult> browseDir([String path = '']) async {
    final uri = Uri.parse(
      '$_baseUrl/api/browse${path.isNotEmpty ? '?path=$path' : ''}',
    );
    final resp = await http.get(uri);
    if (resp.statusCode != 200) {
      throw Exception('Browse failed: ${resp.statusCode}');
    }
    return BrowseResult.fromJson(jsonDecode(resp.body));
  }

  Future<void> registerDeviceToken(String token, {String platform = 'android'}) async {
    final resp = await http.post(
      Uri.parse('$_baseUrl/api/device-token'),
      headers: {'Content-Type': 'application/json'},
      body: jsonEncode({'token': token, 'platform': platform}),
    );
    if (resp.statusCode != 200) {
      throw Exception('Failed to register device token: ${resp.statusCode}');
    }
  }

  Future<List<GlobalNotificationEvent>> fetchNotifications({
    int after = 0,
    int limit = 50,
  }) async {
    final uri = Uri.parse(
      '$_baseUrl/api/notifications?after=$after&limit=$limit',
    );
    final resp = await http.get(uri);
    if (resp.statusCode != 200) {
      throw Exception('Failed to fetch notifications: ${resp.statusCode}');
    }
    final body = jsonDecode(resp.body) as Map<String, dynamic>;
    final events = body['events'] as List? ?? const [];
    return events
        .whereType<Map<String, dynamic>>()
        .map(GlobalNotificationEvent.fromApi)
        .toList();
  }

  SessionConnection connectToSession(String sessionId) {
    return SessionConnection(sessionId: sessionId, api: this);
  }
}

class GlobalNotificationEvent {
  final int id;
  final String eventType;
  final String sessionId;
  final String sessionName;
  final String title;
  final String body;
  final String? backend;
  final String? model;
  final String? sessionTitle; // LLM-generated human title
  final String? sessionSummary;
  final DateTime? createdAt;
  final Map<String, dynamic>? meta;

  GlobalNotificationEvent({
    required this.id,
    required this.eventType,
    required this.sessionId,
    required this.sessionName,
    required this.title,
    required this.body,
    this.backend,
    this.model,
    this.sessionTitle,
    this.sessionSummary,
    this.createdAt,
    this.meta,
  });

  factory GlobalNotificationEvent.fromWS(
    String type,
    Map<String, dynamic> data,
  ) {
    Map<String, dynamic>? parseMeta(dynamic raw) {
      if (raw == null) return null;
      if (raw is Map<String, dynamic>) return raw;
      if (raw is String) {
        try {
          final parsed = jsonDecode(raw);
          if (parsed is Map<String, dynamic>) return parsed;
        } catch (_) {}
      }
      return null;
    }

    DateTime? parseCreatedAt(dynamic v) {
      if (v == null) return null;
      if (v is String) return DateTime.tryParse(v);
      if (v is int) return DateTime.fromMillisecondsSinceEpoch(v);
      return null;
    }

    final metaRaw = data['meta'] ?? data['Meta'];
    final createdRaw = data['createdAt'] ?? data['created_at'] ?? data['created'];

    return GlobalNotificationEvent(
      id: (data['id'] as num?)?.toInt() ?? 0,
      eventType: type,
      sessionId: data['sessionId'] as String? ?? '',
      sessionName: data['sessionName'] as String? ?? 'Agent',
      title:
          data['title'] as String? ??
          (type == 'run_complete' ? 'Agent finished' : 'Tool approval needed'),
      body: data['body'] as String? ?? '',
      backend: data['backend'] as String?,
      model: data['model'] as String?,
      sessionTitle: data['sessionTitle'] as String?,
      sessionSummary: data['sessionSummary'] as String?,
      createdAt: parseCreatedAt(createdRaw),
      meta: parseMeta(metaRaw),
    );
  }

  factory GlobalNotificationEvent.fromApi(Map<String, dynamic> json) {
    Map<String, dynamic>? parseMeta(dynamic raw) {
      if (raw == null) return null;
      if (raw is Map<String, dynamic>) return raw;
      if (raw is String) {
        try {
          final parsed = jsonDecode(raw);
          if (parsed is Map<String, dynamic>) return parsed;
        } catch (_) {}
      }
      return null;
    }

    DateTime? parseCreatedAt(dynamic v) {
      if (v == null) return null;
      if (v is String) return DateTime.tryParse(v);
      if (v is int) return DateTime.fromMillisecondsSinceEpoch(v);
      return null;
    }

    final metaRaw = json['meta'] ?? json['Meta'];
    final createdRaw = json['createdAt'] ?? json['created_at'] ?? json['created'];

    return GlobalNotificationEvent(
      id: (json['ID'] as num?)?.toInt() ?? (json['id'] as num?)?.toInt() ?? 0,
      eventType:
          json['EventType'] as String? ?? json['event_type'] as String? ?? '',
      sessionId:
          json['SessionID'] as String? ?? json['session_id'] as String? ?? '',
      sessionName:
          json['SessionName'] as String? ??
          json['session_name'] as String? ??
          'Agent',
      title: json['Title'] as String? ?? json['title'] as String? ?? '',
      body: json['Body'] as String? ?? json['body'] as String? ?? '',
      backend: json['backend'] as String?,
      model: json['model'] as String?,
      sessionTitle: json['sessionTitle'] as String?,
      sessionSummary: json['sessionSummary'] as String?,
      createdAt: parseCreatedAt(createdRaw),
      meta: parseMeta(metaRaw),
    );
  }
}

class BrowseResult {
  final String path;
  final String parent;
  final List<BrowseEntry> entries;

  BrowseResult({
    required this.path,
    required this.parent,
    required this.entries,
  });

  factory BrowseResult.fromJson(Map<String, dynamic> json) {
    final entries = (json['entries'] as List? ?? [])
        .map((e) => BrowseEntry.fromJson(e))
        .toList();
    return BrowseResult(
      path: json['path'] ?? '',
      parent: json['parent'] ?? '',
      entries: entries,
    );
  }
}

class BrowseEntry {
  final String name;
  final bool isDir;
  final String path;

  BrowseEntry({required this.name, required this.isDir, required this.path});

  factory BrowseEntry.fromJson(Map<String, dynamic> json) {
    return BrowseEntry(
      name: json['name'] ?? '',
      isDir: json['isDir'] ?? false,
      path: json['path'] ?? '',
    );
  }
}

class SessionConnection {
  final String sessionId;
  final ApiService _api;
  WebSocketChannel? _channel;
  final StreamController<ChatMessage> _messageController =
      StreamController.broadcast();
  final StreamController<void> _historyResetController =
      StreamController.broadcast();
  Stream<void> get historyResetStream => _historyResetController.stream;
  final List<ChatMessage> messages = [];
  static const int _maxMessages = 2000;
  bool _closed = false;
  bool _disposed = false;
  int _reconnectAttempts = 0;
  static const int _maxReconnectAttempts = 10;
  Timer? _reconnectTimer;
  bool _backgrounded = false;

  Stream<ChatMessage> get messageStream => _messageController.stream;
  bool get isClosed => _closed;

  SessionConnection({required this.sessionId, required ApiService api})
    : _api = api {
    _connectChannel();
  }

  void _connectChannel() {
    // Close stale channel before opening a new one.
    try {
      _channel?.sink.close();
    } catch (_) {}

    final uri = Uri.parse('${_api._wsBase}/ws/sessions/$sessionId');
    _channel = WebSocketChannel.connect(uri);
    _closed = false;
    _channel!.stream.listen(
      (data) {
        _reconnectAttempts = 0; // reset on successful data
        try {
          final json = jsonDecode(data as String) as Map<String, dynamic>;
          final type = json['type'] as String;

          if (type == 'history') {
            final historyMsgs = (json['messages'] as List)
                .map((m) => ChatMessage.fromWS(m as Map<String, dynamic>))
                .toList();
            // Clear existing messages and signal the UI to reset before
            // re-rendering. This handles reconnects without showing duplicates.
            messages.clear();
            if (!_historyResetController.isClosed) {
              _historyResetController.add(null);
            }
            messages.addAll(historyMsgs);
            _trimMessages();
            for (final msg in historyMsgs) {
              if (!_messageController.isClosed) {
                _messageController.add(msg);
              }
            }
          } else {
            final msg = ChatMessage.fromWS(json);
            // Coalesce adjacent agent_text messages
            if (msg.type == MessageType.agentText &&
                messages.isNotEmpty &&
                messages.last.type == MessageType.agentText) {
              final combined = ChatMessage(
                type: MessageType.agentText,
                text: messages.last.text + msg.text,
                timestamp: messages.last.timestamp,
              );
              messages[messages.length - 1] = combined;
            } else {
              messages.add(msg);
              _trimMessages();
            }
            if (!_messageController.isClosed) {
              _messageController.add(msg);
            }
          }
        } catch (e) {
          debugPrint('WS parse error: $e');
        }
      },
      onError: (e) {
        if (!_messageController.isClosed) {
          _messageController.addError(e);
        }
        _scheduleReconnect();
      },
      onDone: () {
        _closed = true;
        _scheduleReconnect();
      },
    );
  }

  void _trimMessages() {
    if (messages.length > _maxMessages) {
      messages.removeRange(0, messages.length - _maxMessages);
    }
  }

  void _scheduleReconnect() {
    if (_disposed) return;
    // While backgrounded, don't burn through reconnect attempts — just
    // flag that a reconnect is needed and let onResume handle it.
    if (_backgrounded) return;
    if (_reconnectAttempts >= _maxReconnectAttempts) return;
    _reconnectAttempts++;
    final delay = Duration(seconds: _reconnectAttempts.clamp(1, 8));
    debugPrint(
      'WS reconnecting in ${delay.inSeconds}s (attempt $_reconnectAttempts)',
    );
    _reconnectTimer?.cancel();
    _reconnectTimer = Timer(delay, () {
      if (_disposed) return;
      _connectChannel();
      // Notify listeners that connection was re-established
      if (!_messageController.isClosed) {
        _messageController.add(
          ChatMessage(type: MessageType.status, text: 'Reconnected'),
        );
      }
    });
  }

  /// Called when the app moves to the background. Stops reconnect timers
  /// to avoid wasting attempts while the OS may have killed the socket.
  void onBackground() {
    _backgrounded = true;
    _reconnectTimer?.cancel();
  }

  /// Called when the app returns to the foreground. Forces an immediate
  /// reconnect if the WebSocket is closed, resetting the attempt counter
  /// so the connection always recovers after a resume.
  void onResume() {
    _backgrounded = false;
    if (_disposed) return;
    if (_closed) {
      // Reset counter so we get a fresh set of attempts.
      _reconnectAttempts = 0;
      _reconnectTimer?.cancel();
      _connectChannel();
      if (!_messageController.isClosed) {
        _messageController.add(
          ChatMessage(type: MessageType.status, text: 'Reconnected'),
        );
      }
    }
  }

  void sendPrompt(String text) {
    if (_closed || _channel == null) return;
    _channel!.sink.add(jsonEncode({'type': 'prompt', 'text': text}));
    // Don't add locally — server echoes back as prompt_sent
  }

  void sendInterrupt() {
    if (_closed || _channel == null) return;
    _channel!.sink.add(jsonEncode({'type': 'interrupt'}));
  }

  void respondToPermission(String requestId, String optionId) {
    if (_closed || _channel == null) return;
    _channel!.sink.add(
      jsonEncode({
        'type': 'permission_response',
        'requestId': requestId,
        'optionId': optionId,
      }),
    );
  }

  void dispose() {
    _disposed = true;
    _reconnectTimer?.cancel();
    _channel?.sink.close();
    if (!_messageController.isClosed) {
      _messageController.close();
    }
    if (!_historyResetController.isClosed) {
      _historyResetController.close();
    }
  }
}
