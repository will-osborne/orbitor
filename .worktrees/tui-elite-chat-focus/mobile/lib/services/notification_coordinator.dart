import 'dart:async';
import 'dart:io' show Platform;
import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:workmanager/workmanager.dart';
import 'api_service.dart';
import 'notification_service.dart';
import 'notification_worker.dart';

// Top-level FCM background message handler (must be a top-level function).
@pragma('vm:entry-point')
Future<void> _firebaseMessagingBackgroundHandler(RemoteMessage message) async {
  // When the app is in background/terminated, FCM delivers here.
  // The notification is shown automatically by FCM on Android if the message
  // has a notification payload. This handler runs for data-only messages.
}

class NotificationCoordinator {
  final ApiService _api;
  final NotificationService _notificationService;
  final StreamController<String> _sessionTapController =
      StreamController.broadcast();
  Timer? _pollTimer;
  int _lastSeenId = 0;
  final List<String> _pendingSessionTapIds = [];

  NotificationCoordinator(this._api, {NotificationService? notificationService})
    : _notificationService = notificationService ?? NotificationService();

  Stream<String> get sessionTapStream => _sessionTapController.stream;

  List<String> drainPendingSessionTapIds() {
    final pending = List<String>.from(_pendingSessionTapIds);
    _pendingSessionTapIds.clear();
    return pending;
  }

  Future<void> init() async {
    await _notificationService.init(onSessionTap: _handleNotificationTap);
    await _loadLastSeen();
    _api.onGlobalNotification = _handleRealtimeEvent;
    _api.startEventListener();
    await _advanceLastSeen();
    _startForegroundPolling();
    await _initBackgroundWorkerIfSupported();
    await _initFCM();
  }

  Future<void> _initFCM() async {
    if (kIsWeb) return;
    try {
      FirebaseMessaging.onBackgroundMessage(_firebaseMessagingBackgroundHandler);

      // Request permission (required on iOS, harmless on Android).
      await FirebaseMessaging.instance.requestPermission(
        alert: true,
        badge: true,
        sound: true,
      );

      // Register FCM token with server.
      final token = await FirebaseMessaging.instance.getToken();
      if (token != null) {
        await _registerFCMToken(token);
      }

      // Re-register if token refreshes.
      FirebaseMessaging.instance.onTokenRefresh.listen(_registerFCMToken);

      // Handle FCM messages when app is in foreground.
      FirebaseMessaging.onMessage.listen((RemoteMessage message) {
        final data = message.data;
        final eventType = data['eventType'] as String? ?? '';
        final sessionId = data['sessionId'] as String? ?? '';
        if (eventType.isEmpty || sessionId.isEmpty) return;

        final notification = message.notification;
        final title = notification?.title ?? data['title'] ?? '';
        final body = notification?.body ?? data['body'] ?? '';

        final event = GlobalNotificationEvent(
          id: 0,
          eventType: eventType,
          sessionId: sessionId,
          sessionName: title,
          title: title,
          body: body,
        );
        unawaited(_showAndTrack(event));
      });

      // Handle notification tap when app was in background (not terminated).
      FirebaseMessaging.onMessageOpenedApp.listen((RemoteMessage message) {
        final sessionId = message.data['sessionId'] as String? ?? '';
        _handleNotificationTap(sessionId);
      });

      // Handle notification tap when app was terminated.
      final initialMessage = await FirebaseMessaging.instance.getInitialMessage();
      if (initialMessage != null) {
        final sessionId = initialMessage.data['sessionId'] as String? ?? '';
        _handleNotificationTap(sessionId);
      }
    } catch (e) {
      // FCM init is best-effort; don't block the app if Firebase isn't configured.
    }
  }

  Future<void> _registerFCMToken(String token) async {
    try {
      final platform = Platform.isIOS ? 'ios' : 'android';
      await _api.registerDeviceToken(token, platform: platform);
    } catch (_) {}
  }

  Future<void> onBackground() async {
    _api.onBackground();
  }

  Future<void> onResume() async {
    _api.onResume();
    await _advanceLastSeen();
  }

  Future<void> onBaseUrlChanged(String baseUrl) async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(notificationBaseUrlKey, baseUrl);
    await _advanceLastSeen();
  }

  void dispose() {
    _pollTimer?.cancel();
    _sessionTapController.close();
  }

  void _startForegroundPolling() {
    _pollTimer?.cancel();
    _pollTimer = Timer.periodic(const Duration(seconds: 20), (_) async {
      await _advanceLastSeen();
    });
  }

  Future<void> _initBackgroundWorkerIfSupported() async {
    if (kIsWeb || !Platform.isAndroid) {
      return;
    }
    await Workmanager().initialize(
      notificationCallbackDispatcher,
      isInDebugMode: false,
    );
    await Workmanager().registerPeriodicTask(
      notificationWorkerTaskName,
      notificationWorkerTaskName,
      frequency: const Duration(minutes: 15),
      initialDelay: const Duration(minutes: 1),
      existingWorkPolicy: ExistingWorkPolicy.update,
    );
  }

  void _handleRealtimeEvent(GlobalNotificationEvent event) {
    unawaited(_showAndTrack(event));
  }

  void _handleNotificationTap(String sessionId) {
    if (sessionId.isEmpty) return;
    if (!_sessionTapController.hasListener) {
      _pendingSessionTapIds.add(sessionId);
      return;
    }
    _sessionTapController.add(sessionId);
  }

  Future<void> _showAndTrack(GlobalNotificationEvent event) async {
    if (event.id > 0 && event.id <= _lastSeenId) {
      return;
    }

    final displaySessionName = event.sessionTitle?.isNotEmpty == true
        ? event.sessionTitle!
        : event.sessionName;

    final body = event.body.isEmpty
        ? '${displaySessionName} has an update'
        : event.body;

    // Prefer specialized notifications for run_complete and permission_request
    if (event.eventType == 'run_complete') {
      // try to read stopReason from meta if present
      final stopReason = (event.meta != null && event.meta!['stopReason'] != null)
          ? event.meta!['stopReason'].toString()
          : event.body;
      await _notificationService.showRunComplete(
        displaySessionName,
        stopReason,
        sessionId: event.sessionId,
        id: event.id,
        createdAt: event.createdAt,
        meta: event.meta,
      );
    } else if (event.eventType == 'permission_request') {
      await _notificationService.showPermissionRequest(
        displaySessionName,
        sessionId: event.sessionId,
        id: event.id,
        createdAt: event.createdAt,
        meta: event.meta,
      );
    } else {
      await _notificationService.showGeneric(
        id: event.id,
        title: event.title,
        body: body,
        sessionId: event.sessionId,
        createdAt: event.createdAt,
      );
    }

    if (event.id > _lastSeenId) {
      _lastSeenId = event.id;
      await _persistLastSeen();
    }
  }

  // Silently advance _lastSeenId to the current max without showing OS
  // notifications. Used on startup and resume so FCM/workmanager duplicates
  // are not created.
  Future<void> _advanceLastSeen() async {
    try {
      final events = await _api.fetchNotifications(
        after: _lastSeenId,
        limit: 100,
      );
      if (events.isEmpty) return;
      int maxId = _lastSeenId;
      for (final event in events) {
        if (event.id > maxId) maxId = event.id;
      }
      if (maxId > _lastSeenId) {
        _lastSeenId = maxId;
        await _persistLastSeen();
      }
    } catch (_) {}
  }

  Future<void> _loadLastSeen() async {
    final prefs = await SharedPreferences.getInstance();
    _lastSeenId = prefs.getInt(notificationLastSeenIdKey) ?? 0;
    await prefs.setString(notificationBaseUrlKey, _api.baseUrl);
  }

  Future<void> _persistLastSeen() async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setInt(notificationLastSeenIdKey, _lastSeenId);
  }
}
