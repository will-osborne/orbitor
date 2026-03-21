import 'package:flutter_test/flutter_test.dart';
import 'package:copilot_bridge/main.dart';
import 'package:copilot_bridge/services/api_service.dart';
import 'package:copilot_bridge/services/notification_coordinator.dart';

void main() {
  testWidgets('App renders', (WidgetTester tester) async {
    await tester.pumpWidget(
      CopilotBridgeApp(
        notificationCoordinator: NotificationCoordinator(ApiService()),
      ),
    );
  });
}
