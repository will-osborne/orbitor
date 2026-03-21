import 'package:flutter/material.dart';
import '../theme.dart';

/// A cute animated robot booting up. Antenna blinks, eyes power on
/// with a sweep, body vibrates as systems come online.
class BootingRobot extends StatefulWidget {
  final double size;
  const BootingRobot({super.key, this.size = 120});

  @override
  State<BootingRobot> createState() => _BootingRobotState();
}

class _BootingRobotState extends State<BootingRobot>
    with TickerProviderStateMixin {
  late AnimationController _bootCtrl;
  late AnimationController _antennaCtrl;
  late AnimationController _shakeCtrl;

  late Animation<double> _eyeOpen;
  late Animation<double> _antennaBlink;
  late Animation<double> _bodyShake;
  late Animation<double> _glowPulse;

  @override
  void initState() {
    super.initState();

    _bootCtrl = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 2400),
    )..repeat();

    _antennaCtrl = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 800),
    )..repeat(reverse: true);

    _shakeCtrl = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 120),
    )..repeat(reverse: true);

    _eyeOpen = TweenSequence<double>([
      TweenSequenceItem(tween: Tween(begin: 0.0, end: 0.0), weight: 30),
      TweenSequenceItem(
          tween: Tween(begin: 0.0, end: 1.0)
              .chain(CurveTween(curve: Curves.elasticOut)),
          weight: 40),
      TweenSequenceItem(tween: Tween(begin: 1.0, end: 1.0), weight: 30),
    ]).animate(_bootCtrl);

    _antennaBlink = Tween<double>(begin: 0.3, end: 1.0).animate(
      CurvedAnimation(parent: _antennaCtrl, curve: Curves.easeInOut),
    );

    _bodyShake = Tween<double>(begin: -1.5, end: 1.5).animate(_shakeCtrl);

    _glowPulse = TweenSequence<double>([
      TweenSequenceItem(tween: Tween(begin: 0.0, end: 0.0), weight: 25),
      TweenSequenceItem(tween: Tween(begin: 0.0, end: 0.6), weight: 25),
      TweenSequenceItem(tween: Tween(begin: 0.6, end: 0.3), weight: 25),
      TweenSequenceItem(tween: Tween(begin: 0.3, end: 0.5), weight: 25),
    ]).animate(_bootCtrl);
  }

  @override
  void dispose() {
    _bootCtrl.dispose();
    _antennaCtrl.dispose();
    _shakeCtrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: Listenable.merge([_bootCtrl, _antennaCtrl, _shakeCtrl]),
      builder: (_, __) {
        return Transform.translate(
          offset: Offset(_bodyShake.value, 0),
          child: CustomPaint(
            size: Size(widget.size, widget.size),
            painter: _BootingRobotPainter(
              eyeOpen: _eyeOpen.value,
              antennaBrightness: _antennaBlink.value,
              glowIntensity: _glowPulse.value,
            ),
          ),
        );
      },
    );
  }
}

class _BootingRobotPainter extends CustomPainter {
  final double eyeOpen;
  final double antennaBrightness;
  final double glowIntensity;

  _BootingRobotPainter({
    required this.eyeOpen,
    required this.antennaBrightness,
    required this.glowIntensity,
  });

  @override
  void paint(Canvas canvas, Size size) {
    final s = size.width;
    final cx = s / 2;

    // Glow behind robot
    if (glowIntensity > 0) {
      final glowPaint = Paint()
        ..color = CB.cyan.withValues(alpha: glowIntensity * 0.15)
        ..maskFilter = const MaskFilter.blur(BlurStyle.normal, 30);
      canvas.drawCircle(Offset(cx, s * 0.45), s * 0.35, glowPaint);
    }

    // --- Antenna ---
    final antennaPaint = Paint()
      ..color = CB.textTertiary
      ..strokeWidth = 2.5
      ..style = PaintingStyle.stroke
      ..strokeCap = StrokeCap.round;
    canvas.drawLine(Offset(cx, s * 0.28), Offset(cx, s * 0.15), antennaPaint);

    // Antenna bulb
    final bulbPaint = Paint()
      ..color = CB.cyan.withValues(alpha: antennaBrightness);
    canvas.drawCircle(Offset(cx, s * 0.13), s * 0.035, bulbPaint);
    // Bulb glow
    final bulbGlow = Paint()
      ..color = CB.cyan.withValues(alpha: antennaBrightness * 0.4)
      ..maskFilter = const MaskFilter.blur(BlurStyle.normal, 8);
    canvas.drawCircle(Offset(cx, s * 0.13), s * 0.05, bulbGlow);

    // --- Ears ---
    final earPaint = Paint()
      ..color = CB.surfaceLight
      ..style = PaintingStyle.fill;
    final earBorder = Paint()
      ..color = Colors.white.withValues(alpha: 0.12)
      ..style = PaintingStyle.stroke
      ..strokeWidth = 1.5;
    // Left ear
    final leftEar =
        RRect.fromRectAndRadius(
      Rect.fromCenter(
          center: Offset(cx - s * 0.28, s * 0.37), width: s * 0.07, height: s * 0.14),
      const Radius.circular(4),
    );
    canvas.drawRRect(leftEar, earPaint);
    canvas.drawRRect(leftEar, earBorder);
    // Right ear
    final rightEar =
        RRect.fromRectAndRadius(
      Rect.fromCenter(
          center: Offset(cx + s * 0.28, s * 0.37), width: s * 0.07, height: s * 0.14),
      const Radius.circular(4),
    );
    canvas.drawRRect(rightEar, earPaint);
    canvas.drawRRect(rightEar, earBorder);

    // --- Head ---
    final headRect = RRect.fromRectAndRadius(
      Rect.fromCenter(
          center: Offset(cx, s * 0.37), width: s * 0.48, height: s * 0.30),
      const Radius.circular(16),
    );
    final headPaint = Paint()..color = CB.surfaceLight;
    final headBorder = Paint()
      ..color = Colors.white.withValues(alpha: 0.10)
      ..style = PaintingStyle.stroke
      ..strokeWidth = 1.5;
    canvas.drawRRect(headRect, headPaint);
    canvas.drawRRect(headRect, headBorder);

    // --- Eyes ---
    final eyeY = s * 0.37;
    final eyeSpacing = s * 0.10;
    final eyeRadius = s * 0.05;

    // Eye sockets
    final socketPaint = Paint()..color = const Color(0xFF06060E);
    canvas.drawCircle(Offset(cx - eyeSpacing, eyeY), eyeRadius + 2, socketPaint);
    canvas.drawCircle(Offset(cx + eyeSpacing, eyeY), eyeRadius + 2, socketPaint);

    // Eyes (cyan, opening with eyeOpen)
    if (eyeOpen > 0.01) {
      final eyePaint = Paint()..color = CB.cyan.withValues(alpha: eyeOpen);
      final scaledR = eyeRadius * eyeOpen;
      canvas.drawCircle(Offset(cx - eyeSpacing, eyeY), scaledR, eyePaint);
      canvas.drawCircle(Offset(cx + eyeSpacing, eyeY), scaledR, eyePaint);

      // Eye glow
      final eyeGlow = Paint()
        ..color = CB.cyan.withValues(alpha: eyeOpen * 0.3)
        ..maskFilter = const MaskFilter.blur(BlurStyle.normal, 6);
      canvas.drawCircle(Offset(cx - eyeSpacing, eyeY), scaledR * 1.4, eyeGlow);
      canvas.drawCircle(Offset(cx + eyeSpacing, eyeY), scaledR * 1.4, eyeGlow);

      // Pupils (tiny white dots)
      final pupilPaint = Paint()..color = Colors.white.withValues(alpha: eyeOpen);
      canvas.drawCircle(
          Offset(cx - eyeSpacing + 1.5, eyeY - 1.5), scaledR * 0.3, pupilPaint);
      canvas.drawCircle(
          Offset(cx + eyeSpacing + 1.5, eyeY - 1.5), scaledR * 0.3, pupilPaint);
    }

    // --- Mouth (little smile) ---
    final mouthPaint = Paint()
      ..color = CB.cyan.withValues(alpha: eyeOpen * 0.6)
      ..style = PaintingStyle.stroke
      ..strokeWidth = 2
      ..strokeCap = StrokeCap.round;
    final mouthPath = Path()
      ..moveTo(cx - s * 0.06, s * 0.44)
      ..quadraticBezierTo(cx, s * 0.47, cx + s * 0.06, s * 0.44);
    canvas.drawPath(mouthPath, mouthPaint);

    // --- Body ---
    final bodyRect = RRect.fromRectAndRadius(
      Rect.fromCenter(
          center: Offset(cx, s * 0.62), width: s * 0.38, height: s * 0.22),
      const Radius.circular(12),
    );
    canvas.drawRRect(bodyRect, headPaint);
    canvas.drawRRect(bodyRect, headBorder);

    // Chest light (loading bar effect)
    final barWidth = s * 0.24;
    final barHeight = s * 0.03;
    final barLeft = cx - barWidth / 2;
    final barY = s * 0.60;
    // Background bar
    final barBg = Paint()..color = const Color(0xFF06060E);
    canvas.drawRRect(
      RRect.fromRectAndRadius(
          Rect.fromLTWH(barLeft, barY, barWidth, barHeight),
          const Radius.circular(4)),
      barBg,
    );
    // Lit portion
    if (glowIntensity > 0) {
      final litWidth = barWidth * glowIntensity.clamp(0.0, 1.0);
      final barLit = Paint()
        ..shader = const LinearGradient(
          colors: [CB.cyan, CB.purple],
        ).createShader(Rect.fromLTWH(barLeft, barY, litWidth, barHeight));
      canvas.drawRRect(
        RRect.fromRectAndRadius(
            Rect.fromLTWH(barLeft, barY, litWidth, barHeight),
            const Radius.circular(4)),
        barLit,
      );
    }

    // --- Arms (little nubs) ---
    final armPaint = Paint()..color = CB.surfaceLight;
    // Left arm
    canvas.drawRRect(
      RRect.fromRectAndRadius(
        Rect.fromCenter(
            center: Offset(cx - s * 0.23, s * 0.61), width: s * 0.07, height: s * 0.10),
        const Radius.circular(5),
      ),
      armPaint,
    );
    canvas.drawRRect(
      RRect.fromRectAndRadius(
        Rect.fromCenter(
            center: Offset(cx - s * 0.23, s * 0.61), width: s * 0.07, height: s * 0.10),
        const Radius.circular(5),
      ),
      headBorder,
    );
    // Right arm
    canvas.drawRRect(
      RRect.fromRectAndRadius(
        Rect.fromCenter(
            center: Offset(cx + s * 0.23, s * 0.61), width: s * 0.07, height: s * 0.10),
        const Radius.circular(5),
      ),
      armPaint,
    );
    canvas.drawRRect(
      RRect.fromRectAndRadius(
        Rect.fromCenter(
            center: Offset(cx + s * 0.23, s * 0.61), width: s * 0.07, height: s * 0.10),
        const Radius.circular(5),
      ),
      headBorder,
    );

    // --- Legs / feet ---
    final footPaint = Paint()..color = CB.surfaceLight;
    canvas.drawRRect(
      RRect.fromRectAndRadius(
        Rect.fromCenter(
            center: Offset(cx - s * 0.08, s * 0.78), width: s * 0.10, height: s * 0.06),
        const Radius.circular(4),
      ),
      footPaint,
    );
    canvas.drawRRect(
      RRect.fromRectAndRadius(
        Rect.fromCenter(
            center: Offset(cx + s * 0.08, s * 0.78), width: s * 0.10, height: s * 0.06),
        const Radius.circular(4),
      ),
      footPaint,
    );
    // Leg connectors
    final legPaint = Paint()
      ..color = CB.textTertiary
      ..strokeWidth = 2.5
      ..strokeCap = StrokeCap.round;
    canvas.drawLine(
        Offset(cx - s * 0.08, s * 0.73), Offset(cx - s * 0.08, s * 0.75), legPaint);
    canvas.drawLine(
        Offset(cx + s * 0.08, s * 0.73), Offset(cx + s * 0.08, s * 0.75), legPaint);
  }

  @override
  bool shouldRepaint(covariant _BootingRobotPainter old) =>
      old.eyeOpen != eyeOpen ||
      old.antennaBrightness != antennaBrightness ||
      old.glowIntensity != glowIntensity;
}

/// A cute animated robot typing on a laptop. Arms move, screen flickers,
/// antenna sways slightly.
class WorkingRobot extends StatefulWidget {
  final double size;
  const WorkingRobot({super.key, this.size = 120});

  @override
  State<WorkingRobot> createState() => _WorkingRobotState();
}

class _WorkingRobotState extends State<WorkingRobot>
    with TickerProviderStateMixin {
  late AnimationController _typeCtrl;
  late AnimationController _swayCtrl;
  late AnimationController _screenCtrl;

  late Animation<double> _leftArm;
  late Animation<double> _rightArm;
  late Animation<double> _headSway;
  late Animation<double> _screenFlicker;

  @override
  void initState() {
    super.initState();

    _typeCtrl = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 300),
    )..repeat(reverse: true);

    _swayCtrl = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 2000),
    )..repeat(reverse: true);

    _screenCtrl = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 1400),
    )..repeat();

    _leftArm = TweenSequence<double>([
      TweenSequenceItem(tween: Tween(begin: 0.0, end: -3.0), weight: 50),
      TweenSequenceItem(tween: Tween(begin: -3.0, end: 0.0), weight: 50),
    ]).animate(_typeCtrl);

    _rightArm = TweenSequence<double>([
      TweenSequenceItem(tween: Tween(begin: -2.0, end: 1.0), weight: 50),
      TweenSequenceItem(tween: Tween(begin: 1.0, end: -2.0), weight: 50),
    ]).animate(_typeCtrl);

    _headSway = Tween<double>(begin: -2.0, end: 2.0).animate(
      CurvedAnimation(parent: _swayCtrl, curve: Curves.easeInOut),
    );

    _screenFlicker = TweenSequence<double>([
      TweenSequenceItem(tween: Tween(begin: 0.5, end: 0.8), weight: 30),
      TweenSequenceItem(tween: Tween(begin: 0.8, end: 0.6), weight: 20),
      TweenSequenceItem(tween: Tween(begin: 0.6, end: 1.0), weight: 30),
      TweenSequenceItem(tween: Tween(begin: 1.0, end: 0.5), weight: 20),
    ]).animate(_screenCtrl);
  }

  @override
  void dispose() {
    _typeCtrl.dispose();
    _swayCtrl.dispose();
    _screenCtrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: Listenable.merge([_typeCtrl, _swayCtrl, _screenCtrl]),
      builder: (_, __) {
        return CustomPaint(
          size: Size(widget.size, widget.size),
          painter: _WorkingRobotPainter(
            leftArmOffset: _leftArm.value,
            rightArmOffset: _rightArm.value,
            headSway: _headSway.value,
            screenBrightness: _screenFlicker.value,
          ),
        );
      },
    );
  }
}

class _WorkingRobotPainter extends CustomPainter {
  final double leftArmOffset;
  final double rightArmOffset;
  final double headSway;
  final double screenBrightness;

  _WorkingRobotPainter({
    required this.leftArmOffset,
    required this.rightArmOffset,
    required this.headSway,
    required this.screenBrightness,
  });

  @override
  void paint(Canvas canvas, Size size) {
    final s = size.width;
    final cx = s / 2;

    final headBorder = Paint()
      ..color = Colors.white.withValues(alpha: 0.10)
      ..style = PaintingStyle.stroke
      ..strokeWidth = 1.5;
    final partPaint = Paint()..color = CB.surfaceLight;

    // --- Laptop base ---
    final laptopY = s * 0.75;
    final laptopW = s * 0.55;
    final laptopH = s * 0.04;
    final laptopRect = RRect.fromRectAndRadius(
      Rect.fromCenter(
          center: Offset(cx, laptopY), width: laptopW, height: laptopH),
      const Radius.circular(3),
    );
    final laptopPaint = Paint()..color = const Color(0xFF1A1A2E);
    canvas.drawRRect(laptopRect, laptopPaint);
    canvas.drawRRect(laptopRect, headBorder);

    // --- Laptop screen ---
    final screenW = s * 0.40;
    final screenH = s * 0.26;
    final screenBottom = laptopY - laptopH / 2;
    final screenRect = RRect.fromRectAndRadius(
      Rect.fromLTWH(cx - screenW / 2, screenBottom - screenH, screenW, screenH),
      const Radius.circular(6),
    );
    final screenBg = Paint()..color = const Color(0xFF06060E);
    canvas.drawRRect(screenRect, screenBg);
    canvas.drawRRect(screenRect, headBorder);

    // Screen content glow
    final contentRect =
        Rect.fromLTWH(cx - screenW / 2 + 4, screenBottom - screenH + 4, screenW - 8, screenH - 8);
    final screenGlow = Paint()
      ..color = CB.cyan.withValues(alpha: screenBrightness * 0.12);
    canvas.drawRect(contentRect, screenGlow);

    // Fake code lines on screen
    final linePaint = Paint()
      ..strokeWidth = 1.5
      ..strokeCap = StrokeCap.round;
    final rng = [0.7, 0.5, 0.85, 0.4, 0.65, 0.55];
    for (var i = 0; i < 6; i++) {
      final ly = contentRect.top + 6 + i * 6.0;
      if (ly > contentRect.bottom - 4) break;
      final lw = contentRect.width * rng[i] * 0.8;
      linePaint.color =
          (i % 3 == 0 ? CB.purple : CB.cyan).withValues(alpha: screenBrightness * 0.35);
      canvas.drawLine(
          Offset(contentRect.left + 4, ly), Offset(contentRect.left + 4 + lw, ly), linePaint);
    }

    // --- Robot sitting behind the laptop ---
    // Head (offset by sway)
    final headCx = cx + headSway;
    final headCy = screenBottom - screenH - s * 0.06;

    // Antenna
    final antPaint = Paint()
      ..color = CB.textTertiary
      ..strokeWidth = 2
      ..strokeCap = StrokeCap.round;
    canvas.drawLine(
        Offset(headCx, headCy - s * 0.08), Offset(headCx, headCy - s * 0.15), antPaint);
    final bulb = Paint()..color = CB.neonGreen.withValues(alpha: 0.9);
    canvas.drawCircle(Offset(headCx, headCy - s * 0.16), s * 0.025, bulb);
    final bulbGlow = Paint()
      ..color = CB.neonGreen.withValues(alpha: 0.3)
      ..maskFilter = const MaskFilter.blur(BlurStyle.normal, 5);
    canvas.drawCircle(Offset(headCx, headCy - s * 0.16), s * 0.04, bulbGlow);

    // Head shape
    final headRect = RRect.fromRectAndRadius(
      Rect.fromCenter(
          center: Offset(headCx, headCy), width: s * 0.32, height: s * 0.20),
      const Radius.circular(12),
    );
    canvas.drawRRect(headRect, partPaint);
    canvas.drawRRect(headRect, headBorder);

    // Eyes (happy squints while working)
    final eyeY = headCy + s * 0.01;
    final eyeSpacing = s * 0.065;
    final eyePaint = Paint()
      ..color = CB.cyan
      ..strokeWidth = 2.5
      ..style = PaintingStyle.stroke
      ..strokeCap = StrokeCap.round;
    // Left eye - happy arc
    final leftEyePath = Path()
      ..moveTo(headCx - eyeSpacing - s * 0.03, eyeY)
      ..quadraticBezierTo(headCx - eyeSpacing, eyeY - s * 0.025,
          headCx - eyeSpacing + s * 0.03, eyeY);
    canvas.drawPath(leftEyePath, eyePaint);
    // Right eye - happy arc
    final rightEyePath = Path()
      ..moveTo(headCx + eyeSpacing - s * 0.03, eyeY)
      ..quadraticBezierTo(headCx + eyeSpacing, eyeY - s * 0.025,
          headCx + eyeSpacing + s * 0.03, eyeY);
    canvas.drawPath(rightEyePath, eyePaint);

    // Eye glow
    final eyeGlowPaint = Paint()
      ..color = CB.cyan.withValues(alpha: 0.2)
      ..maskFilter = const MaskFilter.blur(BlurStyle.normal, 4);
    canvas.drawCircle(Offset(headCx - eyeSpacing, eyeY), s * 0.03, eyeGlowPaint);
    canvas.drawCircle(Offset(headCx + eyeSpacing, eyeY), s * 0.03, eyeGlowPaint);

    // --- Arms (reaching to keyboard, bobbing) ---
    final armStroke = Paint()
      ..color = CB.surfaceLight
      ..strokeWidth = s * 0.045
      ..strokeCap = StrokeCap.round;
    final armBorderStroke = Paint()
      ..color = Colors.white.withValues(alpha: 0.10)
      ..strokeWidth = s * 0.045 + 1.5
      ..strokeCap = StrokeCap.round;

    // Left arm
    final leftArmStart = Offset(cx - s * 0.18, screenBottom - screenH * 0.3);
    final leftArmEnd =
        Offset(cx - s * 0.14, laptopY - laptopH / 2 + leftArmOffset);
    canvas.drawLine(leftArmStart, leftArmEnd, armBorderStroke);
    canvas.drawLine(leftArmStart, leftArmEnd, armStroke);

    // Right arm
    final rightArmStart = Offset(cx + s * 0.18, screenBottom - screenH * 0.3);
    final rightArmEnd =
        Offset(cx + s * 0.14, laptopY - laptopH / 2 + rightArmOffset);
    canvas.drawLine(rightArmStart, rightArmEnd, armBorderStroke);
    canvas.drawLine(rightArmStart, rightArmEnd, armStroke);

    // Little "hand" dots
    final handPaint = Paint()..color = CB.textTertiary;
    canvas.drawCircle(leftArmEnd, s * 0.018, handPaint);
    canvas.drawCircle(rightArmEnd, s * 0.018, handPaint);
  }

  @override
  bool shouldRepaint(covariant _WorkingRobotPainter old) =>
      old.leftArmOffset != leftArmOffset ||
      old.rightArmOffset != rightArmOffset ||
      old.headSway != headSway ||
      old.screenBrightness != screenBrightness;
}
