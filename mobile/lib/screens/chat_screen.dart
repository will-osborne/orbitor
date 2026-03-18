import 'dart:async';
import 'dart:ui';
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../models/session.dart';
import '../models/message.dart';
import '../services/api_service.dart';
import '../widgets/message_bubble.dart';
import '../widgets/permission_card.dart';
import '../widgets/robot_animation.dart';
import '../widgets/tool_call_card.dart';
import '../theme.dart';

class ChatScreen extends StatefulWidget {
  final Session session;
  const ChatScreen({super.key, required this.session});

  @override
  State<ChatScreen> createState() => _ChatScreenState();
}

class _ChatScreenState extends State<ChatScreen>
    with TickerProviderStateMixin, WidgetsBindingObserver {
  final TextEditingController _inputController = TextEditingController();
  final TextEditingController _searchController = TextEditingController();
  final ScrollController _scrollController = ScrollController();
  final List<ChatMessage> _messages = [];
  SessionConnection? _connection;
  StreamSubscription? _sub;
  bool _connected = false;
  bool _isAgentRunning = false;
  bool _sessionReady = false;
  bool _showDebug = false;
  bool _searchOpen = false;
  String _searchQuery = '';
  bool _appInForeground = true;
  String? _currentToolTitle;
  String? _currentToolKind;
  late bool _skipPermissions;

  // Incoming message buffering to reduce UI rebuild frequency
  final List<ChatMessage> _incomingBuffer = [];
  Timer? _flushTimer;
  final Duration _flushInterval = const Duration(milliseconds: 50);

  @override
  void initState() {
    super.initState();
    _sessionReady = widget.session.status == 'ready';
    _skipPermissions = widget.session.skipPermissions;
    WidgetsBinding.instance.addObserver(this);
    _connect();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    final wasInForeground = _appInForeground;
    _appInForeground = state == AppLifecycleState.resumed;

    if (state == AppLifecycleState.paused ||
        state == AppLifecycleState.inactive) {
      _connection?.onBackground();
    }

    if (state == AppLifecycleState.resumed && !wasInForeground) {
      _connection?.onResume();
    }
  }

  void _connect() {
    final api = context.read<ApiService>();
    _connection = api.connectToSession(widget.session.id);
    setState(() => _connected = true);
    _sub = _connection!.messageStream.listen(
      (msg) {
        if (!mounted) return;
        // A reconnection status message means the WS is back up
        if (msg.type == MessageType.status && msg.text == 'Reconnected') {
          setState(() => _connected = true);
          return;
        }

        // Session became ready
        if (msg.type == MessageType.status &&
            msg.text == 'ready' &&
            !_sessionReady) {
          setState(() => _sessionReady = true);
        }

        // Buffer the incoming message and flush on a timer to batch setState calls
        _enqueueIncoming(msg);
      },
      onError: (e) {
        if (!mounted) return;
        _flushTimer?.cancel();
        setState(() {
          _connected = false;
          _isAgentRunning = false;
          _messages.add(
            ChatMessage(type: MessageType.error, text: 'Connection lost: $e'),
          );
        });
      },
      onDone: () {
        if (!mounted) return;
        _flushTimer?.cancel();
        setState(() {
          _connected = false;
          _isAgentRunning = false;
        });
      },
    );
  }

  void _scrollToBottom({bool animate = false}) {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted || !_scrollController.hasClients) return;
      if (animate) {
        _scrollController.animateTo(
          _scrollController.position.maxScrollExtent,
          duration: const Duration(milliseconds: 200),
          curve: Curves.easeOutCubic,
        );
      } else {
        _scrollController.jumpTo(_scrollController.position.maxScrollExtent);
      }
    });
  }

  void _enqueueIncoming(ChatMessage msg) {
    _incomingBuffer.add(msg);
    // simple backpressure to avoid unbounded growth
    if (_incomingBuffer.length > 1000) {
      _incomingBuffer.removeAt(0);
    }
    _flushTimer ??= Timer(_flushInterval, () => _flushIncoming());
  }

  void _applyMessages(List<ChatMessage> toAdd) {
    // Check if this batch contains a terminal message (runComplete, error,
    // interrupted). If so, don't let earlier messages in the same batch
    // re-set _isAgentRunning to true — the terminal message is authoritative.
    final hasTerminal = toAdd.any(
      (m) =>
          m.type == MessageType.runComplete ||
          m.type == MessageType.error ||
          m.type == MessageType.interrupted,
    );

    for (final msg in toAdd) {
      // Detect session becoming ready from status broadcast
      if (msg.type == MessageType.status && msg.text == 'ready') {
        _sessionReady = true;
      }
      // When a permission is resolved, mark the matching request card
      // as resolved so it collapses to a small "Approved" pill.
      if (msg.type == MessageType.permissionResolved) {
        for (final m in _messages.reversed) {
          if (m.type == MessageType.permissionRequest &&
              m.permission != null &&
              !m.permission!.resolved &&
              msg.text.contains(m.permission!.requestId)) {
            m.permission!.resolved = true;
            break;
          }
        }
        // Don't add the resolved message itself — the card handles it.
        continue;
      }
      if (msg.type == MessageType.agentText &&
          _messages.isNotEmpty &&
          _messages.last.type == MessageType.agentText) {
        _messages[_messages.length - 1] = ChatMessage(
          type: MessageType.agentText,
          text: _messages.last.text + msg.text,
          timestamp: _messages.last.timestamp,
        );
      } else if (msg.type == MessageType.toolCall && msg.toolCallId != null) {
        // Update existing tool call in-place instead of adding a duplicate.
        final idx = _messages.lastIndexWhere(
          (m) =>
              m.type == MessageType.toolCall && m.toolCallId == msg.toolCallId,
        );
        if (idx >= 0) {
          _messages[idx] = msg;
        } else {
          _messages.add(msg);
        }
      } else {
        _messages.add(msg);
      }
      // Only mark as running if this batch doesn't also contain a terminal message.
      if (!hasTerminal &&
          (msg.type == MessageType.userPrompt ||
              msg.type == MessageType.agentText ||
              msg.type == MessageType.toolCall)) {
        _isAgentRunning = true;
      }
      if (msg.type == MessageType.toolCall) {
        final status = msg.toolStatus ?? '';
        if (status == 'completed' || status == 'failed') {
          _currentToolTitle = null;
          _currentToolKind = null;
        } else {
          _currentToolTitle = msg.toolTitle;
          _currentToolKind = msg.toolKind;
        }
      }
      if (msg.type == MessageType.toolResult) {
        _currentToolTitle = null;
        _currentToolKind = null;
      }
      if (msg.type == MessageType.runComplete ||
          msg.type == MessageType.error ||
          msg.type == MessageType.interrupted) {
        _isAgentRunning = false;
        _currentToolTitle = null;
        _currentToolKind = null;
      }
    }
    // Cap message list to prevent unbounded memory growth
    if (_messages.length > 2000) {
      _messages.removeRange(0, _messages.length - 2000);
    }
  }

  void _flushIncoming({bool callSetState = true}) {
    if (_incomingBuffer.isEmpty) return;
    final toAdd = List<ChatMessage>.from(_incomingBuffer);
    _incomingBuffer.clear();
    _flushTimer?.cancel();
    _flushTimer = null;

    if (callSetState && mounted) {
      setState(() => _applyMessages(toAdd));
      _scrollToBottom();
    } else {
      _applyMessages(toAdd);
    }
  }

  void _sendMessage() {
    final text = _inputController.text.trim();
    if (text.isEmpty || _connection == null || !_sessionReady) return;
    _connection!.sendPrompt(text);
    _inputController.clear();
    _scrollToBottom(animate: true);
  }

  void _interruptSession() {
    _connection?.sendInterrupt();
  }

  void _respondToPermission(String requestId, String optionId) {
    _connection?.respondToPermission(requestId, optionId);
  }

  void _toggleSkipPermissions() async {
    final newValue = !_skipPermissions;
    try {
      final api = context.read<ApiService>();
      await api.updateSession(widget.session.id, skipPermissions: newValue);
      if (!mounted) return;
      setState(() {
        _skipPermissions = newValue;
        _sessionReady = false;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _messages.add(
          ChatMessage(
            type: MessageType.error,
            text: 'Failed to toggle YOLO mode: $e',
          ),
        );
      });
    }
  }

  Future<void> _confirmAndDeleteSession() async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Delete session'),
        content: const Text('Are you sure you want to delete this session? This will stop the agent and remove it from the list.'),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(false),
            child: const Text('Cancel'),
          ),
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(true),
            child: const Text('Delete', style: TextStyle(color: Colors.red)),
          ),
        ],
      ),
    );
    if (confirmed != true) return;

    // Show a progress dialog while deleting
    showDialog(
      context: context,
      barrierDismissible: false,
      builder: (ctx) => const Center(child: CircularProgressIndicator()),
    );

    try {
      final api = context.read<ApiService>();
      await api.deleteSession(widget.session.id);
      // Dispose connection immediately
      try {
        _connection?.dispose();
      } catch (_) {}
      if (!mounted) return;
      Navigator.of(context).pop(); // close progress
      // Pop the chat screen to return to sessions list
      Navigator.of(context).pop();
    } catch (e) {
      if (!mounted) return;
      Navigator.of(context).pop(); // close progress
      // Show error in chat stream
      setState(() {
        _messages.add(
          ChatMessage(type: MessageType.error, text: 'Failed to delete session: $e'),
        );
      });
    }
  }

  List<ChatMessage> get _filteredMessages {
    var msgs = _messages.where((m) {
      if (!_showDebug && m.type == MessageType.status) return false;
      return true;
    }).toList();

    if (_searchQuery.isNotEmpty) {
      final q = _searchQuery.toLowerCase();
      msgs = msgs.where((m) => m.text.toLowerCase().contains(q)).toList();
    }
    return msgs;
  }

  /// Groups consecutive completed/failed tool calls into _CollapsedToolGroup
  /// display items. Non-tool messages and pending/running tool calls pass
  /// through as _SingleMessage items.
  List<_DisplayItem> get _displayItems {
    final filtered = _filteredMessages;
    final items = <_DisplayItem>[];
    int i = 0;
    while (i < filtered.length) {
      final msg = filtered[i];
      if (_isCompletedTool(msg)) {
        // Collect consecutive completed tools.
        final group = <ChatMessage>[msg];
        int j = i + 1;
        while (j < filtered.length && _isCompletedTool(filtered[j])) {
          group.add(filtered[j]);
          j++;
        }
        if (group.length >= 3) {
          items.add(_CollapsedToolGroup(group));
        } else {
          for (final m in group) {
            items.add(_SingleMessage(m));
          }
        }
        i = j;
      } else {
        items.add(_SingleMessage(msg));
        i++;
      }
    }
    return items;
  }

  bool _isCompletedTool(ChatMessage msg) {
    if (msg.type == MessageType.toolCall) {
      final s = msg.toolStatus ?? '';
      return s == 'completed' || s == 'failed';
    }
    if (msg.type == MessageType.toolResult) return true;
    return false;
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _sub?.cancel();
    _flushTimer?.cancel();
    if (_incomingBuffer.isNotEmpty) {
      _flushIncoming(callSetState: false);
    }
    _connection?.dispose();
    _inputController.dispose();
    _searchController.dispose();
    _scrollController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final items = _displayItems;
    return Scaffold(
      extendBodyBehindAppBar: true,
      appBar: PreferredSize(
        preferredSize: Size.fromHeight(_searchOpen ? 110 : 64),
        child: ClipRect(
          child: BackdropFilter(
            filter: ImageFilter.blur(sigmaX: 30, sigmaY: 30),
            child: Container(
              color: CB.black.withValues(alpha: 0.7),
              child: SafeArea(
                bottom: false,
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Padding(
                      padding: const EdgeInsets.symmetric(horizontal: 4),
                      child: Row(
                        children: [
                          IconButton(
                            icon: const Icon(Icons.arrow_back_rounded),
                            onPressed: () => Navigator.of(context).pop(),
                            color: CB.textSecondary,
                          ),
                          const SizedBox(width: 4),
                          Expanded(
                            child: Column(
                              mainAxisAlignment: MainAxisAlignment.center,
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                Text(
                                  widget.session.workingDir.split('/').last,
                                  style: const TextStyle(
                                    fontSize: 17,
                                    fontWeight: FontWeight.w700,
                                    letterSpacing: -0.3,
                                  ),
                                ),
                                const SizedBox(height: 2),
                                Row(
                                  children: [
                                    if (_connected && !_sessionReady)
                                      const PulsingDot(color: CB.cyan, size: 7)
                                    else
                                      Container(
                                        width: 7,
                                        height: 7,
                                        decoration: BoxDecoration(
                                          shape: BoxShape.circle,
                                          color: _connected
                                              ? CB.neonGreen
                                              : CB.hotPink,
                                          boxShadow: [
                                            BoxShadow(
                                              color:
                                                  (_connected
                                                          ? CB.neonGreen
                                                          : CB.hotPink)
                                                      .withValues(alpha: 0.5),
                                              blurRadius: 6,
                                            ),
                                          ],
                                        ),
                                      ),
                                    const SizedBox(width: 6),
                                    Text(
                                      !_connected
                                          ? 'Disconnected'
                                          : _sessionReady
                                          ? 'Connected'
                                          : 'Spawning...',
                                      style: TextStyle(
                                        fontSize: 12,
                                        color: !_connected
                                            ? CB.hotPink.withValues(alpha: 0.8)
                                            : _sessionReady
                                            ? CB.neonGreen.withValues(
                                                alpha: 0.8,
                                              )
                                            : CB.cyan.withValues(alpha: 0.8),
                                        fontWeight: FontWeight.w500,
                                      ),
                                    ),
                                  ],
                                ),
                              ],
                            ),
                          ),
                          if (_isAgentRunning)
                            Padding(
                              padding: const EdgeInsets.only(right: 4),
                              child: PulsingDot(
                                color: _currentToolKind != null
                                    ? toolKindColor(_currentToolKind!)
                                    : CB.cyan,
                                size: 10,
                              ),
                            ),
                          IconButton(
                            icon: Icon(
                              _skipPermissions
                                  ? Icons.shield_rounded
                                  : Icons.shield_outlined,
                              size: 20,
                              color: _skipPermissions
                                  ? CB.amber
                                  : CB.textTertiary,
                            ),
                            onPressed: _toggleSkipPermissions,
                          ),
                          IconButton(
                            icon: const Icon(Icons.search_rounded, size: 20),
                            onPressed: () => setState(() {
                              _searchOpen = !_searchOpen;
                              if (!_searchOpen) {
                                _searchQuery = '';
                                _searchController.clear();
                              }
                            }),
                            color: _searchOpen ? CB.cyan : CB.textSecondary,
                          ),
                          IconButton(
                            icon: Icon(
                              Icons.bug_report_rounded,
                              size: 20,
                              color: _showDebug ? CB.amber : CB.textTertiary,
                            ),
                            onPressed: () =>
                                setState(() => _showDebug = !_showDebug),
                          ),
                          // Delete session from inside the chat view
                          IconButton(
                            icon: Icon(
                              Icons.delete_outline_rounded,
                              size: 20,
                              color: CB.hotPink,
                            ),
                            onPressed: _confirmAndDeleteSession,
                          ),
                        ],
                      ),
                    ),
                    if (_searchOpen)
                      Padding(
                        padding: const EdgeInsets.fromLTRB(16, 4, 16, 8),
                        child: TextField(
                          controller: _searchController,
                          autofocus: true,
                          style: const TextStyle(fontSize: 14),
                          decoration: InputDecoration(
                            hintText: 'Search messages...',
                            prefixIcon: const Icon(
                              Icons.search_rounded,
                              size: 18,
                              color: CB.textTertiary,
                            ),
                            suffixIcon: _searchQuery.isNotEmpty
                                ? IconButton(
                                    icon: const Icon(
                                      Icons.clear_rounded,
                                      size: 16,
                                    ),
                                    onPressed: () {
                                      _searchController.clear();
                                      setState(() => _searchQuery = '');
                                    },
                                  )
                                : null,
                            contentPadding: const EdgeInsets.symmetric(
                              horizontal: 14,
                              vertical: 10,
                            ),
                            border: OutlineInputBorder(
                              borderRadius: BorderRadius.circular(12),
                              borderSide: BorderSide(
                                color: Colors.white.withValues(alpha: 0.08),
                              ),
                            ),
                            enabledBorder: OutlineInputBorder(
                              borderRadius: BorderRadius.circular(12),
                              borderSide: BorderSide(
                                color: Colors.white.withValues(alpha: 0.08),
                              ),
                            ),
                            focusedBorder: OutlineInputBorder(
                              borderRadius: BorderRadius.circular(12),
                              borderSide: const BorderSide(
                                color: CB.cyan,
                                width: 1,
                              ),
                            ),
                          ),
                          onChanged: (v) => setState(() => _searchQuery = v),
                        ),
                      ),
                  ],
                ),
              ),
            ),
          ),
        ),
      ),
      body: Stack(
        children: [
          Positioned(
            top: 0,
            left: 0,
            right: 0,
            height: 200,
            child: Container(
              decoration: BoxDecoration(
                gradient: LinearGradient(
                  begin: Alignment.topCenter,
                  end: Alignment.bottomCenter,
                  colors: [CB.cyan.withValues(alpha: 0.03), Colors.transparent],
                ),
              ),
            ),
          ),
          Column(
            children: [
              Expanded(
                child: items.isEmpty
                    ? _buildEmptyState()
                    : ListView.builder(
                        controller: _scrollController,
                        padding: EdgeInsets.fromLTRB(
                          16,
                          MediaQuery.of(context).padding.top +
                              (_searchOpen ? 110 : 64) +
                              12,
                          16,
                          16,
                        ),
                        itemCount: items.length + (_isAgentRunning ? 1 : 0),
                        itemBuilder: (ctx, i) {
                          if (i < items.length) {
                            final item = items[i];
                            return switch (item) {
                              _SingleMessage s => _buildMessage(s.message),
                              _CollapsedToolGroup g => _ToolGroupCard(
                                tools: g.tools,
                                buildMessage: _buildMessage,
                              ),
                            };
                          }
                          // Agent working indicator at the bottom
                          return _buildWorkingIndicator();
                        },
                      ),
              ),
              _buildInputBar(),
            ],
          ),
        ],
      ),
    );
  }

  Widget _buildEmptyState() {
    if (_searchQuery.isNotEmpty) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              Icons.search_off_rounded,
              size: 48,
              color: CB.textTertiary.withValues(alpha: 0.5),
            ),
            const SizedBox(height: 16),
            Text(
              'No results for "$_searchQuery"',
              style: const TextStyle(color: CB.textSecondary, fontSize: 15),
            ),
          ],
        ),
      );
    }

    // Session is still spawning / respawning
    if (!_sessionReady) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const BootingRobot(size: 140),
            const SizedBox(height: 24),
            GradientText(
              widget.session.status == 'starting'
                  ? 'Booting up...'
                  : 'Respawning...',
              style: const TextStyle(
                fontSize: 18,
                fontWeight: FontWeight.w700,
                letterSpacing: -0.3,
              ),
            ),
            const SizedBox(height: 8),
            Text(
              widget.session.workingDir.split('/').last,
              style: const TextStyle(
                color: CB.textSecondary,
                fontFamily: 'monospace',
                fontSize: 13,
              ),
            ),
            const SizedBox(height: 4),
            Text(
              widget.session.backend.toUpperCase(),
              style: TextStyle(
                color: widget.session.backend == 'claude'
                    ? CB.amber
                    : CB.purple,
                fontSize: 11,
                fontWeight: FontWeight.w700,
                letterSpacing: 0.8,
              ),
            ),
          ],
        ),
      );
    }

    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          ShaderMask(
            blendMode: BlendMode.srcIn,
            shaderCallback: (bounds) => CB.accentGradient.createShader(bounds),
            child: const Icon(Icons.auto_awesome_rounded, size: 48),
          ),
          const SizedBox(height: 16),
          const Text(
            'Ask Copilot anything',
            style: TextStyle(
              color: CB.textSecondary,
              fontSize: 16,
              fontWeight: FontWeight.w500,
            ),
          ),
          const SizedBox(height: 6),
          Text(
            widget.session.workingDir,
            style: const TextStyle(
              color: CB.textTertiary,
              fontFamily: 'monospace',
              fontSize: 12,
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildWorkingIndicator() {
    final hasToolContext =
        _currentToolTitle != null && _currentToolTitle!.isNotEmpty;
    final kindColor = hasToolContext
        ? toolKindColor(_currentToolKind ?? '')
        : CB.cyan;

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 16),
      child: Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const WorkingRobot(size: 100),
            const SizedBox(height: 12),
            AnimatedSwitcher(
              duration: const Duration(milliseconds: 200),
              child: hasToolContext
                  ? Row(
                      key: ValueKey(_currentToolTitle),
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        Icon(
                          toolKindIcon(_currentToolKind ?? ''),
                          size: 14,
                          color: kindColor.withValues(alpha: 0.8),
                        ),
                        const SizedBox(width: 6),
                        Flexible(
                          child: Text(
                            _currentToolTitle!,
                            style: TextStyle(
                              color: kindColor.withValues(alpha: 0.8),
                              fontSize: 13,
                              fontWeight: FontWeight.w500,
                              fontFamily: 'monospace',
                            ),
                            overflow: TextOverflow.ellipsis,
                          ),
                        ),
                      ],
                    )
                  : Text(
                      key: const ValueKey('working'),
                      'Working on it...',
                      style: TextStyle(
                        color: CB.cyan.withValues(alpha: 0.7),
                        fontSize: 13,
                        fontWeight: FontWeight.w500,
                      ),
                    ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildMessage(ChatMessage msg) {
    switch (msg.type) {
      case MessageType.agentText:
        return MessageBubble(text: msg.text, isUser: false);
      case MessageType.userPrompt:
        return MessageBubble(text: msg.text, isUser: true);
      case MessageType.toolCall:
        return ToolCallCard(
          title: msg.toolTitle ?? 'Tool call',
          kind: msg.toolKind ?? '',
          status: msg.toolStatus ?? '',
          content: msg.text.isNotEmpty ? msg.text : null,
        );
      case MessageType.toolResult:
        return ToolCallCard(
          title: 'Result: ${msg.toolCallId ?? ''}',
          kind: 'result',
          status: 'completed',
          content: msg.text,
        );
      case MessageType.permissionRequest:
        if (msg.permission == null) return const SizedBox.shrink();
        return PermissionCard(
          permission: msg.permission!,
          onRespond: _respondToPermission,
        );
      case MessageType.permissionResolved:
        return _statusPill(msg.text, CB.neonGreen, Icons.check_circle_rounded);
      case MessageType.runComplete:
        return _statusPill(
          'Done  -  ${msg.text}',
          CB.cyan,
          Icons.done_all_rounded,
        );
      case MessageType.interrupted:
        return _statusPill(
          'Interrupted',
          CB.hotPink,
          Icons.stop_circle_outlined,
        );
      case MessageType.error:
        return _statusPill(msg.text, CB.hotPink, Icons.error_rounded);
      case MessageType.status:
        return _statusPill(
          msg.text,
          CB.textTertiary,
          Icons.info_outline_rounded,
        );
    }
  }

  Widget _statusPill(String text, Color color, IconData icon) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: Center(
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 7),
          decoration: BoxDecoration(
            color: color.withValues(alpha: 0.1),
            borderRadius: BorderRadius.circular(20),
            border: Border.all(color: color.withValues(alpha: 0.2)),
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(icon, size: 14, color: color),
              const SizedBox(width: 8),
              Flexible(
                child: Text(
                  text,
                  style: TextStyle(
                    fontSize: 12,
                    color: color,
                    fontWeight: FontWeight.w500,
                  ),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildInputBar() {
    return ClipRect(
      child: BackdropFilter(
        filter: ImageFilter.blur(sigmaX: 30, sigmaY: 30),
        child: Container(
          padding: EdgeInsets.fromLTRB(
            16,
            12,
            12,
            MediaQuery.of(context).padding.bottom + 12,
          ),
          decoration: BoxDecoration(
            color: CB.black.withValues(alpha: 0.7),
            border: Border(
              top: BorderSide(color: Colors.white.withValues(alpha: 0.06)),
            ),
          ),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.end,
            children: [
              Expanded(
                child: TextField(
                  controller: _inputController,
                  maxLines: 5,
                  minLines: 1,
                  enabled: _sessionReady,
                  style: const TextStyle(fontSize: 15, height: 1.4),
                  decoration: InputDecoration(
                    hintText: _sessionReady
                        ? 'Message Copilot...'
                        : 'Waiting for agent...',
                    border: OutlineInputBorder(
                      borderRadius: BorderRadius.circular(20),
                      borderSide: BorderSide(
                        color: Colors.white.withValues(alpha: 0.08),
                      ),
                    ),
                    enabledBorder: OutlineInputBorder(
                      borderRadius: BorderRadius.circular(20),
                      borderSide: BorderSide(
                        color: Colors.white.withValues(alpha: 0.08),
                      ),
                    ),
                    focusedBorder: OutlineInputBorder(
                      borderRadius: BorderRadius.circular(20),
                      borderSide: const BorderSide(color: CB.cyan, width: 1),
                    ),
                    disabledBorder: OutlineInputBorder(
                      borderRadius: BorderRadius.circular(20),
                      borderSide: BorderSide(
                        color: Colors.white.withValues(alpha: 0.04),
                      ),
                    ),
                    contentPadding: const EdgeInsets.symmetric(
                      horizontal: 18,
                      vertical: 12,
                    ),
                  ),
                  textInputAction: TextInputAction.send,
                  onSubmitted: (_) => _sendMessage(),
                ),
              ),
              const SizedBox(width: 8),
              _buildSendButton(),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildSendButton() {
    if (_isAgentRunning) {
      return GestureDetector(
        onTap: _interruptSession,
        child: Container(
          width: 44,
          height: 44,
          decoration: BoxDecoration(
            color: CB.hotPink.withValues(alpha: 0.15),
            borderRadius: BorderRadius.circular(14),
            border: Border.all(color: CB.hotPink.withValues(alpha: 0.5)),
          ),
          child: const Icon(Icons.stop_rounded, color: CB.hotPink, size: 22),
        ),
      );
    }

    final enabled = _connected && _sessionReady;
    return GestureDetector(
      onTap: enabled ? _sendMessage : null,
      child: Container(
        width: 44,
        height: 44,
        decoration: BoxDecoration(
          gradient: enabled ? CB.accentGradient : null,
          color: enabled ? null : CB.textTertiary.withValues(alpha: 0.3),
          borderRadius: BorderRadius.circular(14),
          boxShadow: enabled
              ? [
                  BoxShadow(
                    color: CB.cyan.withValues(alpha: 0.3),
                    blurRadius: 12,
                    offset: const Offset(0, 4),
                  ),
                ]
              : null,
        ),
        child: Icon(
          Icons.arrow_upward_rounded,
          color: enabled ? CB.black : CB.textTertiary,
          size: 22,
        ),
      ),
    );
  }
}

// Display item types for grouping consecutive tool calls.
sealed class _DisplayItem {}

class _SingleMessage extends _DisplayItem {
  final ChatMessage message;
  _SingleMessage(this.message);
}

class _CollapsedToolGroup extends _DisplayItem {
  final List<ChatMessage> tools;
  _CollapsedToolGroup(this.tools);
}

/// A compact, expandable summary for a run of consecutive completed tool calls.
class _ToolGroupCard extends StatefulWidget {
  final List<ChatMessage> tools;
  final Widget Function(ChatMessage) buildMessage;

  const _ToolGroupCard({required this.tools, required this.buildMessage});

  @override
  State<_ToolGroupCard> createState() => _ToolGroupCardState();
}

class _ToolGroupCardState extends State<_ToolGroupCard> {
  bool _expanded = false;

  @override
  Widget build(BuildContext context) {
    // Count by kind for the summary.
    final kindCounts = <String, int>{};
    for (final t in widget.tools) {
      if (t.type == MessageType.toolResult) continue;
      final k = t.toolKind ?? 'other';
      kindCounts[k] = (kindCounts[k] ?? 0) + 1;
    }
    // If all are results, count those.
    if (kindCounts.isEmpty) {
      kindCounts['result'] = widget.tools.length;
    }

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: Column(
        children: [
          GestureDetector(
            onTap: () => setState(() => _expanded = !_expanded),
            child: GlassCard(
              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
              child: Row(
                children: [
                  Icon(
                    _expanded
                        ? Icons.expand_less_rounded
                        : Icons.expand_more_rounded,
                    color: CB.textTertiary,
                    size: 18,
                  ),
                  const SizedBox(width: 8),
                  ...kindCounts.entries.expand(
                    (e) => [
                      Container(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 6,
                          vertical: 2,
                        ),
                        margin: const EdgeInsets.only(right: 6),
                        decoration: BoxDecoration(
                          color: toolKindColor(e.key).withValues(alpha: 0.12),
                          borderRadius: BorderRadius.circular(6),
                        ),
                        child: Row(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            Icon(
                              toolKindIcon(e.key),
                              size: 11,
                              color: toolKindColor(e.key),
                            ),
                            const SizedBox(width: 4),
                            Text(
                              '${e.value}',
                              style: TextStyle(
                                fontSize: 11,
                                fontWeight: FontWeight.w600,
                                color: toolKindColor(e.key),
                              ),
                            ),
                          ],
                        ),
                      ),
                    ],
                  ),
                  const Spacer(),
                  Text(
                    '${widget.tools.where((t) => t.type == MessageType.toolCall).length} tools',
                    style: const TextStyle(
                      fontSize: 11,
                      color: CB.textTertiary,
                      fontFamily: 'monospace',
                    ),
                  ),
                ],
              ),
            ),
          ),
          if (_expanded) ...widget.tools.map((t) => widget.buildMessage(t)),
        ],
      ),
    );
  }
}
