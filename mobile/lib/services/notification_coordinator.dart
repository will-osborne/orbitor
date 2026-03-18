import 'dart:async';
import 'dart:io' show Platform;
import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:shared_preferences/shared_preferences.dart';
import 'package:workmanager/workmanager.dart';
import 'api_service.dart';
import 'notification_service.dart';
import 'notification_worker.dart';

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
    await _syncMissedNotifications();
    _startForegroundPolling();
    await _initBackgroundWorkerIfSupported();
  }

  Future<void> onBackground() async {
    _api.onBackground();
  }

  Future<void> onResume() async {
    _api.onResume();
    await _syncMissedNotifications();
  }

  Future<void> onBaseUrlChanged(String baseUrl) async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(notificationBaseUrlKey, baseUrl);
    await _syncMissedNotifications();
  }

  void dispose() {
    _pollTimer?.cancel();
    _sessionTapController.close();
  }

  void _startForegroundPolling() {
    _pollTimer?.cancel();
    _pollTimer = Timer.periodic(const Duration(seconds: 20), (_) async {
      await _syncMissedNotifications();
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

  Future<void> _syncMissedNotifications() async {
    try {
      final events = await _api.fetchNotifications(
        after: _lastSeenId,
        limit: 100,
      );
      for (final event in events) {
        await _showAndTrack(event);
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
