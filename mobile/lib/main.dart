import 'dart:async';
import 'package:firebase_core/firebase_core.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:provider/provider.dart';
import 'models/session.dart';
import 'screens/chat_screen.dart';
import 'services/api_service.dart';
import 'services/notification_coordinator.dart';
import 'screens/onboarding_screen.dart';
import 'screens/sessions_screen.dart';
import 'theme.dart';

void main() async {
  WidgetsFlutterBinding.ensureInitialized();
  await Firebase.initializeApp();
  SystemChrome.setSystemUIOverlayStyle(
    const SystemUiOverlayStyle(
      statusBarColor: Colors.transparent,
      statusBarIconBrightness: Brightness.light,
      systemNavigationBarColor: CB.black,
      systemNavigationBarIconBrightness: Brightness.light,
    ),
  );
  final api = await ApiService.create();
  final notificationCoordinator = NotificationCoordinator(api);
  await notificationCoordinator.init();
  runApp(
    ChangeNotifierProvider.value(
      value: api,
      child: CopilotBridgeApp(notificationCoordinator: notificationCoordinator),
    ),
  );
}

class CopilotBridgeApp extends StatefulWidget {
  final NotificationCoordinator notificationCoordinator;
  const CopilotBridgeApp({super.key, required this.notificationCoordinator});

  @override
  State<CopilotBridgeApp> createState() => _CopilotBridgeAppState();
}

class _CopilotBridgeAppState extends State<CopilotBridgeApp>
    with WidgetsBindingObserver {
  final GlobalKey<NavigatorState> _navigatorKey = GlobalKey<NavigatorState>();
  StreamSubscription<String>? _notificationTapSub;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _notificationTapSub = widget.notificationCoordinator.sessionTapStream.listen(
      (sessionId) => unawaited(_openSessionFromNotification(sessionId)),
    );
    for (final sessionId
        in widget.notificationCoordinator.drainPendingSessionTapIds()) {
      unawaited(_openSessionFromNotification(sessionId));
    }
  }

  @override
  void dispose() {
    _notificationTapSub?.cancel();
    WidgetsBinding.instance.removeObserver(this);
    widget.notificationCoordinator.dispose();
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.paused ||
        state == AppLifecycleState.inactive) {
      widget.notificationCoordinator.onBackground();
    }
    if (state == AppLifecycleState.resumed) {
      widget.notificationCoordinator.onResume();
    }
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      navigatorKey: _navigatorKey,
      title: 'Orbitor',
      debugShowCheckedModeBanner: false,
      theme: buildAppTheme(),
      home: Consumer<ApiService>(
        builder: (context, api, _) =>
            api.isConfigured ? const SessionsScreen() : const OnboardingScreen(),
      ),
    );
  }

  Future<void> _openSessionFromNotification(String sessionId) async {
    if (sessionId.isEmpty || !mounted) return;
    try {
      final api = context.read<ApiService>();
      final sessions = await api.listSessions();
      Session? match;
      for (final session in sessions) {
        if (session.id == sessionId) {
          match = session;
          break;
        }
      }
      final selected = match;
      if (selected == null) return;

      final navigator = _navigatorKey.currentState;
      if (navigator == null) return;
      navigator.push(
        PageRouteBuilder(
          pageBuilder: (context, animation, secondaryAnimation) =>
              ChatScreen(session: selected),
          transitionsBuilder: (context, anim, secondaryAnimation, child) {
            return FadeTransition(
              opacity: anim,
              child: SlideTransition(
                position: Tween(
                  begin: const Offset(0.03, 0),
                  end: Offset.zero,
                ).animate(
                  CurvedAnimation(parent: anim, curve: Curves.easeOutCubic),
                ),
                child: child,
              ),
            );
          },
          transitionDuration: const Duration(milliseconds: 350),
        ),
      );
    } catch (e) {
      debugPrint('Failed to route notification tap: $e');
    }
  }
}
