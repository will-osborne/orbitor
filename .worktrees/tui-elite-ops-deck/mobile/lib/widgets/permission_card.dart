import 'package:flutter/material.dart';
import '../models/message.dart';
import '../theme.dart';

class PermissionCard extends StatelessWidget {
  final PermissionData permission;
  final void Function(String requestId, String optionId) onRespond;

  const PermissionCard({
    super.key,
    required this.permission,
    required this.onRespond,
  });

  @override
  Widget build(BuildContext context) {
    if (permission.resolved) {
      return Padding(
        padding: const EdgeInsets.symmetric(vertical: 4),
        child: Center(
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 7),
            decoration: BoxDecoration(
              color: CB.neonGreen.withValues(alpha: 0.08),
              borderRadius: BorderRadius.circular(20),
            ),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(Icons.check_circle_rounded, size: 14, color: CB.neonGreen),
                const SizedBox(width: 8),
                Text('Approved',
                    style: TextStyle(fontSize: 12, color: CB.neonGreen, fontWeight: FontWeight.w500)),
                if (permission.command.isNotEmpty) ...[
                  const SizedBox(width: 10),
                  Flexible(
                    child: Text(
                      permission.command,
                      style: const TextStyle(fontSize: 12, color: CB.textSecondary),
                      overflow: TextOverflow.ellipsis,
                      maxLines: 1,
                    ),
                  ),
                ] else if (permission.title.isNotEmpty) ...[
                  const SizedBox(width: 10),
                  Flexible(
                    child: Text(
                      permission.title,
                      style: const TextStyle(fontSize: 12, color: CB.textSecondary),
                      overflow: TextOverflow.ellipsis,
                      maxLines: 1,
                    ),
                  ),
                ],
              ],
            ),
          ),
        ),
      );
    }

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: GlassCard(
        borderGradient: CB.warmGradient,
        borderWidth: 1.5,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            // Header row
            Row(
              children: [
                Container(
                  width: 32, height: 32,
                  decoration: BoxDecoration(
                    gradient: CB.warmGradient,
                    borderRadius: BorderRadius.circular(9),
                  ),
                  child: Icon(_kindIcon(), size: 18, color: CB.black),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        permission.title.isNotEmpty ? permission.title : 'Permission Required',
                        style: const TextStyle(
                          fontSize: 14,
                          fontWeight: FontWeight.w700,
                          letterSpacing: -0.3,
                        ),
                        maxLines: 2,
                        overflow: TextOverflow.ellipsis,
                      ),
                      const SizedBox(height: 2),
                      Row(
                        children: [
                          Container(
                            padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                            decoration: BoxDecoration(
                              color: _kindColor().withValues(alpha: 0.15),
                              borderRadius: BorderRadius.circular(4),
                            ),
                            child: Text(
                              permission.kind.isNotEmpty ? permission.kind.toUpperCase() : 'TOOL',
                              style: TextStyle(
                                fontSize: 9,
                                fontWeight: FontWeight.w800,
                                letterSpacing: 0.8,
                                color: _kindColor(),
                              ),
                            ),
                          ),
                          const SizedBox(width: 6),
                          const PulsingDot(color: CB.amber, size: 5),
                          const SizedBox(width: 4),
                          const Text(
                            'Waiting for approval',
                            style: TextStyle(fontSize: 11, color: CB.textSecondary),
                          ),
                        ],
                      ),
                    ],
                  ),
                ),
              ],
            ),

            // Command preview (if available)
            if (permission.command.isNotEmpty) ...[
              const SizedBox(height: 12),
              Container(
                width: double.infinity,
                padding: const EdgeInsets.all(10),
                decoration: BoxDecoration(
                  color: Colors.white.withValues(alpha: 0.04),
                  borderRadius: BorderRadius.circular(8),
                  border: Border.all(color: Colors.white.withValues(alpha: 0.06)),
                ),
                constraints: const BoxConstraints(maxHeight: 80),
                child: SingleChildScrollView(
                  child: Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        '\$ ',
                        style: TextStyle(
                          fontSize: 12,
                          fontFamily: 'monospace',
                          fontWeight: FontWeight.w700,
                          color: _kindColor().withValues(alpha: 0.7),
                        ),
                      ),
                      Expanded(
                        child: SelectableText(
                          permission.command,
                          style: const TextStyle(
                            fontSize: 12,
                            fontFamily: 'monospace',
                            color: CB.textPrimary,
                            height: 1.4,
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ],

            // Action buttons
            const SizedBox(height: 14),
            Row(
              children: _buildButtons(),
            ),
          ],
        ),
      ),
    );
  }

  List<Widget> _buildButtons() {
    // Sort: deny first (left), then allow options (right) — prominent position
    final deny = permission.options.where((o) => !o.kind.contains('allow')).toList();
    final allow = permission.options.where((o) => o.kind.contains('allow')).toList();
    final ordered = [...deny, ...allow];

    return ordered.asMap().entries.map((entry) {
      final i = entry.key;
      final opt = entry.value;
      final isAllow = opt.kind.contains('allow');
      final isAlways = opt.kind == 'allow_always';

      return Expanded(
        child: Padding(
          padding: EdgeInsets.only(right: i < ordered.length - 1 ? 8 : 0),
          child: isAllow
              ? _allowButton(opt, isAlways)
              : _denyButton(opt),
        ),
      );
    }).toList();
  }

  Widget _allowButton(PermissionOption opt, bool isAlways) {
    return GestureDetector(
      onTap: () => onRespond(permission.requestId, opt.optionId),
      child: Container(
        padding: const EdgeInsets.symmetric(vertical: 10),
        decoration: BoxDecoration(
          gradient: isAlways
              ? const LinearGradient(colors: [CB.neonGreen, Color(0xFF00CC6A)])
              : null,
          color: isAlways ? null : CB.neonGreen.withValues(alpha: 0.12),
          borderRadius: BorderRadius.circular(10),
          border: isAlways ? null : Border.all(color: CB.neonGreen.withValues(alpha: 0.3)),
        ),
        child: Center(
          child: Text(
            opt.name,
            style: TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w700,
              color: isAlways ? CB.black : CB.neonGreen,
            ),
          ),
        ),
      ),
    );
  }

  Widget _denyButton(PermissionOption opt) {
    return GestureDetector(
      onTap: () => onRespond(permission.requestId, opt.optionId),
      child: Container(
        padding: const EdgeInsets.symmetric(vertical: 10),
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(10),
          border: Border.all(color: CB.hotPink.withValues(alpha: 0.3)),
          color: CB.hotPink.withValues(alpha: 0.08),
        ),
        child: Center(
          child: Text(
            opt.name,
            style: TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w600,
              color: CB.hotPink.withValues(alpha: 0.9),
            ),
          ),
        ),
      ),
    );
  }

  IconData _kindIcon() {
    switch (permission.kind) {
      case 'execute': return Icons.terminal_rounded;
      case 'read': return Icons.visibility_rounded;
      case 'edit': return Icons.edit_rounded;
      case 'delete': return Icons.delete_rounded;
      default: return Icons.shield_rounded;
    }
  }

  Color _kindColor() {
    switch (permission.kind) {
      case 'execute': return CB.neonGreen;
      case 'read': return CB.cyan;
      case 'edit': return CB.amber;
      case 'delete': return CB.hotPink;
      default: return CB.amber;
    }
  }
}
