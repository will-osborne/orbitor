import 'dart:ui';
import 'package:flutter/material.dart';

// --- Copilot Bridge Design System ---
// True-black OLED base, neon accents, glassmorphism surfaces.

abstract final class CB {
  // Base palette
  static const black = Color(0xFF000000);
  static const surface = Color(0xFF0A0A0F);
  static const surfaceLight = Color(0xFF12121F);

  // Neon accents
  static const cyan = Color(0xFF00F0FF);
  static const purple = Color(0xFFA855F7);
  static const pink = Color(0xFFFF3CAC);
  static const neonGreen = Color(0xFF00FF87);
  static const amber = Color(0xFFFFB800);
  static const hotPink = Color(0xFFFF3366);

  // Text
  static const textPrimary = Color(0xFFFFFFFF);
  static const textSecondary = Color(0xFF7B7B9E);
  static const textTertiary = Color(0xFF4A4A6A);

  // Gradients
  static const accentGradient = LinearGradient(
    colors: [cyan, purple],
    begin: Alignment.topLeft,
    end: Alignment.bottomRight,
  );

  static const accentGradientHoriz = LinearGradient(
    colors: [cyan, purple, pink],
    begin: Alignment.centerLeft,
    end: Alignment.centerRight,
  );

  static const warmGradient = LinearGradient(
    colors: [Color(0xFFFF6B00), amber, Color(0xFFFFE066)],
    begin: Alignment.topLeft,
    end: Alignment.bottomRight,
  );

  static const dangerGradient = LinearGradient(
    colors: [hotPink, Color(0xFFFF6644)],
    begin: Alignment.topLeft,
    end: Alignment.bottomRight,
  );

  // Glass surface properties
  static const glassOpacity = 0.06;
  static const glassBorderOpacity = 0.10;
  static const glassBlur = 24.0;
}

/// Glassmorphic container with optional gradient border.
class GlassCard extends StatelessWidget {
  final Widget child;
  final EdgeInsetsGeometry? padding;
  final EdgeInsetsGeometry? margin;
  final Gradient? borderGradient;
  final double borderWidth;
  final double borderRadius;
  final Color? backgroundColor;
  final VoidCallback? onTap;

  const GlassCard({
    super.key,
    required this.child,
    this.padding,
    this.margin,
    this.borderGradient,
    this.borderWidth = 1.0,
    this.borderRadius = 16.0,
    this.backgroundColor,
    this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    final radius = BorderRadius.circular(borderRadius);
    final bg = backgroundColor ?? Colors.white.withValues(alpha: CB.glassOpacity);

    Widget card = ClipRRect(
      borderRadius: radius,
      child: BackdropFilter(
        filter: ImageFilter.blur(sigmaX: CB.glassBlur, sigmaY: CB.glassBlur),
        child: Container(
          padding: padding ?? const EdgeInsets.all(16),
          decoration: BoxDecoration(
            color: bg,
            borderRadius: radius,
          ),
          child: child,
        ),
      ),
    );

    // Gradient border wrapper
    if (borderGradient != null) {
      card = Container(
        decoration: BoxDecoration(
          gradient: borderGradient,
          borderRadius: radius,
        ),
        padding: EdgeInsets.all(borderWidth),
        child: Container(
          decoration: BoxDecoration(
            color: CB.surface,
            borderRadius: BorderRadius.circular(borderRadius - borderWidth),
          ),
          child: ClipRRect(
            borderRadius: BorderRadius.circular(borderRadius - borderWidth),
            child: BackdropFilter(
              filter: ImageFilter.blur(sigmaX: CB.glassBlur, sigmaY: CB.glassBlur),
              child: Container(
                padding: padding ?? const EdgeInsets.all(16),
                decoration: BoxDecoration(
                  color: bg,
                  borderRadius: BorderRadius.circular(borderRadius - borderWidth),
                ),
                child: child,
              ),
            ),
          ),
        ),
      );
    }

    if (margin != null) {
      card = Padding(padding: margin!, child: card);
    }

    if (onTap != null) {
      card = GestureDetector(onTap: onTap, child: card);
    }

    return card;
  }
}

/// Gradient text.
class GradientText extends StatelessWidget {
  final String text;
  final TextStyle style;
  final Gradient gradient;

  const GradientText(
    this.text, {
    super.key,
    required this.style,
    this.gradient = CB.accentGradient,
  });

  @override
  Widget build(BuildContext context) {
    return ShaderMask(
      blendMode: BlendMode.srcIn,
      shaderCallback: (bounds) => gradient.createShader(
        Rect.fromLTWH(0, 0, bounds.width, bounds.height),
      ),
      child: Text(text, style: style),
    );
  }
}

/// Pulsing dot indicator for animated status.
class PulsingDot extends StatefulWidget {
  final Color color;
  final double size;
  const PulsingDot({super.key, required this.color, this.size = 10});

  @override
  State<PulsingDot> createState() => _PulsingDotState();
}

class _PulsingDotState extends State<PulsingDot> with SingleTickerProviderStateMixin {
  late AnimationController _ctrl;

  @override
  void initState() {
    super.initState();
    _ctrl = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 1200),
    )..repeat(reverse: true);
  }

  @override
  void dispose() {
    _ctrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: _ctrl,
      builder: (_, __) {
        final scale = 0.6 + 0.4 * _ctrl.value;
        final opacity = 0.4 + 0.6 * _ctrl.value;
        return Transform.scale(
          scale: scale,
          child: Container(
            width: widget.size,
            height: widget.size,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              color: widget.color.withValues(alpha: opacity),
              boxShadow: [
                BoxShadow(
                  color: widget.color.withValues(alpha: opacity * 0.5),
                  blurRadius: widget.size * 2,
                  spreadRadius: widget.size * 0.5,
                ),
              ],
            ),
          ),
        );
      },
    );
  }
}

/// Gradient border button.
class GradientButton extends StatelessWidget {
  final VoidCallback? onPressed;
  final Widget child;
  final Gradient gradient;
  final double borderRadius;
  final EdgeInsetsGeometry padding;

  const GradientButton({
    super.key,
    required this.onPressed,
    required this.child,
    this.gradient = CB.accentGradient,
    this.borderRadius = 14,
    this.padding = const EdgeInsets.symmetric(horizontal: 24, vertical: 14),
  });

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onPressed,
      child: Container(
        decoration: BoxDecoration(
          gradient: onPressed != null ? gradient : null,
          color: onPressed == null ? CB.textTertiary : null,
          borderRadius: BorderRadius.circular(borderRadius),
          boxShadow: onPressed != null
              ? [
                  BoxShadow(
                    color: CB.cyan.withValues(alpha: 0.25),
                    blurRadius: 16,
                    offset: const Offset(0, 4),
                  ),
                ]
              : null,
        ),
        padding: padding,
        child: DefaultTextStyle(
          style: const TextStyle(
            color: CB.black,
            fontWeight: FontWeight.w700,
            fontSize: 15,
          ),
          child: child,
        ),
      ),
    );
  }
}

/// The app theme.
ThemeData buildAppTheme() {
  return ThemeData(
    brightness: Brightness.dark,
    useMaterial3: true,
    fontFamily: 'sans-serif',
    scaffoldBackgroundColor: CB.black,
    colorScheme: const ColorScheme.dark(
      primary: CB.cyan,
      secondary: CB.purple,
      surface: CB.surface,
      error: CB.hotPink,
    ),
    cardTheme: CardThemeData(
      color: Colors.white.withValues(alpha: CB.glassOpacity),
      elevation: 0,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
    ),
    appBarTheme: const AppBarTheme(
      backgroundColor: Colors.transparent,
      foregroundColor: CB.textPrimary,
      elevation: 0,
      scrolledUnderElevation: 0,
      centerTitle: false,
      titleTextStyle: TextStyle(
        fontSize: 22,
        fontWeight: FontWeight.w800,
        letterSpacing: -0.5,
      ),
    ),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: Colors.white.withValues(alpha: 0.05),
      border: OutlineInputBorder(
        borderRadius: BorderRadius.circular(14),
        borderSide: BorderSide(color: Colors.white.withValues(alpha: 0.08)),
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(14),
        borderSide: BorderSide(color: Colors.white.withValues(alpha: 0.08)),
      ),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(14),
        borderSide: const BorderSide(color: CB.cyan, width: 1.5),
      ),
      hintStyle: const TextStyle(color: CB.textTertiary, fontSize: 15),
      contentPadding: const EdgeInsets.symmetric(horizontal: 18, vertical: 16),
    ),
    dialogTheme: DialogThemeData(
      backgroundColor: CB.surface,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(20)),
    ),
    snackBarTheme: SnackBarThemeData(
      backgroundColor: CB.surfaceLight,
      contentTextStyle: const TextStyle(color: CB.textPrimary),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
      behavior: SnackBarBehavior.floating,
    ),
    textTheme: const TextTheme(
      headlineLarge: TextStyle(fontSize: 28, fontWeight: FontWeight.w800, letterSpacing: -1),
      headlineMedium: TextStyle(fontSize: 22, fontWeight: FontWeight.w700, letterSpacing: -0.5),
      titleLarge: TextStyle(fontSize: 18, fontWeight: FontWeight.w700),
      titleMedium: TextStyle(fontSize: 15, fontWeight: FontWeight.w600),
      bodyLarge: TextStyle(fontSize: 15, fontWeight: FontWeight.w400, height: 1.5),
      bodyMedium: TextStyle(fontSize: 14, fontWeight: FontWeight.w400),
      bodySmall: TextStyle(fontSize: 12, fontWeight: FontWeight.w400, color: CB.textSecondary),
      labelSmall: TextStyle(fontSize: 11, fontWeight: FontWeight.w600, letterSpacing: 0.5),
    ),
  );
}
