import 'dart:async';
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../models/session.dart';
import '../services/api_service.dart';
import '../theme.dart';
import 'chat_screen.dart';

class SessionsScreen extends StatefulWidget {
  const SessionsScreen({super.key});

  @override
  State<SessionsScreen> createState() => _SessionsScreenState();
}

class _SessionsScreenState extends State<SessionsScreen>
    with TickerProviderStateMixin {
  List<Session> _sessions = [];
  bool _loading = true;
  String? _missionTitle;
  String? _missionSummary;
  bool _updating = false;
  bool _releasing = false;
  String? _error;
  Timer? _pollTimer;
  late AnimationController _scanlineCtrl;

  @override
  void initState() {
    super.initState();
    _scanlineCtrl = AnimationController(
      vsync: this,
      duration: const Duration(seconds: 4),
    )..repeat();
    _refresh();
    _pollTimer = Timer.periodic(const Duration(seconds: 2), (_) => _poll());
  }

  @override
  void dispose() {
    _pollTimer?.cancel();
    _scanlineCtrl.dispose();
    super.dispose();
  }

  Future<void> _poll() async {
    try {
      final sessions = await context.read<ApiService>().listSessions();
      if (!mounted) return;
      setState(() => _sessions = sessions);
    } catch (_) {
      // silent poll failure — don't overwrite error state
    }
  }

  Future<void> _refresh() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final api = context.read<ApiService>();
      final sessions = await api.listSessions();
      Map<String, String> mission = {'title': '', 'summary': ''};
      try {
        mission = await api.getMissionSummary();
      } catch (_) {
        // mission summary is optional; ignore errors
      }
      setState(() {
        _sessions = sessions;
        _missionTitle = mission['title'];
        _missionSummary = mission['summary'];
        _loading = false;
      });
    } catch (e) {
      setState(() {
        _error = e.toString();
        _loading = false;
      });
    }
  }

  Future<void> _createSession() async {
    final result = await showModalBottomSheet<
        ({String dir, String backend, String model, bool skipPermissions})>(
      context: context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      builder: (ctx) => const _NewSessionSheet(),
    );
    if (result == null || result.dir.isEmpty) return;

    try {
      final session = await context.read<ApiService>().createSession(result.dir,
          backend: result.backend,
          model: result.model,
          skipPermissions: result.skipPermissions);
      if (!mounted) return;
      _refresh();
      _openSession(session);
    } catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text('$e')));
    }
  }

  void _openSession(Session session) {
    Navigator.of(context)
        .push(
      PageRouteBuilder(
        pageBuilder: (_, __, ___) => ChatScreen(session: session),
        transitionsBuilder: (_, anim, __, child) {
          return FadeTransition(
            opacity: anim,
            child: SlideTransition(
              position: Tween(
                      begin: const Offset(0.03, 0), end: Offset.zero)
                  .animate(
                CurvedAnimation(parent: anim, curve: Curves.easeOutCubic),
              ),
              child: child,
            ),
          );
        },
        transitionDuration: const Duration(milliseconds: 350),
      ),
    )
        .then((_) => _refresh());
  }

  Future<void> _deleteSession(Session session) async {
    final confirm = await showModalBottomSheet<bool>(
      context: context,
      backgroundColor: Colors.transparent,
      builder: (ctx) => Container(
        decoration: const BoxDecoration(
          color: CB.surface,
          borderRadius: BorderRadius.vertical(top: Radius.circular(24)),
        ),
        padding: const EdgeInsets.fromLTRB(24, 16, 24, 32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Center(
              child: Container(
                width: 40,
                height: 4,
                decoration: BoxDecoration(
                  color: Colors.white.withValues(alpha: 0.15),
                  borderRadius: BorderRadius.circular(2),
                ),
              ),
            ),
            const SizedBox(height: 24),
            const Text('End Session?',
                style: TextStyle(fontSize: 20, fontWeight: FontWeight.w700)),
            const SizedBox(height: 8),
            Text(
              session.workingDir,
              style: const TextStyle(
                  color: CB.textSecondary,
                  fontFamily: 'monospace',
                  fontSize: 13),
            ),
            const SizedBox(height: 24),
            Row(
              children: [
                Expanded(
                  child: GestureDetector(
                    onTap: () => Navigator.of(ctx).pop(false),
                    child: Container(
                      padding: const EdgeInsets.symmetric(vertical: 14),
                      decoration: BoxDecoration(
                        borderRadius: BorderRadius.circular(14),
                        border: Border.all(
                            color: Colors.white.withValues(alpha: 0.12)),
                      ),
                      child: const Center(
                        child: Text(
                          'Cancel',
                          style: TextStyle(
                            color: CB.textSecondary,
                            fontWeight: FontWeight.w600,
                            fontSize: 15,
                          ),
                        ),
                      ),
                    ),
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: GradientButton(
                    gradient: CB.dangerGradient,
                    onPressed: () => Navigator.of(ctx).pop(true),
                    padding: const EdgeInsets.symmetric(vertical: 14),
                    child: const Text(
                      'End Session',
                      style: TextStyle(
                        color: Colors.white,
                        fontWeight: FontWeight.w700,
                        fontSize: 15,
                      ),
                    ),
                  ),
                ),
              ],
            ),
          ],
        ),
      ),
    );
    if (confirm != true) return;
    try {
      await context.read<ApiService>().deleteSession(session.id);
      _refresh();
    } catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text('$e')));
    }
  }

  Future<void> _releaseApk() async {
    setState(() => _releasing = true);
    try {
      await context.read<ApiService>().releaseApk();
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('APK release triggered')),
      );
    } catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text('$e')));
    } finally {
      if (mounted) setState(() => _releasing = false);
    }
  }

  Future<void> _triggerUpdate() async {
    setState(() => _updating = true);
    try {
      await context.read<ApiService>().selfUpdate();
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Update complete — server is restarting'),
          backgroundColor: Color(0xFF00FFCC),
        ),
      );
    } catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text('Update failed: $e'),
          backgroundColor: Color(0xFFFF2D6B),
        ),
      );
    } finally {
      if (mounted) setState(() => _updating = false);
    }
  }

  void _showSettings() {
    final api = context.read<ApiService>();
    final controller = TextEditingController(text: api.baseUrl);
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      builder: (ctx) => Padding(
        padding:
            EdgeInsets.only(bottom: MediaQuery.of(ctx).viewInsets.bottom),
        child: Container(
          decoration: const BoxDecoration(
            color: CB.surface,
            borderRadius: BorderRadius.vertical(top: Radius.circular(24)),
          ),
          padding: const EdgeInsets.fromLTRB(24, 16, 24, 32),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Center(
                child: Container(
                  width: 40,
                  height: 4,
                  decoration: BoxDecoration(
                    color: Colors.white.withValues(alpha: 0.15),
                    borderRadius: BorderRadius.circular(2),
                  ),
                ),
              ),
              const SizedBox(height: 24),
              const GradientText(
                'Connection',
                style: TextStyle(
                    fontSize: 22,
                    fontWeight: FontWeight.w800,
                    letterSpacing: -0.5),
              ),
              const SizedBox(height: 8),
              const Text(
                'Enter your Orbitor server address.',
                style: TextStyle(color: CB.textSecondary, fontSize: 14),
              ),
              const SizedBox(height: 24),
              TextField(
                controller: controller,
                style:
                    const TextStyle(fontFamily: 'monospace', fontSize: 15),
                decoration: const InputDecoration(
                  hintText: 'http://100.x.y.z:8080',
                  prefixIcon: Icon(Icons.dns_outlined, color: CB.cyan),
                ),
                onSubmitted: (v) {
                  api.updateBaseUrl(v);
                  Navigator.of(ctx).pop();
                  _refresh();
                },
              ),
              const SizedBox(height: 20),
              SizedBox(
                width: double.infinity,
                child: GradientButton(
                  onPressed: () {
                    api.updateBaseUrl(controller.text);
                    Navigator.of(ctx).pop();
                    _refresh();
                  },
                  child: const Text('Save'),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  // --- Aggregate stats ---
  int _countByState(AgentState state) =>
      _sessions.where((s) => s.agentState == state).length;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: CustomScrollView(
        slivers: [
          SliverAppBar(
            floating: true,
            pinned: true,
            expandedHeight: 100,
            backgroundColor: CB.black,
            flexibleSpace: FlexibleSpaceBar(
              titlePadding: const EdgeInsets.only(left: 20, bottom: 16),
              title: const GradientText(
                'orbitor',
                style: TextStyle(
                    fontSize: 22,
                    fontWeight: FontWeight.w900,
                    letterSpacing: -1),
              ),
            ),
            actions: [
              if (_releasing)
                const Padding(
                  padding: EdgeInsets.symmetric(horizontal: 12),
                  child: SizedBox(
                    width: 20,
                    height: 20,
                    child: CircularProgressIndicator(
                        strokeWidth: 2, color: Color(0xFFFF8C00)),
                  ),
                )
              else
                IconButton(
                  icon: const Icon(Icons.send_to_mobile_rounded, size: 22),
                  onPressed: _releaseApk,
                  color: CB.textSecondary,
                  tooltip: 'Release APK',
                ),
              if (_updating)
                const Padding(
                  padding: EdgeInsets.symmetric(horizontal: 12),
                  child: SizedBox(
                    width: 20,
                    height: 20,
                    child: CircularProgressIndicator(
                        strokeWidth: 2, color: Color(0xFF00FFCC)),
                  ),
                )
              else
                IconButton(
                  icon:
                      const Icon(Icons.system_update_alt_rounded, size: 22),
                  onPressed: _triggerUpdate,
                  color: CB.textSecondary,
                  tooltip: 'Update server',
                ),
              IconButton(
                icon: const Icon(Icons.dns_outlined, size: 22),
                onPressed: _showSettings,
                color: CB.textSecondary,
              ),
              const SizedBox(width: 8),
            ],
          ),
          SliverToBoxAdapter(child: _buildBody()),
        ],
      ),
      floatingActionButton: _buildFAB(),
    );
  }

  Widget _buildFAB() {
    return Container(
      decoration: BoxDecoration(
        gradient: CB.accentGradient,
        borderRadius: BorderRadius.circular(18),
        boxShadow: [
          BoxShadow(
              color: CB.cyan.withValues(alpha: 0.3),
              blurRadius: 20,
              offset: const Offset(0, 6)),
        ],
      ),
      child: FloatingActionButton.extended(
        onPressed: _createSession,
        backgroundColor: Colors.transparent,
        foregroundColor: CB.black,
        elevation: 0,
        hoverElevation: 0,
        focusElevation: 0,
        highlightElevation: 0,
        icon: const Icon(Icons.add_rounded, color: CB.black),
        label: const Text('New Agent',
            style: TextStyle(color: CB.black, fontWeight: FontWeight.w800)),
      ),
    );
  }

  Widget _buildBody() {
    if (_loading) {
      return const Padding(
        padding: EdgeInsets.only(top: 120),
        child: Center(
            child: CircularProgressIndicator(
                color: CB.cyan, strokeWidth: 2.5)),
      );
    }
    
    // Mission summary panel
    Widget missionPanel() {
      if (_missionTitle == null && _missionSummary == null) return const SizedBox.shrink();
      if ((_missionTitle ?? '').isEmpty && (_missionSummary ?? '').isEmpty) return const SizedBox.shrink();
      return Padding(
        padding: const EdgeInsets.only(top: 12, bottom: 8),
        child: Container(
          width: double.infinity,
          padding: const EdgeInsets.fromLTRB(14, 12, 14, 12),
          decoration: BoxDecoration(
            color: Colors.white.withValues(alpha: 0.02),
            borderRadius: BorderRadius.circular(12),
            border: Border.all(color: Colors.white.withValues(alpha: 0.04)),
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              if ((_missionTitle ?? '').isNotEmpty)
                Text(
                  _missionTitle!,
                  style: const TextStyle(fontSize: 15, fontWeight: FontWeight.w800),
                ),
              if ((_missionSummary ?? '').isNotEmpty) ...[
                const SizedBox(height: 6),
                Text(
                  _missionSummary!,
                  style: const TextStyle(color: CB.textSecondary),
                ),
              ]
            ],
          ),
        ),
      );
    }
    if (_error != null) {
      return Padding(
        padding: const EdgeInsets.only(top: 80),
        child: Center(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Container(
                width: 64,
                height: 64,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: CB.hotPink.withValues(alpha: 0.1),
                ),
                child: const Icon(Icons.cloud_off_rounded,
                    size: 32, color: CB.hotPink),
              ),
              const SizedBox(height: 20),
              Text(
                _error!,
                style: const TextStyle(
                    color: CB.textSecondary, fontSize: 13),
                textAlign: TextAlign.center,
              ),
              const SizedBox(height: 24),
              GradientButton(
                onPressed: _refresh,
                child: const Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Icon(Icons.refresh_rounded, size: 18, color: CB.black),
                    SizedBox(width: 8),
                    Text('Retry'),
                  ],
                ),
              ),
            ],
          ),
        ),
      );
    }
    if (_sessions.isEmpty) {
      return Padding(
        padding: const EdgeInsets.only(top: 80),
        child: Center(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              ShaderMask(
                blendMode: BlendMode.srcIn,
                shaderCallback: (bounds) =>
                    CB.accentGradient.createShader(bounds),
                child: const Icon(Icons.terminal_rounded, size: 56),
              ),
              const SizedBox(height: 20),
              const Text(
                'No active agents',
                style: TextStyle(
                    color: CB.textSecondary,
                    fontSize: 16,
                    fontWeight: FontWeight.w500),
              ),
              const SizedBox(height: 6),
              const Text(
                'Tap + to launch one.',
                style: TextStyle(color: CB.textTertiary, fontSize: 14),
              ),
            ],
          ),
        ),
      );
    }

    // Sort by creation time only (newest first) so sessions don't jump
    // around when their state changes. Visual state indicators on each
    // card convey urgency without reordering.
    final sorted = List<Session>.from(_sessions)
      ..sort((a, b) => b.createdAt.compareTo(a.createdAt));

    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // mission summary
          missionPanel(),
          // --- Status bar ---
          _buildStatusBar(),
          const SizedBox(height: 16),
          // --- Agent cards ---
          ...sorted.map((s) => _agentCard(s)),
          const SizedBox(height: 80),
        ],
      ),
    );
  }

  Widget _buildStatusBar() {
    final working = _countByState(AgentState.working);
    final waiting = _countByState(AgentState.waitingForInput);
    final idle = _countByState(AgentState.idle);
    final starting = _countByState(AgentState.starting);
    final errored = _countByState(AgentState.error);
    final total = _sessions.length;

    return AnimatedBuilder(
      animation: _scanlineCtrl,
      builder: (_, __) {
        return Container(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(14),
            color: Colors.white.withValues(alpha: 0.03),
            border: Border.all(
                color: Colors.white.withValues(alpha: 0.06)),
          ),
          child: Column(
            children: [
              Row(
                children: [
                  ShaderMask(
                    blendMode: BlendMode.srcIn,
                    shaderCallback: (bounds) =>
                        CB.accentGradient.createShader(bounds),
                    child: const Icon(Icons.radar_rounded, size: 18),
                  ),
                  const SizedBox(width: 8),
                  Text(
                    '$total agent${total == 1 ? '' : 's'}',
                    style: const TextStyle(
                      fontSize: 13,
                      fontWeight: FontWeight.w700,
                      letterSpacing: 0.5,
                      color: CB.textSecondary,
                    ),
                  ),
                  const Spacer(),
                  // Live indicator
                  const PulsingDot(color: CB.neonGreen, size: 6),
                  const SizedBox(width: 6),
                  const Text(
                    'LIVE',
                    style: TextStyle(
                      fontSize: 9,
                      fontWeight: FontWeight.w800,
                      letterSpacing: 1.5,
                      color: CB.neonGreen,
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 10),
              Row(
                children: [
                  if (working > 0)
                    _statusPill(
                        '$working working', CB.cyan, Icons.memory_rounded),
                  if (waiting > 0)
                    _statusPill('$waiting waiting', CB.amber,
                        Icons.front_hand_rounded),
                  if (idle > 0)
                    _statusPill('$idle idle', CB.neonGreen,
                        Icons.check_circle_outline_rounded),
                  if (starting > 0)
                    _statusPill('$starting starting', CB.purple,
                        Icons.rocket_launch_rounded),
                  if (errored > 0)
                    _statusPill('$errored error', CB.hotPink,
                        Icons.error_outline_rounded),
                ],
              ),
            ],
          ),
        );
      },
    );
  }

  Widget _statusPill(String label, Color color, IconData icon) {
    return Padding(
      padding: const EdgeInsets.only(right: 8),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
        decoration: BoxDecoration(
          color: color.withValues(alpha: 0.12),
          borderRadius: BorderRadius.circular(8),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(icon, size: 11, color: color),
            const SizedBox(width: 4),
            Text(
              label,
              style: TextStyle(
                fontSize: 11,
                fontWeight: FontWeight.w700,
                color: color,
                letterSpacing: 0.3,
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _agentCard(Session s) {
    final state = s.agentState;
    final stateColor = _agentStateColor(state);
    final isAttentionNeeded = state == AgentState.waitingForInput;

    return GlassCard(
      margin: const EdgeInsets.only(bottom: 10),
      borderGradient: isAttentionNeeded ? CB.warmGradient : null,
      borderWidth: isAttentionNeeded ? 1.2 : 0,
      onTap: () => _openSession(s),
      padding: const EdgeInsets.fromLTRB(14, 12, 8, 12),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // Status indicator column
          Padding(
            padding: const EdgeInsets.only(top: 2),
            child: _agentStateIcon(state),
          ),
          const SizedBox(width: 12),
          // Main content
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                // Project name + state label
                Row(
                  children: [
                    Expanded(
                      child: Text(
                        s.title.isNotEmpty ? s.title : s.workingDir.split('/').last,
                        style: const TextStyle(
                            fontSize: 15,
                            fontWeight: FontWeight.w700,
                            letterSpacing: -0.3),
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                    _agentStateBadge(state, stateColor),
                  ],
                ),
                const SizedBox(height: 3),
                // Activity detail
                _agentActivityLine(s, state, stateColor),
                const SizedBox(height: 6),
                // Project directory path
                const SizedBox(height: 2),
                Text(
                  s.workingDir,
                  style: const TextStyle(
                    fontSize: 11,
                    color: CB.textTertiary,
                    fontFamily: 'monospace',
                  ),
                  overflow: TextOverflow.ellipsis,
                  maxLines: 1,
                ),
                const SizedBox(height: 4),
                // Meta row: backend, model, id
                Row(
                  children: [
                    _metaChip(
                      s.backend.toUpperCase(),
                      s.backend == 'claude' ? CB.amber : CB.purple,
                    ),
                    if (s.model.isNotEmpty) ...[
                      const SizedBox(width: 5),
                      _metaChip(s.model, CB.cyan, mono: true),
                    ],
                    const SizedBox(width: 5),
                    Text(s.id,
                        style: const TextStyle(
                            fontSize: 10, color: CB.textTertiary)),
                  ],
                ),
              ],
            ),
          ),
          // Actions
          Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              if (isAttentionNeeded)
                Container(
                  width: 36,
                  height: 36,
                  decoration: BoxDecoration(
                    borderRadius: BorderRadius.circular(10),
                    color: CB.amber.withValues(alpha: 0.15),
                  ),
                  child: const Icon(Icons.arrow_forward_rounded,
                      size: 18, color: CB.amber),
                )
              else
                SizedBox(
                  width: 36,
                  height: 36,
                  child: IconButton(
                    icon: Icon(Icons.close_rounded,
                        size: 18,
                        color: Colors.white.withValues(alpha: 0.2)),
                    padding: EdgeInsets.zero,
                    onPressed: () => _deleteSession(s),
                  ),
                ),
            ],
          ),
        ],
      ),
    );
  }

  Widget _agentActivityLine(Session s, AgentState state, Color stateColor) {
    switch (state) {
      case AgentState.working:
        if (s.currentTool.isNotEmpty) {
          return Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  _ToolSpinner(color: CB.cyan),
                  const SizedBox(width: 6),
                  Expanded(
                    child: Text(
                      s.currentTool,
                      style: TextStyle(
                          fontSize: 12,
                          color: CB.cyan.withValues(alpha: 0.85),
                          fontFamily: 'monospace'),
                      overflow: TextOverflow.ellipsis,
                    ),
                  ),
                ],
              ),
              if (s.currentPrompt.isNotEmpty) ...[
                const SizedBox(height: 3),
                Text(
                  s.currentPrompt.replaceAll('\n', ' '),
                  style: TextStyle(
                      fontSize: 11,
                      color: Colors.white.withValues(alpha: 0.35)),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
              ],
            ],
          );
        }
        if (s.currentPrompt.isNotEmpty) {
          return Row(
            children: [
              _ToolSpinner(color: CB.cyan),
              const SizedBox(width: 6),
              Expanded(
                child: Text(
                  s.currentPrompt.replaceAll('\n', ' '),
                  style: TextStyle(
                      fontSize: 12,
                      color: Colors.white.withValues(alpha: 0.5)),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
              ),
            ],
          );
        }
        if (s.lastMessage.isNotEmpty) {
          return Row(
            children: [
              _ToolSpinner(color: CB.cyan),
              const SizedBox(width: 6),
              Expanded(
                child: Text(
                  s.lastMessage.replaceAll('\n', ' '),
                  style: TextStyle(
                      fontSize: 12,
                      color: Colors.white.withValues(alpha: 0.5)),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
              ),
            ],
          );
        }
        return Text('Processing...',
            style: TextStyle(
                fontSize: 12,
                color: CB.cyan.withValues(alpha: 0.6)));
      case AgentState.waitingForInput:
        return Row(
          children: [
            const PulsingDot(color: CB.amber, size: 8),
            const SizedBox(width: 8),
            const Text(
              'Needs your approval',
              style: TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w600,
                color: CB.amber,
              ),
            ),
          ],
        );
      case AgentState.idle:
        if (s.summary.isNotEmpty) {
          return Text(
            s.summary,
            style: TextStyle(
                fontSize: 12,
                color: Colors.white.withValues(alpha: 0.4)),
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
          );
        }
        if (s.currentPrompt.isNotEmpty) {
          return Text(
            s.currentPrompt.replaceAll('\n', ' '),
            style: TextStyle(
                fontSize: 12,
                color: Colors.white.withValues(alpha: 0.4)),
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
          );
        }
        if (s.lastMessage.isNotEmpty) {
          return Text(
            s.lastMessage.replaceAll('\n', ' '),
            style: TextStyle(
                fontSize: 12,
                color: Colors.white.withValues(alpha: 0.4)),
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
          );
        }
        return Text('Ready for instructions',
            style: TextStyle(
                fontSize: 12,
                color: CB.neonGreen.withValues(alpha: 0.5)));
      case AgentState.starting:
        return Row(
          children: [
            const PulsingDot(color: CB.purple, size: 8),
            const SizedBox(width: 8),
            Text('Initializing...',
                style: TextStyle(
                    fontSize: 12,
                    color: CB.purple.withValues(alpha: 0.7))),
          ],
        );
      case AgentState.error:
        return Text('Agent error — tap to view',
            style: TextStyle(
                fontSize: 12,
                color: CB.hotPink.withValues(alpha: 0.8)));
      case AgentState.offline:
        return Text(s.status,
            style: const TextStyle(
                fontSize: 12, color: CB.textTertiary));
    }
  }

  Widget _agentStateIcon(AgentState state) {
    switch (state) {
      case AgentState.working:
        return _WorkingIndicator();
      case AgentState.waitingForInput:
        return Container(
          width: 28,
          height: 28,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            color: CB.amber.withValues(alpha: 0.15),
            border: Border.all(color: CB.amber.withValues(alpha: 0.4)),
          ),
          child: const Icon(Icons.front_hand_rounded,
              size: 14, color: CB.amber),
        );
      case AgentState.idle:
        return Container(
          width: 28,
          height: 28,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            color: CB.neonGreen.withValues(alpha: 0.1),
          ),
          child: Center(
            child: Container(
              width: 10,
              height: 10,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                color: CB.neonGreen,
                boxShadow: [
                  BoxShadow(
                      color: CB.neonGreen.withValues(alpha: 0.5),
                      blurRadius: 8,
                      spreadRadius: 1),
                ],
              ),
            ),
          ),
        );
      case AgentState.starting:
        return SizedBox(
          width: 28,
          height: 28,
          child: Center(child: PulsingDot(color: CB.purple, size: 14)),
        );
      case AgentState.error:
        return Container(
          width: 28,
          height: 28,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            color: CB.hotPink.withValues(alpha: 0.12),
          ),
          child: const Icon(Icons.error_rounded,
              color: CB.hotPink, size: 16),
        );
      case AgentState.offline:
        return Container(
          width: 28,
          height: 28,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            border:
                Border.all(color: CB.textTertiary.withValues(alpha: 0.4)),
          ),
          child: const Icon(Icons.power_settings_new_rounded,
              size: 14, color: CB.textTertiary),
        );
    }
  }

  Widget _agentStateBadge(AgentState state, Color color) {
    final label = switch (state) {
      AgentState.working => 'WORKING',
      AgentState.waitingForInput => 'NEEDS INPUT',
      AgentState.idle => 'IDLE',
      AgentState.starting => 'STARTING',
      AgentState.error => 'ERROR',
      AgentState.offline => 'OFFLINE',
    };
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 2),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.15),
        borderRadius: BorderRadius.circular(6),
      ),
      child: Text(
        label,
        style: TextStyle(
          fontSize: 9,
          fontWeight: FontWeight.w800,
          letterSpacing: 0.8,
          color: color,
        ),
      ),
    );
  }

  Color _agentStateColor(AgentState state) {
    return switch (state) {
      AgentState.working => CB.cyan,
      AgentState.waitingForInput => CB.amber,
      AgentState.idle => CB.neonGreen,
      AgentState.starting => CB.purple,
      AgentState.error => CB.hotPink,
      AgentState.offline => CB.textTertiary,
    };
  }

  Widget _metaChip(String label, Color color, {bool mono = false}) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(5),
      ),
      child: Text(
        label,
        style: TextStyle(
          fontSize: 9,
          fontWeight: FontWeight.w700,
          letterSpacing: mono ? 0.3 : 0.6,
          color: color,
          fontFamily: mono ? 'monospace' : null,
        ),
      ),
    );
  }
}

// --- Animated working indicator (rotating ring) ---
class _WorkingIndicator extends StatefulWidget {
  @override
  State<_WorkingIndicator> createState() => _WorkingIndicatorState();
}

class _WorkingIndicatorState extends State<_WorkingIndicator>
    with SingleTickerProviderStateMixin {
  late AnimationController _ctrl;

  @override
  void initState() {
    super.initState();
    _ctrl = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 1500),
    )..repeat();
  }

  @override
  void dispose() {
    _ctrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: 28,
      height: 28,
      child: Stack(
        alignment: Alignment.center,
        children: [
          RotationTransition(
            turns: _ctrl,
            child: Container(
              width: 28,
              height: 28,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                gradient: SweepGradient(
                  colors: [
                    CB.cyan.withValues(alpha: 0.0),
                    CB.cyan.withValues(alpha: 0.6),
                    CB.purple.withValues(alpha: 0.8),
                  ],
                ),
              ),
            ),
          ),
          Container(
            width: 22,
            height: 22,
            decoration: const BoxDecoration(
              shape: BoxShape.circle,
              color: CB.surface,
            ),
          ),
          Container(
            width: 8,
            height: 8,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              color: CB.cyan,
              boxShadow: [
                BoxShadow(
                    color: CB.cyan.withValues(alpha: 0.5),
                    blurRadius: 6,
                    spreadRadius: 1),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

// --- Tool execution spinner (small) ---
class _ToolSpinner extends StatefulWidget {
  final Color color;
  const _ToolSpinner({required this.color});

  @override
  State<_ToolSpinner> createState() => _ToolSpinnerState();
}

class _ToolSpinnerState extends State<_ToolSpinner>
    with SingleTickerProviderStateMixin {
  late AnimationController _ctrl;

  @override
  void initState() {
    super.initState();
    _ctrl = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 1000),
    )..repeat();
  }

  @override
  void dispose() {
    _ctrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: 12,
      height: 12,
      child: CircularProgressIndicator(
        strokeWidth: 1.5,
        color: widget.color.withValues(alpha: 0.7),
      ),
    );
  }
}

// --- New Session Bottom Sheet with backend toggle ---

class _NewSessionSheet extends StatefulWidget {
  const _NewSessionSheet();

  @override
  State<_NewSessionSheet> createState() => _NewSessionSheetState();
}

class _NewSessionSheetState extends State<_NewSessionSheet> {
  final _dirController = TextEditingController();
  String _backend = 'copilot';
  String _model = 'gpt-5-mini';
  bool _skipPermissions = false;

  static const _copilotModels = [
    'gpt-5-mini',
    'gpt-5.4',
    'gpt-5.3-codex',
    'gpt-5.1',
    'gpt-4.1',
    'claude-sonnet-4-6',
    'claude-opus-4-6',
  ];

  static const _claudeModels = [
    'claude-sonnet-4-6',
    'claude-opus-4-6',
    'claude-haiku-4-5',
  ];

  List<String> get _availableModels =>
      _backend == 'claude' ? _claudeModels : _copilotModels;

  @override
  void dispose() {
    _dirController.dispose();
    super.dispose();
  }

  void _submit() {
    Navigator.of(context).pop((
      dir: _dirController.text,
      backend: _backend,
      model: _model,
      skipPermissions: _skipPermissions
    ));
  }

  Future<void> _openBrowser() async {
    final selected = await showModalBottomSheet<String>(
      context: context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      builder: (_) => DraggableScrollableSheet(
        initialChildSize: 0.75,
        minChildSize: 0.4,
        maxChildSize: 0.95,
        builder: (ctx, scrollController) => _DirectoryBrowser(
          scrollController: scrollController,
          initialPath: _dirController.text,
        ),
      ),
    );
    if (selected != null) {
      setState(() => _dirController.text = selected);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding:
          EdgeInsets.only(bottom: MediaQuery.of(context).viewInsets.bottom),
      child: Container(
        decoration: const BoxDecoration(
          color: CB.surface,
          borderRadius: BorderRadius.vertical(top: Radius.circular(24)),
        ),
        padding: const EdgeInsets.fromLTRB(24, 16, 24, 32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Center(
              child: Container(
                width: 40,
                height: 4,
                decoration: BoxDecoration(
                  color: Colors.white.withValues(alpha: 0.15),
                  borderRadius: BorderRadius.circular(2),
                ),
              ),
            ),
            const SizedBox(height: 24),
            const GradientText(
              'New Agent',
              style: TextStyle(
                  fontSize: 22,
                  fontWeight: FontWeight.w800,
                  letterSpacing: -0.5),
            ),
            const SizedBox(height: 8),
            const Text(
              'Choose a backend and project directory.',
              style: TextStyle(color: CB.textSecondary, fontSize: 14),
            ),
            const SizedBox(height: 24),
            // Backend toggle
            Container(
              decoration: BoxDecoration(
                color: Colors.white.withValues(alpha: 0.04),
                borderRadius: BorderRadius.circular(14),
                border: Border.all(
                    color: Colors.white.withValues(alpha: 0.08)),
              ),
              padding: const EdgeInsets.all(4),
              child: Row(
                children: [
                  _backendOption('copilot', 'Copilot',
                      Icons.auto_fix_high_rounded, CB.purple),
                  const SizedBox(width: 4),
                  _backendOption('claude', 'Claude',
                      Icons.auto_awesome_rounded, CB.amber),
                ],
              ),
            ),
            const SizedBox(height: 12),
            // Model selector
            Container(
              decoration: BoxDecoration(
                color: Colors.white.withValues(alpha: 0.04),
                borderRadius: BorderRadius.circular(14),
                border: Border.all(
                    color: Colors.white.withValues(alpha: 0.08)),
              ),
              padding: const EdgeInsets.symmetric(horizontal: 16),
              child: DropdownButtonHideUnderline(
                child: DropdownButton<String>(
                  value: _availableModels.contains(_model)
                      ? _model
                      : _availableModels.first,
                  isExpanded: true,
                  dropdownColor: CB.surface,
                  icon: const Icon(Icons.expand_more_rounded,
                      color: CB.textSecondary),
                  style: const TextStyle(
                      fontSize: 14,
                      fontWeight: FontWeight.w500,
                      color: CB.textPrimary),
                  items: _availableModels
                      .map((m) => DropdownMenuItem(
                            value: m,
                            child: Row(
                              children: [
                                const Icon(Icons.model_training_rounded,
                                    size: 16, color: CB.cyan),
                                const SizedBox(width: 10),
                                Text(
                                  m,
                                  style: const TextStyle(
                                    color: CB.textPrimary,
                                    fontFamily: 'monospace',
                                    fontSize: 14,
                                  ),
                                ),
                              ],
                            ),
                          ))
                      .toList(),
                  onChanged: (v) => setState(() => _model = v ?? ''),
                ),
              ),
            ),
            const SizedBox(height: 12),
            // Skip permissions toggle
            Container(
              decoration: BoxDecoration(
                color: Colors.white.withValues(alpha: 0.04),
                borderRadius: BorderRadius.circular(14),
                border: Border.all(
                    color: Colors.white.withValues(alpha: 0.08)),
              ),
              padding:
                  const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
              child: Row(
                children: [
                  const Icon(Icons.warning_amber_rounded,
                      size: 18, color: CB.amber),
                  const SizedBox(width: 10),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        const Text(
                          'Skip permissions',
                          style: TextStyle(
                              fontSize: 14,
                              fontWeight: FontWeight.w600,
                              color: CB.textPrimary),
                        ),
                        Text(
                          _backend == 'claude'
                              ? '--dangerously-skip-permissions'
                              : '--yolo',
                          style: const TextStyle(
                              fontSize: 11,
                              fontFamily: 'monospace',
                              color: CB.textTertiary),
                        ),
                      ],
                    ),
                  ),
                  Switch(
                    value: _skipPermissions,
                    onChanged: (v) =>
                        setState(() => _skipPermissions = v),
                    activeThumbColor: CB.amber,
                  ),
                ],
              ),
            ),
            const SizedBox(height: 12),
            Row(
              children: [
                Expanded(
                  child: TextField(
                    controller: _dirController,
                    autofocus: true,
                    style: const TextStyle(
                        fontFamily: 'monospace', fontSize: 15),
                    decoration: const InputDecoration(
                      hintText: '/home/user/project',
                      prefixIcon:
                          Icon(Icons.folder_outlined, color: CB.cyan),
                    ),
                    onSubmitted: (_) => _submit(),
                  ),
                ),
                const SizedBox(width: 8),
                GestureDetector(
                  onTap: _openBrowser,
                  child: Container(
                    width: 52,
                    height: 52,
                    decoration: BoxDecoration(
                      gradient: CB.accentGradient,
                      borderRadius: BorderRadius.circular(14),
                    ),
                    child: const Icon(Icons.snippet_folder_rounded,
                        color: CB.black, size: 22),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 20),
            SizedBox(
              width: double.infinity,
              child: GradientButton(
                onPressed: _submit,
                child: const Row(
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    Icon(Icons.rocket_launch, size: 18, color: CB.black),
                    SizedBox(width: 8),
                    Text('Launch'),
                  ],
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _backendOption(
      String value, String label, IconData icon, Color color) {
    final selected = _backend == value;
    return Expanded(
      child: GestureDetector(
        onTap: () => setState(() {
          _backend = value;
          if (!_availableModels.contains(_model)) {
            _model = _availableModels.first;
          }
        }),
        child: AnimatedContainer(
          duration: const Duration(milliseconds: 200),
          curve: Curves.easeOutCubic,
          padding: const EdgeInsets.symmetric(vertical: 12),
          decoration: BoxDecoration(
            gradient: selected
                ? LinearGradient(colors: [
                    color.withValues(alpha: 0.2),
                    color.withValues(alpha: 0.08)
                  ])
                : null,
            borderRadius: BorderRadius.circular(10),
            border:
                selected ? Border.all(color: color.withValues(alpha: 0.4)) : null,
          ),
          child: Row(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              Icon(icon,
                  size: 18,
                  color: selected ? color : CB.textTertiary),
              const SizedBox(width: 8),
              Text(
                label,
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: selected ? FontWeight.w700 : FontWeight.w400,
                  color: selected ? color : CB.textTertiary,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

// --- Remote directory browser ---

class _DirectoryBrowser extends StatefulWidget {
  final ScrollController scrollController;
  final String initialPath;

  const _DirectoryBrowser(
      {required this.scrollController, this.initialPath = ''});

  @override
  State<_DirectoryBrowser> createState() => _DirectoryBrowserState();
}

class _DirectoryBrowserState extends State<_DirectoryBrowser> {
  String _currentPath = '';
  String _parentPath = '';
  List<BrowseEntry> _entries = [];
  bool _loading = true;
  String? _error;

  @override
  void initState() {
    super.initState();
    _browse(widget.initialPath);
  }

  Future<void> _browse(String path) async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final result = await context.read<ApiService>().browseDir(path);
      setState(() {
        _currentPath = result.path;
        _parentPath = result.parent;
        _entries = result.entries;
        _loading = false;
      });
    } catch (e) {
      setState(() {
        _error = e.toString();
        _loading = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: const BoxDecoration(
        color: CB.surface,
        borderRadius: BorderRadius.vertical(top: Radius.circular(24)),
      ),
      child: Column(
        children: [
          // Handle
          Padding(
            padding: const EdgeInsets.only(top: 12, bottom: 8),
            child: Center(
              child: Container(
                width: 40,
                height: 4,
                decoration: BoxDecoration(
                  color: Colors.white.withValues(alpha: 0.15),
                  borderRadius: BorderRadius.circular(2),
                ),
              ),
            ),
          ),
          // Header
          Padding(
            padding: const EdgeInsets.fromLTRB(20, 8, 12, 0),
            child: Row(
              children: [
                const Expanded(
                  child: GradientText(
                    'Browse',
                    style: TextStyle(
                        fontSize: 20,
                        fontWeight: FontWeight.w800,
                        letterSpacing: -0.5),
                  ),
                ),
                GradientButton(
                  onPressed: () => Navigator.of(context).pop(_currentPath),
                  padding: const EdgeInsets.symmetric(
                      horizontal: 18, vertical: 10),
                  child: const Text('Select'),
                ),
                const SizedBox(width: 4),
              ],
            ),
          ),
          // Breadcrumb path
          Padding(
            padding: const EdgeInsets.fromLTRB(20, 12, 20, 8),
            child: GestureDetector(
              onTap: () => _browse(_parentPath),
              child: Container(
                width: double.infinity,
                padding:
                    const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
                decoration: BoxDecoration(
                  color: Colors.white.withValues(alpha: 0.04),
                  borderRadius: BorderRadius.circular(10),
                  border: Border.all(
                      color: Colors.white.withValues(alpha: 0.06)),
                ),
                child: Row(
                  children: [
                    const Icon(Icons.folder_open_rounded,
                        size: 16, color: CB.cyan),
                    const SizedBox(width: 10),
                    Expanded(
                      child: Text(
                        _currentPath,
                        style: const TextStyle(
                            fontFamily: 'monospace',
                            fontSize: 13,
                            color: CB.textSecondary),
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ),
          // Directory list
          Expanded(child: _buildContent()),
        ],
      ),
    );
  }

  Widget _buildContent() {
    if (_loading) {
      return const Center(
          child:
              CircularProgressIndicator(color: CB.cyan, strokeWidth: 2));
    }
    if (_error != null) {
      return Center(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(Icons.error_outline_rounded,
                  color: CB.hotPink, size: 32),
              const SizedBox(height: 12),
              Text(_error!,
                  style: const TextStyle(
                      color: CB.textSecondary, fontSize: 13),
                  textAlign: TextAlign.center),
              const SizedBox(height: 16),
              GradientButton(
                onPressed: () => _browse(_parentPath),
                padding: const EdgeInsets.symmetric(
                    horizontal: 20, vertical: 10),
                child: const Text('Go up'),
              ),
            ],
          ),
        ),
      );
    }

    final items = <Widget>[
      // Go up
      if (_currentPath != _parentPath)
        _dirTile('..', _parentPath, isUp: true),
    ];
    for (final entry in _entries) {
      items.add(_dirTile(entry.name, entry.path));
    }

    if (_entries.isEmpty && _currentPath == _parentPath) {
      items.add(
        const Padding(
          padding: EdgeInsets.all(32),
          child: Center(
            child: Text('Root directory',
                style:
                    TextStyle(color: CB.textTertiary, fontSize: 14)),
          ),
        ),
      );
    }

    return ListView.builder(
      controller: widget.scrollController,
      padding: const EdgeInsets.symmetric(horizontal: 12),
      itemCount: items.length,
      itemBuilder: (_, i) => items[i],
    );
  }

  Widget _dirTile(String name, String path, {bool isUp = false}) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 2),
      child: Material(
        color: Colors.transparent,
        child: InkWell(
          onTap: () => _browse(path),
          borderRadius: BorderRadius.circular(12),
          splashColor: CB.cyan.withValues(alpha: 0.08),
          child: Padding(
            padding:
                const EdgeInsets.symmetric(horizontal: 12, vertical: 12),
            child: Row(
              children: [
                Container(
                  width: 36,
                  height: 36,
                  decoration: BoxDecoration(
                    color: isUp
                        ? Colors.white.withValues(alpha: 0.06)
                        : CB.cyan.withValues(alpha: 0.08),
                    borderRadius: BorderRadius.circular(10),
                  ),
                  child: Icon(
                    isUp
                        ? Icons.arrow_upward_rounded
                        : Icons.folder_rounded,
                    size: 18,
                    color: isUp ? CB.textSecondary : CB.cyan,
                  ),
                ),
                const SizedBox(width: 14),
                Expanded(
                  child: Text(
                    name,
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight:
                          isUp ? FontWeight.w400 : FontWeight.w500,
                      color: isUp ? CB.textSecondary : CB.textPrimary,
                    ),
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
                Icon(
                  Icons.chevron_right_rounded,
                  size: 20,
                  color: Colors.white.withValues(alpha: 0.15),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
