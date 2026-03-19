import 'package:flutter_test/flutter_test.dart';
import 'package:provider/provider.dart';
import 'package:orbitor/main.dart';
import 'package:orbitor/services/api_service.dart';
import 'package:orbitor/services/notification_coordinator.dart';

void main() {
  testWidgets('App renders', (WidgetTester tester) async {
    final api = ApiService();
    final coordinator = NotificationCoordinator(api);
    await tester.pumpWidget(
      ChangeNotifierProvider.value(
        value: api,
        child: CopilotBridgeApp(notificationCoordinator: coordinator),
      ),
    );
  });
}
