import 'dart:math' as math;
import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter/scheduler.dart';
import 'package:provider/provider.dart';
import '../models/session.dart';
import '../services/api_service.dart';
import '../theme.dart';
import 'chat_screen.dart';

// ─── Behavior state machine ───────────────────────────────────────────────────

enum _Behavior { wander, pause, dash, zigzag }

// ─── Bot physics / personality ────────────────────────────────────────────────

class _Bot {
  final String sessionId;
  Session session;

  // 2.5D world position: x = screen X, z = depth (0=far, 1=near)
  double x, z, vz;
  bool facingRight;
  double walkPhase = 0;
  double floatPhase;

  // Jump spring
  double jumpOffset = 0;
  double jumpVel = 0;

  // Head look-around tilt (radians)
  double headTilt = 0;

  // Urgency animation phase
  double urgencyPhase = 0;

  // Behavior state machine
  _Behavior behavior = _Behavior.wander;
  double behaviorTimer = 0;

  // Permanent per-bot personality (seeded from session id)
  final double speedMult;
  final double bounceMult;
  final math.Random _rng;

  _Bot({
    required this.sessionId,
    required this.session,
    required this.x,
    required this.z,
    required double vx,
    required math.Random seed,
  })  : facingRight = vx >= 0,
        vz = 0,
        floatPhase = seed.nextDouble() * math.pi * 2,
        speedMult = 0.6 + seed.nextDouble() * 0.9,
        bounceMult = 0.4 + seed.nextDouble() * 1.6,
        _rng = math.Random(sessionId.hashCode) {
    _pickBehavior(stagger: true);
  }

  bool get isUrgent =>
      session.agentState == AgentState.waitingForInput ||
      session.agentState == AgentState.error;

  // Visual scale: far bots (z≈0) are tiny, near bots (z≈1) are large
  double get scale => 0.42 + z * 0.88;

  // Gentle floating bob
  double get floatOffset => math.sin(floatPhase) * 3;

  void _pickBehavior({bool stagger = false}) {
    final r = _rng.nextDouble();
    if (r < 0.44) {
      behavior = _Behavior.wander;
      behaviorTimer = 3.0 + _rng.nextDouble() * 8.0;
    } else if (r < 0.71) {
      behavior = _Behavior.pause;
      behaviorTimer = 1.0 + _rng.nextDouble() * 3.5;
      headTilt = (_rng.nextDouble() - 0.5) * 0.55;
    } else if (r < 0.87) {
      behavior = _Behavior.dash;
      behaviorTimer = 0.3 + _rng.nextDouble() * 0.8;
    } else {
      behavior = _Behavior.zigzag;
      behaviorTimer = 2.5 + _rng.nextDouble() * 5.0;
    }
    if (stagger) behaviorTimer *= _rng.nextDouble();
    if (behavior != _Behavior.pause) headTilt = 0;
  }

  void tick(double dt, double minX, double maxX) {
    floatPhase += dt * (1.4 + speedMult * 0.3);
    urgencyPhase += dt * 2.2;
    behaviorTimer -= dt;
    if (behaviorTimer <= 0) _pickBehavior();

    // Urgent bots drift to the front for visibility
    if (session.agentState == AgentState.waitingForInput) {
      z = (z + (0.80 - z) * dt * 0.25).clamp(0.05, 0.95);
    }

    // ── Jump physics ──────────────────────────────────────────────────────────
    if (jumpOffset > 0 || jumpVel > 0) {
      jumpVel -= 380.0 * dt * bounceMult;
      jumpOffset = math.max(0, jumpOffset + jumpVel * dt);
      if (jumpOffset <= 0) {
        jumpOffset = 0;
        jumpVel = 0;
      }
    }

    // Spontaneous jump chance varies by agent state
    if (jumpOffset == 0 && jumpVel == 0) {
      final chance = switch (session.agentState) {
        AgentState.working  => 0.30,
        AgentState.starting => 0.20,
        AgentState.idle     => 0.04,
        _                   => 0.0,
      };
      if (_rng.nextDouble() < chance * dt) {
        jumpVel = (50 + _rng.nextDouble() * 65) * bounceMult;
      }
    }

    // ── Movement speed ────────────────────────────────────────────────────────
    final baseSpeed = speedMult * switch (session.agentState) {
      AgentState.working         => 36.0,
      AgentState.idle            => 11.0,
      AgentState.waitingForInput => 0.0,
      AgentState.starting        => 22.0,
      AgentState.error           => 6.0,
      AgentState.offline         => 0.0,
    };

    final speed = switch (behavior) {
      _Behavior.pause  => 0.0,
      _Behavior.dash   => baseSpeed * 2.8,
      _Behavior.zigzag => baseSpeed * 0.65,
      _Behavior.wander => baseSpeed,
    };

    if (speed > 0) {
      x += (facingRight ? speed : -speed) * dt;
      walkPhase += dt * (speed / 5.0);
    }

    // ── Depth (Z) drift ───────────────────────────────────────────────────────
    if (behavior != _Behavior.pause &&
        session.agentState != AgentState.waitingForInput) {
      if (_rng.nextDouble() < dt * 1.5) {
        vz += (_rng.nextDouble() - 0.5) * 0.22;
        vz = vz.clamp(-0.22, 0.22);
      }
      z = (z + vz * dt).clamp(0.05, 0.95);
    }
    if (behavior == _Behavior.zigzag) {
      z = (z + math.sin(walkPhase * 0.4) * 0.22 * dt).clamp(0.05, 0.95);
    }

    // ── Wall bouncing ─────────────────────────────────────────────────────────
    if (x < minX) { x = minX; facingRight = true; }
    if (x > maxX) { x = maxX; facingRight = false; }
  }
}

// ─── Zoo Screen ───────────────────────────────────────────────────────────────

class ZooScreen extends StatefulWidget {
  const ZooScreen({super.key});

  @override
  State<ZooScreen> createState() => _ZooScreenState();
}

class _ZooScreenState extends State<ZooScreen>
    with SingleTickerProviderStateMixin {
  final _seed = math.Random(99887);
  List<_Bot> _bots = [];
  late Ticker _ticker;
  Duration? _lastTime;
  Timer? _refreshTimer;
  late List<_Star> _stars;

  static const _floorFrac = 0.58;

  @override
  void initState() {
    super.initState();
    _stars = List.generate(90, (_) => _Star(_seed));
    _ticker = createTicker(_onTick)..start();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      _refresh();
      _refreshTimer =
          Timer.periodic(const Duration(seconds: 2), (_) => _refresh());
    });
  }

  void _onTick(Duration elapsed) {
    if (_lastTime == null) { _lastTime = elapsed; return; }
    final dt = (elapsed - _lastTime!).inMicroseconds / 1e6;
    _lastTime = elapsed;
    if (dt <= 0 || dt > 0.1) return;

    final w = (context.findRenderObject() as RenderBox?)?.size.width ?? 400.0;
    setState(() {
      for (final b in _bots) { b.tick(dt, 40.0, w - 40.0); }
    });
  }

  Future<void> _refresh() async {
    try {
      final sessions = await context.read<ApiService>().listSessions();
      if (!mounted) return;
      setState(() => _syncBots(sessions));
    } catch (_) {}
  }

  void _syncBots(List<Session> sessions) {
    final byId = {for (final b in _bots) b.sessionId: b};
    final w = (context.findRenderObject() as RenderBox?)?.size.width ?? 400.0;
    _bots = sessions.map((s) {
      final e = byId[s.id];
      if (e != null) { e.session = s; return e; }
      final sp = 15.0 + _seed.nextDouble() * 30.0;
      return _Bot(
        sessionId: s.id,
        session: s,
        x: w * (0.1 + _seed.nextDouble() * 0.8),
        z: 0.2 + _seed.nextDouble() * 0.6,
        vx: (_seed.nextBool() ? 1 : -1) * sp,
        seed: _seed,
      );
    }).toList();
  }

  @override
  void dispose() {
    _ticker.dispose();
    _refreshTimer?.cancel();
    super.dispose();
  }

  Map<AgentState, int> get _stateCounts {
    final counts = <AgentState, int>{};
    for (final b in _bots) {
      counts[b.session.agentState] =
          (counts[b.session.agentState] ?? 0) + 1;
    }
    return counts;
  }

  List<_Bot> get _urgentBots => _bots
      .where((b) =>
          b.session.agentState == AgentState.waitingForInput ||
          b.session.agentState == AgentState.error)
      .toList();

  @override
  Widget build(BuildContext context) {
    final size = MediaQuery.of(context).size;
    final floorY = size.height * _floorFrac;
    final groundH = size.height - floorY;

    // Render back→front so near bots overlap far ones correctly
    final sorted = [..._bots]..sort((a, b) => a.z.compareTo(b.z));
    final urgent = _urgentBots;

    return Scaffold(
      backgroundColor: CB.black,
      body: Stack(
        children: [
          // ── Background (static) ──────────────────────────────────────────
          CustomPaint(
            size: size,
            painter: _ZooBgPainter(stars: _stars, floorFrac: _floorFrac),
          ),

          // ── Spotlight overlay for urgent bots ────────────────────────────
          CustomPaint(
            size: size,
            painter: _SpotlightPainter(
              bots: _bots,
              floorY: floorY,
              groundH: groundH,
            ),
          ),

          // ── Header + status pills ────────────────────────────────────────
          SafeArea(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Padding(
                  padding: const EdgeInsets.fromLTRB(8, 12, 12, 0),
                  child: Row(
                    children: [
                      IconButton(
                        icon: const Icon(Icons.arrow_back_ios_new_rounded,
                            size: 18),
                        color: CB.textSecondary,
                        onPressed: () => Navigator.pop(context),
                      ),
                      const SizedBox(width: 4),
                      Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          const GradientText(
                            'AGENT ZOO',
                            style: TextStyle(
                                fontSize: 20,
                                fontWeight: FontWeight.w900,
                                letterSpacing: 3),
                          ),
                          Text(
                            _bots.isEmpty
                                ? 'habitat empty'
                                : '${_bots.length} agent${_bots.length == 1 ? '' : 's'} roaming',
                            style: const TextStyle(
                                color: CB.textSecondary,
                                fontSize: 10,
                                letterSpacing: 1.2),
                          ),
                        ],
                      ),
                      const Spacer(),
                      _StatusPills(counts: _stateCounts),
                    ],
                  ),
                ),
                // Urgent banner (animated) — only when something needs action
                if (urgent.isNotEmpty)
                  Padding(
                    padding: const EdgeInsets.fromLTRB(16, 8, 16, 0),
                    child: _UrgentBanner(count: urgent.length),
                  ),
              ],
            ),
          ),

          // ── Empty state ──────────────────────────────────────────────────
          if (_bots.isEmpty)
            Center(
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: const [
                  Text('🤖', style: TextStyle(fontSize: 52)),
                  SizedBox(height: 16),
                  Text('No agents in the zoo yet',
                      style:
                          TextStyle(color: CB.textSecondary, fontSize: 14)),
                  SizedBox(height: 4),
                  Text('Start a session to populate your habitat!',
                      style:
                          TextStyle(color: CB.textTertiary, fontSize: 12)),
                ],
              ),
            ),

          // ── Robots — back to front ───────────────────────────────────────
          for (final bot in sorted) _positionedBot(bot, floorY, groundH),

          // ── Bottom: urgent tray OR legend ────────────────────────────────
          if (urgent.isNotEmpty)
            Positioned(
              bottom: 0,
              left: 0,
              right: 0,
              child: _UrgentTray(bots: urgent, onTap: _openChat),
            )
          else
            Positioned(
              bottom: 20,
              left: 0,
              right: 0,
              child: Wrap(
                alignment: WrapAlignment.center,
                spacing: 20,
                runSpacing: 6,
                children: const [
                  _LegendItem(color: CB.cyan, label: 'Copilot'),
                  _LegendItem(color: CB.purple, label: 'Claude'),
                  _LegendItem(color: CB.neonGreen, label: 'Working'),
                  _LegendItem(color: CB.amber, label: 'Waiting'),
                  _LegendItem(color: CB.hotPink, label: 'Error'),
                ],
              ),
            ),
        ],
      ),
    );
  }

  Widget _positionedBot(_Bot bot, double floorY, double groundH) {
    final s = bot.scale;
    final feetY = floorY + bot.z * groundH * 0.80;
    final top = feetY - 112.0 * s + bot.floatOffset * s - bot.jumpOffset * s;
    final left = bot.x - 40.0 * s;
    final depthFade = 0.40 + bot.z * 0.60;

    return Positioned(
      left: left,
      top: top,
      child: GestureDetector(
        onTap: () => _openChat(bot.session),
        child: _RobotWidget(
          key: ValueKey(bot.sessionId),
          bot: bot,
          scale: s,
          depthFade: depthFade,
        ),
      ),
    );
  }

  void _openChat(Session session) {
    Navigator.push(
      context,
      PageRouteBuilder(
        pageBuilder: (context, animation, secondaryAnimation) => ChatScreen(session: session),
        transitionsBuilder: (context, anim, secondaryAnimation, child) => FadeTransition(
          opacity: anim.drive(CurveTween(curve: Curves.easeInOut)),
          child: child,
        ),
        transitionDuration: const Duration(milliseconds: 350),
      ),
    );
  }
}

// ─── Spotlight painter ────────────────────────────────────────────────────────

class _SpotlightPainter extends CustomPainter {
  final List<_Bot> bots;
  final double floorY;
  final double groundH;

  const _SpotlightPainter(
      {required this.bots, required this.floorY, required this.groundH});

  @override
  void paint(Canvas canvas, Size size) {
    for (final bot in bots) {
      if (bot.session.agentState == AgentState.waitingForInput) {
        _drawWaitingSpotlight(canvas, bot);
      } else if (bot.session.agentState == AgentState.error) {
        _drawErrorAura(canvas, bot);
      }
    }
  }

  void _drawWaitingSpotlight(Canvas canvas, _Bot bot) {
    final s = bot.scale;
    final feetY = floorY + bot.z * groundH * 0.80;
    final bx = bot.x;
    final pulse = (math.sin(bot.urgencyPhase) * 0.5 + 0.5); // 0..1

    // Beam cone from sky down to feet
    final beamW = 28.0 * s;
    final topY = floorY * 0.25;
    final path = Path()
      ..moveTo(bx, topY)
      ..lineTo(bx - beamW, feetY)
      ..lineTo(bx + beamW, feetY)
      ..close();

    canvas.drawPath(
      path,
      Paint()
        ..shader = LinearGradient(
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
          colors: [
            Colors.transparent,
            CB.amber.withValues(alpha: 0.04 + pulse * 0.05),
          ],
        ).createShader(
            Rect.fromLTWH(bx - beamW, topY, beamW * 2, feetY - topY)),
    );

    // Glowing floor pool
    canvas.drawOval(
      Rect.fromCenter(
          center: Offset(bx, feetY + 3),
          width: beamW * 2.8,
          height: beamW * 0.65),
      Paint()
        ..color = CB.amber.withValues(alpha: 0.07 + pulse * 0.06)
        ..maskFilter = const MaskFilter.blur(BlurStyle.normal, 10),
    );
  }

  void _drawErrorAura(Canvas canvas, _Bot bot) {
    final s = bot.scale;
    final feetY = floorY + bot.z * groundH * 0.80;
    final bx = bot.x;
    final pulse = (math.sin(bot.urgencyPhase * 1.6) * 0.5 + 0.5);

    // Red ground glow
    canvas.drawOval(
      Rect.fromCenter(
          center: Offset(bx, feetY + 3),
          width: 50.0 * s,
          height: 16.0 * s),
      Paint()
        ..color = CB.hotPink.withValues(alpha: 0.08 + pulse * 0.07)
        ..maskFilter = const MaskFilter.blur(BlurStyle.normal, 12),
    );
  }

  @override
  bool shouldRepaint(_SpotlightPainter old) => true;
}

// ─── Stars ────────────────────────────────────────────────────────────────────

class _Star {
  final double x, y, size;
  _Star(math.Random r)
      : x = r.nextDouble(),
        y = r.nextDouble() * 0.60,
        size = 0.5 + r.nextDouble() * 1.5;
}

// ─── Background painter ───────────────────────────────────────────────────────

class _ZooBgPainter extends CustomPainter {
  final List<_Star> stars;
  final double floorFrac;

  const _ZooBgPainter({required this.stars, required this.floorFrac});

  @override
  void paint(Canvas canvas, Size size) {
    final floorY = size.height * floorFrac;

    // Sky gradient
    canvas.drawRect(
      Rect.fromLTWH(0, 0, size.width, floorY),
      Paint()
        ..shader = const LinearGradient(
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
          colors: [Color(0xFF010108), Color(0xFF070418), Color(0xFF0E0A26)],
          stops: [0.0, 0.55, 1.0],
        ).createShader(Rect.fromLTWH(0, 0, size.width, floorY)),
    );

    // Ground
    canvas.drawRect(
      Rect.fromLTWH(0, floorY, size.width, size.height - floorY),
      Paint()
        ..shader = const LinearGradient(
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
          colors: [Color(0xFF1A1035), Color(0xFF0A0818)],
        ).createShader(
            Rect.fromLTWH(0, floorY, size.width, size.height - floorY)),
    );

    // Perspective grid
    _drawPerspectiveGrid(canvas, size, floorY);

    // Horizon glow line
    canvas.drawLine(
      Offset(0, floorY),
      Offset(size.width, floorY),
      Paint()
        ..shader = LinearGradient(
          colors: [
            Colors.transparent,
            CB.cyan.withValues(alpha: 0.75),
            CB.purple.withValues(alpha: 0.75),
            Colors.transparent,
          ],
          stops: const [0.0, 0.3, 0.7, 1.0],
        ).createShader(Rect.fromLTWH(0, floorY - 1, size.width, 2))
        ..strokeWidth = 1.5
        ..style = PaintingStyle.stroke,
    );

    // Stars
    for (final s in stars) {
      canvas.drawCircle(
        Offset(s.x * size.width, s.y * size.height),
        s.size,
        Paint()..color = Colors.white.withValues(alpha: 0.25 + s.size * 0.14),
      );
    }

    // Nebula blobs
    _drawNebula(canvas, size.width * 0.15, size.height * 0.20, 120, 0xFF5500FF, 0.09);
    _drawNebula(canvas, size.width * 0.80, size.height * 0.12, 90, 0xFF0066FF, 0.07);
    _drawNebula(canvas, size.width * 0.55, size.height * 0.38, 65, 0xFFAA00FF, 0.04);
  }

  void _drawNebula(Canvas canvas, double cx, double cy, double r,
      int colorHex, double alpha) {
    canvas.drawCircle(
      Offset(cx, cy),
      r,
      Paint()
        ..shader = RadialGradient(
          colors: [Color(colorHex).withValues(alpha: alpha), Colors.transparent],
        ).createShader(Rect.fromCircle(center: Offset(cx, cy), radius: r)),
    );
  }

  void _drawPerspectiveGrid(Canvas canvas, Size size, double floorY) {
    final vp = Offset(size.width / 2, floorY);
    final bottom = size.height;
    final gridStroke = Paint()..strokeWidth = 0.5;

    const numRadial = 18;
    for (var i = 0; i <= numRadial; i++) {
      final t = i / numRadial;
      final endX = size.width * t;
      final alpha = 0.012 + math.sin(t * math.pi) * 0.038;
      canvas.drawLine(
        vp,
        Offset(endX, bottom),
        gridStroke
          ..color = CB.cyan.withValues(alpha: alpha.clamp(0.0, 1.0)),
      );
    }

    const numH = 9;
    for (var i = 1; i <= numH; i++) {
      final t = i / numH;
      final y = floorY + t * t * (bottom - floorY);
      final alpha = (0.015 + (1 - t) * 0.065).clamp(0.0, 1.0);
      canvas.drawLine(
        Offset(0, y),
        Offset(size.width, y),
        gridStroke..color = CB.purple.withValues(alpha: alpha),
      );
    }
  }

  @override
  bool shouldRepaint(_ZooBgPainter old) => false;
}

// ─── Status pills ─────────────────────────────────────────────────────────────

class _StatusPills extends StatelessWidget {
  final Map<AgentState, int> counts;
  const _StatusPills({required this.counts});

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        if ((counts[AgentState.working] ?? 0) > 0)
          _Pill(color: CB.neonGreen, label: '${counts[AgentState.working]}', icon: Icons.settings_outlined),
        if ((counts[AgentState.waitingForInput] ?? 0) > 0)
          _Pill(color: CB.amber, label: '${counts[AgentState.waitingForInput]}', icon: Icons.hourglass_top_rounded),
        if ((counts[AgentState.error] ?? 0) > 0)
          _Pill(color: CB.hotPink, label: '${counts[AgentState.error]}', icon: Icons.error_outline_rounded),
        if ((counts[AgentState.idle] ?? 0) > 0)
          _Pill(color: CB.textSecondary, label: '${counts[AgentState.idle]}', icon: Icons.radio_button_unchecked_rounded),
      ],
    );
  }
}

class _Pill extends StatelessWidget {
  final Color color;
  final String label;
  final IconData icon;

  const _Pill({required this.color, required this.label, required this.icon});

  @override
  Widget build(BuildContext context) {
    return Container(
      margin: const EdgeInsets.only(left: 5),
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.11),
        borderRadius: BorderRadius.circular(20),
        border: Border.all(color: color.withValues(alpha: 0.28), width: 1),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(icon, size: 10, color: color.withValues(alpha: 0.9)),
          const SizedBox(width: 4),
          Text(
            label,
            style: TextStyle(
              color: color,
              fontSize: 11,
              fontWeight: FontWeight.w800,
              letterSpacing: 0.3,
            ),
          ),
        ],
      ),
    );
  }
}

// ─── Urgent banner ────────────────────────────────────────────────────────────

class _UrgentBanner extends StatefulWidget {
  final int count;
  const _UrgentBanner({required this.count});

  @override
  State<_UrgentBanner> createState() => _UrgentBannerState();
}

class _UrgentBannerState extends State<_UrgentBanner>
    with SingleTickerProviderStateMixin {
  late AnimationController _ctrl;

  @override
  void initState() {
    super.initState();
    _ctrl = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 950),
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
      builder: (context, child) => Container(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 7),
        decoration: BoxDecoration(
          color: CB.amber.withValues(alpha: 0.07 + _ctrl.value * 0.07),
          borderRadius: BorderRadius.circular(10),
          border: Border.all(
            color: CB.amber.withValues(alpha: 0.30 + _ctrl.value * 0.28),
            width: 1,
          ),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              Icons.warning_amber_rounded,
              size: 13,
              color: CB.amber.withValues(alpha: 0.75 + _ctrl.value * 0.25),
            ),
            const SizedBox(width: 7),
            Text(
              '${widget.count} agent${widget.count == 1 ? '' : 's'} '
              '${widget.count == 1 ? 'needs' : 'need'} your attention  ↓',
              style: TextStyle(
                color: CB.amber.withValues(alpha: 0.75 + _ctrl.value * 0.25),
                fontSize: 11,
                fontWeight: FontWeight.w700,
                letterSpacing: 0.5,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ─── Urgent tray ──────────────────────────────────────────────────────────────

class _UrgentTray extends StatelessWidget {
  final List<_Bot> bots;
  final void Function(Session) onTap;
  const _UrgentTray({required this.bots, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: BoxDecoration(
        gradient: LinearGradient(
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
          colors: [
            Colors.transparent,
            CB.black.withValues(alpha: 0.96),
          ],
          stops: const [0.0, 0.35],
        ),
      ),
      padding: const EdgeInsets.fromLTRB(16, 28, 16, 28),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Padding(
            padding: EdgeInsets.only(bottom: 10),
            child: Text(
              'NEEDS ATTENTION',
              style: TextStyle(
                color: CB.amber,
                fontSize: 9,
                fontWeight: FontWeight.w900,
                letterSpacing: 2.5,
              ),
            ),
          ),
          SizedBox(
            height: 80,
            child: ListView.builder(
              scrollDirection: Axis.horizontal,
              itemCount: bots.length,
              itemBuilder: (context, i) => _UrgentCard(
                bot: bots[i],
                onTap: () => onTap(bots[i].session),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _UrgentCard extends StatefulWidget {
  final _Bot bot;
  final VoidCallback onTap;
  const _UrgentCard({required this.bot, required this.onTap});

  @override
  State<_UrgentCard> createState() => _UrgentCardState();
}

class _UrgentCardState extends State<_UrgentCard>
    with SingleTickerProviderStateMixin {
  late AnimationController _ctrl;

  @override
  void initState() {
    super.initState();
    _ctrl = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 1100),
    )..repeat(reverse: true);
  }

  @override
  void dispose() {
    _ctrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final session = widget.bot.session;
    final isError = session.agentState == AgentState.error;
    final color = isError ? CB.hotPink : CB.amber;
    final name = _botName(session);
    final stateLabel = isError ? '⚠ ERROR' : '⏳ WAITING';
    final detail = isError
        ? (session.lastMessage.isNotEmpty
            ? session.lastMessage
            : 'Error occurred')
        : (session.currentTool.isNotEmpty
            ? session.currentTool
            : session.currentPrompt.isNotEmpty
                ? session.currentPrompt
                : 'Awaiting your response');
    final dir = () {
      final parts = session.workingDir.split('/');
      return parts.lastWhere((p) => p.isNotEmpty, orElse: () => '');
    }();

    return AnimatedBuilder(
      animation: _ctrl,
      builder: (context, child) => GestureDetector(
        onTap: widget.onTap,
        child: Container(
          width: 168,
          margin: const EdgeInsets.only(right: 10),
          padding: const EdgeInsets.all(12),
          decoration: BoxDecoration(
            color: color.withValues(alpha: 0.07 + _ctrl.value * 0.04),
            borderRadius: BorderRadius.circular(14),
            border: Border.all(
              color: color.withValues(alpha: 0.28 + _ctrl.value * 0.22),
              width: 1,
            ),
            boxShadow: [
              BoxShadow(
                color: color.withValues(alpha: 0.06 + _ctrl.value * 0.06),
                blurRadius: 12,
                spreadRadius: 0,
              ),
            ],
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              Row(
                children: [
                  Container(
                    width: 6,
                    height: 6,
                    decoration: BoxDecoration(
                      color: color.withValues(
                          alpha: 0.6 + _ctrl.value * 0.4),
                      shape: BoxShape.circle,
                      boxShadow: [
                        BoxShadow(
                          color: color.withValues(alpha: 0.5),
                          blurRadius: 6,
                        )
                      ],
                    ),
                  ),
                  const SizedBox(width: 6),
                  Expanded(
                    child: Text(
                      name,
                      style: TextStyle(
                        color: color,
                        fontSize: 12,
                        fontWeight: FontWeight.w800,
                        letterSpacing: 0.2,
                      ),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                    ),
                  ),
                  Icon(
                    Icons.arrow_forward_ios_rounded,
                    size: 9,
                    color: color.withValues(alpha: 0.55),
                  ),
                ],
              ),
              if (dir.isNotEmpty && dir != name)
                Text(
                  dir,
                  style: const TextStyle(
                    color: CB.textTertiary,
                    fontSize: 9,
                    letterSpacing: 0.3,
                  ),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
              Row(
                children: [
                  Container(
                    padding:
                        const EdgeInsets.symmetric(horizontal: 5, vertical: 2),
                    decoration: BoxDecoration(
                      color: color.withValues(alpha: 0.14),
                      borderRadius: BorderRadius.circular(4),
                    ),
                    child: Text(
                      stateLabel,
                      style: TextStyle(
                        color: color,
                        fontSize: 8,
                        fontWeight: FontWeight.w900,
                        letterSpacing: 1,
                      ),
                    ),
                  ),
                  const SizedBox(width: 6),
                  Expanded(
                    child: Text(
                      detail,
                      style: const TextStyle(
                        color: CB.textSecondary,
                        fontSize: 9,
                      ),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                    ),
                  ),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }
}

// ─── Robot widget ─────────────────────────────────────────────────────────────

class _RobotWidget extends StatefulWidget {
  final _Bot bot;
  final double scale;
  final double depthFade;

  const _RobotWidget({
    super.key,
    required this.bot,
    required this.scale,
    required this.depthFade,
  });

  @override
  State<_RobotWidget> createState() => _RobotWidgetState();
}

class _RobotWidgetState extends State<_RobotWidget>
    with TickerProviderStateMixin {
  late AnimationController _blinkCtrl;
  late AnimationController _antCtrl;
  late AnimationController _glowCtrl;
  late AnimationController _ringCtrl;

  @override
  void initState() {
    super.initState();
    _blinkCtrl = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 180),
    );
    final antMs = 650 + (widget.bot.sessionId.hashCode.abs() % 500);
    _antCtrl =
        AnimationController(vsync: this, duration: Duration(milliseconds: antMs))
          ..repeat(reverse: true);
    _glowCtrl = AnimationController(
        vsync: this, duration: const Duration(milliseconds: 1400))
      ..repeat(reverse: true);
    _ringCtrl = AnimationController(
        vsync: this, duration: const Duration(milliseconds: 1600))
      ..repeat();
    _scheduleBlink();
  }

  void _scheduleBlink() {
    final delay = 1500 + math.Random().nextInt(3000);
    Future.delayed(Duration(milliseconds: delay), () {
      if (!mounted) return;
      _blinkCtrl.forward().then((_) {
        if (!mounted) return;
        _blinkCtrl.reverse().then((_) => _scheduleBlink());
      });
    });
  }

  @override
  void dispose() {
    _blinkCtrl.dispose();
    _antCtrl.dispose();
    _glowCtrl.dispose();
    _ringCtrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final bot = widget.bot;
    final s = widget.scale;
    final fade = widget.depthFade;
    final color = bot.session.backend == 'claude' ? CB.purple : CB.cyan;
    final w = 80.0 * s;
    final canvasH = 78.0 * s;

    return AnimatedBuilder(
      animation:
          Listenable.merge([_blinkCtrl, _antCtrl, _glowCtrl, _ringCtrl]),
      builder: (context, child) {
        final robotContent = SizedBox(
          width: w,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              // Thought bubble
              _ThoughtBubble(session: bot.session, scale: s, fade: fade),
              SizedBox(height: 3 * s),
              // Robot body — flipped horizontally when facing left
              Transform(
                alignment: Alignment.center,
                transform: Matrix4.diagonal3Values(
                    bot.facingRight ? 1.0 : -1.0, 1.0, 1.0),
                child: CustomPaint(
                  size: Size(w, canvasH),
                  painter: _RobotPainter(
                    walkPhase: bot.walkPhase,
                    state: bot.session.agentState,
                    baseColor: color,
                    blinkValue: _blinkCtrl.value,
                    antValue: _antCtrl.value,
                    glowValue: _glowCtrl.value,
                    headTilt: bot.headTilt,
                    behavior: bot.behavior,
                    depthFade: fade,
                  ),
                ),
              ),
              SizedBox(height: 2 * s),
              // Name tag
              Container(
                constraints: BoxConstraints(maxWidth: w),
                padding: EdgeInsets.symmetric(
                    horizontal: 6 * s, vertical: 2 * s),
                decoration: BoxDecoration(
                  color: color.withValues(alpha: 0.12 * fade),
                  borderRadius: BorderRadius.circular(4 * s),
                  border: Border.all(
                      color: color.withValues(alpha: 0.30 * fade),
                      width: 0.5),
                ),
                child: Text(
                  _botName(bot.session),
                  style: TextStyle(
                    color: color.withValues(alpha: 0.85 * fade),
                    fontSize: 8 * s,
                    letterSpacing: 0.5,
                    fontFamily: 'monospace',
                  ),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  textAlign: TextAlign.center,
                ),
              ),
            ],
          ),
        );

        if (!bot.isUrgent) return robotContent;

        // Urgency ring beneath the feet for urgent bots
        final ringColor = bot.session.agentState == AgentState.error
            ? CB.hotPink
            : CB.amber;

        return Stack(
          clipBehavior: Clip.none,
          alignment: Alignment.bottomCenter,
          children: [
            Positioned(
              bottom: 0,
              left: 0,
              right: 0,
              child: Center(
                child: CustomPaint(
                  size: Size(w * 1.3, 28 * s),
                  painter: _UrgencyRingPainter(
                    color: ringColor,
                    phase: _ringCtrl.value,
                    depthFade: fade,
                  ),
                ),
              ),
            ),
            robotContent,
          ],
        );
      },
    );
  }
}

// ─── Urgency ring painter ─────────────────────────────────────────────────────

class _UrgencyRingPainter extends CustomPainter {
  final Color color;
  final double phase;
  final double depthFade;

  const _UrgencyRingPainter(
      {required this.color, required this.phase, required this.depthFade});

  @override
  void paint(Canvas canvas, Size size) {
    final cx = size.width / 2;
    final cy = size.height * 0.55;

    // Three staggered expanding elliptical rings
    for (var i = 0; i < 3; i++) {
      final p = (phase + i / 3.0) % 1.0;
      final rx = (size.width * 0.25) + p * (size.width * 0.25);
      final ry = rx * 0.30;
      final alpha = ((1.0 - p) * 0.55 * depthFade).clamp(0.0, 1.0);
      canvas.drawOval(
        Rect.fromCenter(center: Offset(cx, cy), width: rx * 2, height: ry * 2),
        Paint()
          ..color = color.withValues(alpha: alpha)
          ..style = PaintingStyle.stroke
          ..strokeWidth = 1.2,
      );
    }
  }

  @override
  bool shouldRepaint(_UrgencyRingPainter old) => old.phase != phase;
}

String _botName(Session s) {
  if (s.title.isNotEmpty) return s.title;
  final parts = s.workingDir.split('/');
  final dir = parts.lastWhere((p) => p.isNotEmpty, orElse: () => '');
  return dir.isEmpty ? s.id.substring(0, 6) : dir;
}

// ─── Thought bubble ───────────────────────────────────────────────────────────

class _ThoughtBubble extends StatelessWidget {
  final Session session;
  final double scale;
  final double fade;

  const _ThoughtBubble({
    required this.session,
    required this.scale,
    required this.fade,
  });

  @override
  Widget build(BuildContext context) {
    final isUrgent = session.agentState == AgentState.waitingForInput ||
        session.agentState == AgentState.error;
    final thought = _thoughtFor(session);
    final color = _bubbleColor(session.agentState);
    final s = scale;

    return Container(
      constraints: BoxConstraints(maxWidth: 120 * s, minWidth: 50 * s),
      padding: EdgeInsets.symmetric(horizontal: 8 * s, vertical: isUrgent ? 6 * s : 5 * s),
      decoration: BoxDecoration(
        color: color.withValues(alpha: (isUrgent ? 0.16 : 0.12) * fade),
        borderRadius: BorderRadius.circular(10 * s),
        border: Border.all(
          color: color.withValues(alpha: (isUrgent ? 0.55 : 0.35) * fade),
          width: isUrgent ? 1.2 : 1,
        ),
        boxShadow: [
          BoxShadow(
            color: color.withValues(alpha: (isUrgent ? 0.28 : 0.18) * fade),
            blurRadius: (isUrgent ? 12 : 8) * s,
            spreadRadius: 1,
          ),
        ],
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            thought,
            style: TextStyle(
              fontSize: 9 * s,
              color: color.withValues(alpha: 0.9 * fade),
              height: 1.3,
            ),
            textAlign: TextAlign.center,
            maxLines: 2,
            overflow: TextOverflow.ellipsis,
          ),
          if (session.agentState == AgentState.waitingForInput) ...[
            SizedBox(height: 3 * s),
            Container(
              padding: EdgeInsets.symmetric(horizontal: 5 * s, vertical: 1.5 * s),
              decoration: BoxDecoration(
                color: color.withValues(alpha: 0.20 * fade),
                borderRadius: BorderRadius.circular(3 * s),
              ),
              child: Text(
                'TAP TO RESPOND',
                style: TextStyle(
                  fontSize: 7 * s,
                  color: color.withValues(alpha: 0.95 * fade),
                  fontWeight: FontWeight.w800,
                  letterSpacing: 0.8,
                ),
              ),
            ),
          ],
        ],
      ),
    );
  }

  Color _bubbleColor(AgentState state) => switch (state) {
        AgentState.working         => CB.neonGreen,
        AgentState.waitingForInput => CB.amber,
        AgentState.error           => CB.hotPink,
        AgentState.starting        => CB.cyan,
        _                          => CB.textSecondary,
      };
}

String _thoughtFor(Session s) => switch (s.agentState) {
      AgentState.working => s.currentTool.isNotEmpty
          ? '🔧 ${_trunc(s.currentTool, 22)}'
          : s.currentPrompt.isNotEmpty
              ? '💭 ${_trunc(s.currentPrompt, 22)}'
              : '⚙️ Working hard...',
      AgentState.waitingForInput => s.currentTool.isNotEmpty
          ? '🙏 ${_trunc(s.currentTool, 18)}'
          : '🙏 Need your OK!',
      AgentState.idle => const [
          '☕ Coffee break!',
          '🌟 Ready for action',
          '🎵 Bip bop boop~',
          '👾 Beep boop beep',
          '🧩 Idle thoughts...',
          '🤖 Standing by',
          '🌙 Daydreaming...',
          '🎲 Random.next()',
          '🍕 If only I ate...',
          '💡 Eureka? Nope.',
          '🔋 85% charged',
          '🎮 Want to play',
        ][s.id.hashCode.abs() % 12],
      AgentState.starting => '🚀 Booting up!',
      AgentState.error    => s.lastMessage.isNotEmpty
          ? '💥 ${_trunc(s.lastMessage, 20)}'
          : '💥 I broke!',
      AgentState.offline  => '💤 Zzz...',
    };

String _trunc(String s, int max) =>
    s.length > max ? '${s.substring(0, max)}…' : s;

// ─── Legend item ──────────────────────────────────────────────────────────────

class _LegendItem extends StatelessWidget {
  final Color color;
  final String label;

  const _LegendItem({required this.color, required this.label});

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Container(
          width: 7,
          height: 7,
          decoration: BoxDecoration(
            color: color,
            shape: BoxShape.circle,
            boxShadow: [
              BoxShadow(
                  color: color.withValues(alpha: 0.5), blurRadius: 6)
            ],
          ),
        ),
        const SizedBox(width: 5),
        Text(label,
            style: const TextStyle(color: CB.textSecondary, fontSize: 10)),
      ],
    );
  }
}

// ─── Robot painter ────────────────────────────────────────────────────────────

class _RobotPainter extends CustomPainter {
  final double walkPhase, blinkValue, antValue, glowValue, headTilt, depthFade;
  final AgentState state;
  final Color baseColor;
  final _Behavior behavior;

  const _RobotPainter({
    required this.walkPhase,
    required this.state,
    required this.baseColor,
    required this.blinkValue,
    required this.antValue,
    required this.glowValue,
    required this.headTilt,
    required this.behavior,
    required this.depthFade,
  });

  Color get _accent => switch (state) {
        AgentState.working         => CB.neonGreen,
        AgentState.waitingForInput => CB.amber,
        AgentState.error           => CB.hotPink,
        _                          => baseColor,
      };

  double _a(double alpha) => (alpha * depthFade).clamp(0.0, 1.0);

  @override
  void paint(Canvas canvas, Size size) {
    canvas.save();
    canvas.scale(size.width / 80.0, size.height / 78.0);
    _paint(canvas);
    canvas.restore();
  }

  void _paint(Canvas canvas) {
    const cx = 40.0;
    _drawShadow(canvas, cx);
    _drawLegs(canvas, cx);
    _drawBody(canvas, cx);
    _drawArms(canvas, cx);
    _drawHead(canvas, cx);
    _drawAntenna(canvas, cx);
  }

  void _drawShadow(Canvas canvas, double cx) {
    canvas.drawOval(
      Rect.fromCenter(center: Offset(cx, 75.5), width: 28, height: 6),
      Paint()
        ..color = baseColor.withValues(alpha: _a(0.12 + glowValue * 0.06))
        ..maskFilter = const MaskFilter.blur(BlurStyle.normal, 5),
    );
  }

  void _drawLegs(Canvas canvas, double cx) {
    final swing = behavior == _Behavior.dash ? 8.0 : 5.0;
    final lSwing = math.sin(walkPhase) * swing;
    final rSwing = -math.sin(walkPhase) * swing;

    final legPaint = Paint()
      ..color = baseColor.withValues(alpha: _a(0.85))
      ..strokeWidth = 5
      ..strokeCap = StrokeCap.round
      ..style = PaintingStyle.stroke;

    canvas.drawLine(Offset(cx - 7, 56), Offset(cx - 7 + lSwing, 72), legPaint);
    canvas.drawLine(Offset(cx + 7, 56), Offset(cx + 7 + rSwing, 72), legPaint);

    final footPaint = Paint()
      ..color = baseColor.withValues(alpha: _a(0.70))
      ..strokeWidth = 3.5
      ..strokeCap = StrokeCap.round
      ..style = PaintingStyle.stroke;

    canvas.drawLine(
        Offset(cx - 11 + lSwing, 73), Offset(cx - 3 + lSwing, 73), footPaint);
    canvas.drawLine(
        Offset(cx + 3 + rSwing, 73), Offset(cx + 11 + rSwing, 73), footPaint);
  }

  void _drawBody(Canvas canvas, double cx) {
    final lean = behavior == _Behavior.dash ? 2.5 : 0.0;
    final bodyRect = RRect.fromRectAndRadius(
      Rect.fromCenter(
          center: Offset(cx + lean * 0.5, 46), width: 26, height: 20),
      const Radius.circular(5),
    );
    canvas.drawRRect(
      bodyRect,
      Paint()
        ..shader = LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [
            baseColor.withValues(alpha: _a(0.24 + glowValue * 0.06)),
            baseColor.withValues(alpha: _a(0.07)),
          ],
        ).createShader(bodyRect.outerRect),
    );
    canvas.drawRRect(
      bodyRect,
      Paint()
        ..color =
            baseColor.withValues(alpha: _a(0.55 + glowValue * 0.25))
        ..style = PaintingStyle.stroke
        ..strokeWidth = 1.5,
    );
    _drawChest(canvas, Offset(cx + lean * 0.5, 46));
  }

  void _drawChest(Canvas canvas, Offset c) {
    canvas.drawRRect(
      RRect.fromRectAndRadius(
          Rect.fromCenter(center: c, width: 12, height: 10),
          const Radius.circular(2)),
      Paint()..color = _accent.withValues(alpha: _a(0.20)),
    );
    final ip = Paint()
      ..color = _accent.withValues(alpha: _a(1.0))
      ..strokeWidth = 1.5
      ..strokeCap = StrokeCap.round
      ..style = PaintingStyle.stroke;

    switch (state) {
      case AgentState.working:
        for (var i = 0; i < 4; i++) {
          final a = walkPhase * 2 + i * math.pi / 2;
          canvas.drawCircle(
            c + Offset(math.cos(a) * 3.5, math.sin(a) * 3.5),
            1.0,
            Paint()..color = _accent.withValues(alpha: _a(1.0)),
          );
        }
        break;
      case AgentState.waitingForInput:
        canvas.drawLine(c.translate(0, -3.5), c.translate(0, 0.5), ip);
        canvas.drawCircle(c.translate(0, 2.5), 0.9,
            Paint()..color = _accent.withValues(alpha: _a(1.0)));
        break;
      case AgentState.error:
        canvas.drawLine(c.translate(-3, -3), c.translate(3, 3), ip);
        canvas.drawLine(c.translate(3, -3), c.translate(-3, 3), ip);
        break;
      case AgentState.idle:
        final path = Path()
          ..moveTo(c.dx, c.dy + 3)
          ..cubicTo(c.dx - 5, c.dy - 2, c.dx - 5, c.dy - 5, c.dx, c.dy - 2)
          ..cubicTo(c.dx + 5, c.dy - 5, c.dx + 5, c.dy - 2, c.dx, c.dy + 3);
        canvas.drawPath(path, ip);
        break;
      default:
        canvas.drawCircle(
            c, 2, Paint()..color = _accent.withValues(alpha: _a(0.4)));
    }
  }

  void _drawArms(Canvas canvas, double cx) {
    final swing = behavior == _Behavior.dash ? 7.0 : 4.0;
    final lSwing = math.sin(walkPhase + math.pi) * swing;
    final rSwing = math.sin(walkPhase) * swing;
    final lean = behavior == _Behavior.dash ? 2.5 : 0.0;

    final paint = Paint()
      ..color = baseColor.withValues(alpha: _a(0.80))
      ..strokeWidth = 4
      ..strokeCap = StrokeCap.round
      ..style = PaintingStyle.stroke;

    canvas.drawLine(Offset(cx - 13, 42),
        Offset(cx - 22 + lean * 0.5, 49 + lSwing), paint);
    canvas.drawLine(Offset(cx + 13, 42),
        Offset(cx + 22 + lean * 0.5, 49 + rSwing), paint);
  }

  void _drawHead(Canvas canvas, double cx) {
    final lean = behavior == _Behavior.dash ? 3.0 : 0.0;
    final headCX = cx + lean;

    canvas.save();
    canvas.translate(headCX, 28);
    canvas.rotate(headTilt);
    canvas.translate(-headCX, -28);

    final headRect = RRect.fromRectAndRadius(
      Rect.fromCenter(center: Offset(headCX, 28), width: 22, height: 18),
      const Radius.circular(7),
    );
    canvas.drawRRect(
      headRect,
      Paint()
        ..shader = LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [
            baseColor.withValues(alpha: _a(0.28 + glowValue * 0.06)),
            baseColor.withValues(alpha: _a(0.08)),
          ],
        ).createShader(headRect.outerRect),
    );
    canvas.drawRRect(
      headRect,
      Paint()
        ..color =
            baseColor.withValues(alpha: _a(0.65 + glowValue * 0.20))
        ..style = PaintingStyle.stroke
        ..strokeWidth = 1.5,
    );
    canvas.drawRRect(
      RRect.fromRectAndRadius(
          Rect.fromLTWH(headCX - 9, 24, 18, 7), const Radius.circular(2)),
      Paint()..color = _accent.withValues(alpha: _a(0.10)),
    );

    _drawEyes(canvas, headCX);
    canvas.restore();
  }

  void _drawEyes(Canvas canvas, double cx) {
    final eyeOpen = blinkValue < 0.5;
    final pupilShift = headTilt * 3.5;

    if (eyeOpen) {
      for (final ex in [cx - 5.0, cx + 5.0]) {
        canvas.drawCircle(
          Offset(ex, 28),
          3.5,
          Paint()
            ..color =
                _accent.withValues(alpha: _a(0.25 + glowValue * 0.20))
            ..maskFilter = const MaskFilter.blur(BlurStyle.normal, 3),
        );
      }
      canvas.drawCircle(Offset(cx - 5, 28), 2.5,
          Paint()..color = _accent.withValues(alpha: _a(1.0)));
      canvas.drawCircle(Offset(cx + 5, 28), 2.5,
          Paint()..color = _accent.withValues(alpha: _a(1.0)));
      canvas.drawCircle(Offset(cx - 4 + pupilShift, 27), 0.8,
          Paint()..color = Colors.white.withValues(alpha: _a(0.85)));
      canvas.drawCircle(Offset(cx + 6 + pupilShift, 27), 0.8,
          Paint()..color = Colors.white.withValues(alpha: _a(0.85)));
    } else {
      final cp = Paint()
        ..color = _accent.withValues(alpha: _a(0.50))
        ..strokeWidth = 2
        ..strokeCap = StrokeCap.round
        ..style = PaintingStyle.stroke;
      canvas.drawLine(Offset(cx - 7.5, 28), Offset(cx - 2.5, 28), cp);
      canvas.drawLine(Offset(cx + 2.5, 28), Offset(cx + 7.5, 28), cp);
    }
  }

  void _drawAntenna(Canvas canvas, double cx) {
    final lean = behavior == _Behavior.dash ? 3.0 : 0.0;
    final swayAmp =
        (state == AgentState.working || behavior == _Behavior.dash) ? 8.0 : 3.0;
    final sway = math.sin(antValue * math.pi) * swayAmp;
    final tipX = cx + sway + lean;
    const tipY = 11.0;

    canvas.drawLine(
      Offset(cx + lean, 19),
      Offset(tipX, tipY),
      Paint()
        ..color = baseColor.withValues(alpha: _a(0.70))
        ..strokeWidth = 1.5
        ..strokeCap = StrokeCap.round
        ..style = PaintingStyle.stroke,
    );

    canvas.drawCircle(
      Offset(tipX, tipY),
      5,
      Paint()
        ..color = _accent.withValues(alpha: _a(0.40 + glowValue * 0.40))
        ..maskFilter = const MaskFilter.blur(BlurStyle.normal, 4),
    );
    canvas.drawCircle(Offset(tipX, tipY), 2.5,
        Paint()..color = _accent.withValues(alpha: _a(1.0)));

    if (state == AgentState.working || behavior == _Behavior.dash) {
      for (var i = 0; i < 3; i++) {
        final angle = walkPhase * 3 + i * math.pi * 2 / 3;
        final dist = 5.0 + glowValue * 3;
        canvas.drawCircle(
          Offset(tipX + math.cos(angle) * dist,
              tipY + math.sin(angle) * dist),
          1.0,
          Paint()
            ..color =
                _accent.withValues(alpha: _a(0.50 + glowValue * 0.40)),
        );
      }
    }
  }

  @override
  bool shouldRepaint(_RobotPainter old) =>
      old.walkPhase != walkPhase ||
      old.blinkValue != blinkValue ||
      old.antValue != antValue ||
      old.glowValue != glowValue ||
      old.headTilt != headTilt ||
      old.behavior != behavior ||
      old.state != state;
}
