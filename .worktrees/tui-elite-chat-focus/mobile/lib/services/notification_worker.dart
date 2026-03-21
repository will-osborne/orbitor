import 'package:flutter/widgets.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:workmanager/workmanager.dart';
import 'dart:ui';
import 'api_service.dart';
import 'notification_service.dart';

const String notificationWorkerTaskName = 'copilotBridgeNotificationPoll';
const String notificationLastSeenIdKey = 'notifications_last_seen_id';
const String notificationBaseUrlKey = 'api_base_url';

@pragma('vm:entry-point')
void notificationCallbackDispatcher() {
  Workmanager().executeTask((task, inputData) async {
    WidgetsFlutterBinding.ensureInitialized();
    DartPluginRegistrant.ensureInitialized();

    final prefs = await SharedPreferences.getInstance();
    final baseUrl =
        prefs.getString(notificationBaseUrlKey) ?? ApiService.defaultBaseUrl;
    final lastSeen = prefs.getInt(notificationLastSeenIdKey) ?? 0;

    final api = ApiService(baseUrl: baseUrl);
    final notificationService = NotificationService();
    await notificationService.init();

    try {
      final events = await api.fetchNotifications(after: lastSeen, limit: 50);
      var maxSeen = lastSeen;
      for (final event in events) {
        final body = event.body.isEmpty
            ? '${event.sessionName} has an update'
            : event.body;
        await notificationService.showGeneric(
          id: event.id,
          title: event.title,
          body: body,
          sessionId: event.sessionId,
        );
        if (event.id > maxSeen) {
          maxSeen = event.id;
        }
      }
      if (maxSeen > lastSeen) {
        await prefs.setInt(notificationLastSeenIdKey, maxSeen);
      }
      return true;
    } catch (_) {
      return true;
    }
  });
}
