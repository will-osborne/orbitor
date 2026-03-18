import 'package:flutter/material.dart';
import '../theme.dart';

Color toolKindColor(String kind) {
  switch (kind) {
    case 'read': return CB.cyan;
    case 'edit': return CB.amber;
    case 'execute': return CB.neonGreen;
    case 'search': return CB.purple;
    case 'delete': return CB.hotPink;
    case 'result': return const Color(0xFF00D4AA);
    default: return CB.textTertiary;
  }
}

IconData toolKindIcon(String kind) {
  switch (kind) {
    case 'read': return Icons.visibility_rounded;
    case 'edit': return Icons.edit_rounded;
    case 'execute': return Icons.terminal_rounded;
    case 'search': return Icons.search_rounded;
    case 'delete': return Icons.delete_rounded;
    case 'result': return Icons.output_rounded;
    default: return Icons.build_rounded;
  }
}

class ToolCallCard extends StatelessWidget {
  final String title;
  final String kind;
  final String status;
  final String? content;

  const ToolCallCard({
    super.key,
    required this.title,
    required this.kind,
    required this.status,
    this.content,
  });

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: GlassCard(
        padding: EdgeInsets.zero,
        child: IntrinsicHeight(
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              // Colored accent bar
              Container(
                width: 3,
                decoration: BoxDecoration(
                  gradient: LinearGradient(
                    begin: Alignment.topCenter,
                    end: Alignment.bottomCenter,
                    colors: [_kindColor(), _kindColor().withValues(alpha: 0.2)],
                  ),
                  borderRadius: const BorderRadius.only(
                    topLeft: Radius.circular(16),
                    bottomLeft: Radius.circular(16),
                  ),
                ),
              ),
              Expanded(
              child: Padding(
                padding: const EdgeInsets.all(12),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        _kindIcon(),
                        const SizedBox(width: 8),
                        Expanded(
                          child: Text(
                            title,
                            style: const TextStyle(
                              fontSize: 13,
                              fontFamily: 'monospace',
                              fontWeight: FontWeight.w500,
                            ),
                            overflow: TextOverflow.ellipsis,
                          ),
                        ),
                        _statusChip(),
                      ],
                    ),
                    if (content != null && content!.isNotEmpty) ...[
                      const SizedBox(height: 10),
                      Container(
                        width: double.infinity,
                        padding: const EdgeInsets.all(10),
                        decoration: BoxDecoration(
                          color: Colors.white.withValues(alpha: 0.03),
                          borderRadius: BorderRadius.circular(8),
                          border: Border.all(color: Colors.white.withValues(alpha: 0.05)),
                        ),
                        constraints: const BoxConstraints(maxHeight: 100),
                        child: SingleChildScrollView(
                          child: SelectableText(
                            content!,
                            style: const TextStyle(
                              fontSize: 11,
                              fontFamily: 'monospace',
                              color: CB.textSecondary,
                              height: 1.4,
                            ),
                          ),
                        ),
                      ),
                    ],
                  ],
                ),
              ),
            ),
            ],
          ),
        ),
      ),
    );
  }

  Color _kindColor() => toolKindColor(kind);

  Widget _kindIcon() {
    final color = _kindColor();
    final icon = toolKindIcon(kind);
    return Container(
      width: 26, height: 26,
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(7),
      ),
      child: Icon(icon, size: 14, color: color),
    );
  }

  Widget _statusChip() {
    Color color;
    switch (status) {
      case 'pending':
        color = CB.amber;
      case 'completed':
        color = CB.neonGreen;
      case 'failed':
        color = CB.hotPink;
      default:
        color = CB.textTertiary;
    }
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: color.withValues(alpha: 0.2)),
      ),
      child: Text(
        status.toUpperCase(),
        style: TextStyle(
          fontSize: 9,
          fontWeight: FontWeight.w700,
          letterSpacing: 0.8,
          color: color,
        ),
      ),
    );
  }
}
