import 'dart:convert';
import 'dart:io' show Platform;
import 'package:flutter/foundation.dart' show debugPrint, kIsWeb;
import 'package:flutter_local_notifications/flutter_local_notifications.dart';

class NotificationService {
  static final NotificationService _instance = NotificationService._();
  factory NotificationService() => _instance;
  NotificationService._();

  final FlutterLocalNotificationsPlugin _plugin =
      FlutterLocalNotificationsPlugin();
  bool _initialized = false;
  void Function(String sessionId)? _onSessionTap;

  Future<void> init({void Function(String sessionId)? onSessionTap}) async {
    if (onSessionTap != null) {
      _onSessionTap = onSessionTap;
    }
    if (_initialized) return;
    // Notifications aren't supported on web builds.
    if (kIsWeb) {
      _initialized = true;
      return;
    }
    const android = AndroidInitializationSettings('@mipmap/ic_launcher');
    const darwin = DarwinInitializationSettings(
      requestAlertPermission: true,
      requestBadgePermission: true,
      requestSoundPermission: true,
    );
    const settings = InitializationSettings(
      android: android,
      iOS: darwin,
      macOS: darwin,
    );
    await _plugin.initialize(
      settings,
      onDidReceiveNotificationResponse: (response) {
        _handleNotificationPayload(response.payload);
      },
    );
    final launchDetails = await _plugin.getNotificationAppLaunchDetails();
    if (launchDetails?.didNotificationLaunchApp ?? false) {
      _handleNotificationPayload(launchDetails?.notificationResponse?.payload);
    }
    _initialized = true;

    // Request runtime notification permission on Android 13+ (API 33).
    if (!kIsWeb && Platform.isAndroid) {
      await _plugin
          .resolvePlatformSpecificImplementation<
            AndroidFlutterLocalNotificationsPlugin
          >()
          ?.requestNotificationsPermission();
    }
  }

  Future<void> showRunComplete(
    String sessionName,
    String stopReason, {
    String sessionId = '',
    int id = 0,
    DateTime? createdAt,
    Map<String, dynamic>? meta,
  }) async {
    if (!_initialized) return;
    await _plugin.show(
      id == 0 ? DateTime.now().millisecondsSinceEpoch ~/ 1000 : id,
      'Agent finished',
      '$sessionName — $stopReason',
      const NotificationDetails(
        android: AndroidNotificationDetails(
          'agent_events',
          'Agent Events',
          channelDescription:
              'Notifications when agent finishes or needs attention',
          importance: Importance.high,
          priority: Priority.high,
        ),
        iOS: DarwinNotificationDetails(),
      ),
      payload: _buildPayload(sessionId, id: id, createdAt: createdAt),
    );
  }

  Future<void> showPermissionRequest(
    String sessionName, {
    String sessionId = '',
    int id = 0,
    DateTime? createdAt,
    Map<String, dynamic>? meta,
  }) async {
    if (!_initialized) return;
    await _plugin.show(
      id == 0 ? DateTime.now().millisecondsSinceEpoch ~/ 1000 : id,
      'Permission needed',
      '$sessionName is waiting for approval',
      const NotificationDetails(
        android: AndroidNotificationDetails(
          'agent_events',
          'Agent Events',
          channelDescription:
              'Notifications when agent finishes or needs attention',
          importance: Importance.max,
          priority: Priority.max,
        ),
        iOS: DarwinNotificationDetails(),
      ),
      payload: _buildPayload(sessionId, id: id, createdAt: createdAt),
    );
  }

  Future<void> showGeneric({
    required int id,
    required String title,
    required String body,
    String sessionId = '',
    DateTime? createdAt,
  }) async {
    if (!_initialized) return;
    await _plugin.show(
      id == 0 ? DateTime.now().millisecondsSinceEpoch ~/ 1000 : id,
      title,
      body,
      const NotificationDetails(
        android: AndroidNotificationDetails(
          'agent_events',
          'Agent Events',
          channelDescription:
              'Notifications when agent finishes or needs attention',
          importance: Importance.high,
          priority: Priority.high,
        ),
        iOS: DarwinNotificationDetails(),
      ),
      payload: _buildPayload(sessionId, id: id, createdAt: createdAt),
    );
  }

  String? _buildPayload(String sessionId, {int id = 0, DateTime? createdAt}) {
    if (sessionId.isEmpty && id == 0 && createdAt == null) return null;
    final payload = <String, dynamic>{};
    if (sessionId.isNotEmpty) payload['sessionId'] = sessionId;
    if (id != 0) payload['notificationId'] = id;
    if (createdAt != null) payload['createdAt'] = createdAt.toIso8601String();
    return jsonEncode(payload);
  }

  void _handleNotificationPayload(String? payload) {
    if (payload == null || payload.isEmpty || _onSessionTap == null) return;
    try {
      final decoded = jsonDecode(payload);
      if (decoded is Map<String, dynamic>) {
        final sessionId = decoded['sessionId'] as String? ?? '';
        if (sessionId.isNotEmpty) {
          _onSessionTap?.call(sessionId);
        }
      }
    } catch (e) {
      debugPrint('Ignoring invalid notification payload: $e');
    }
  }
}
