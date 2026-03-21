import 'package:flutter/material.dart';
import '../theme.dart';

Color toolKindColor(String kind) {
  switch (kind) {
    case 'read': return CB.cyan;
    case 'edit': return CB.amber;
    case 'execute': return CB.neonGreen;
    case 'search': return CB.purple;
    case 'delete': return CB.hotPink;
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
    default: return Icons.build_rounded;
  }
}

/// Compact, expandable single-row tool call indicator.
class ToolCallCard extends StatefulWidget {
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
  State<ToolCallCard> createState() => _ToolCallCardState();
}

class _ToolCallCardState extends State<ToolCallCard> {
  bool _expanded = false;

  @override
  Widget build(BuildContext context) {
    final color = toolKindColor(widget.kind);
    final icon = toolKindIcon(widget.kind);
    final isPending = widget.status == 'pending' || widget.status.isEmpty;
    final isFailed = widget.status == 'failed';
    final hasContent = widget.content != null && widget.content!.isNotEmpty;

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 2),
      child: GestureDetector(
        onTap: hasContent ? () => setState(() => _expanded = !_expanded) : null,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                // Kind icon box
                Container(
                  width: 20,
                  height: 20,
                  decoration: BoxDecoration(
                    color: color.withValues(alpha: isPending ? 0.18 : 0.1),
                    borderRadius: BorderRadius.circular(5),
                  ),
                  child: isPending
                      ? Center(
                          child: SizedBox(
                            width: 10,
                            height: 10,
                            child: CircularProgressIndicator(
                              strokeWidth: 1.5,
                              color: color,
                            ),
                          ),
                        )
                      : Icon(icon, size: 11, color: color),
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(
                    widget.title,
                    style: TextStyle(
                      fontSize: 12,
                      fontFamily: 'monospace',
                      color: isPending ? CB.textSecondary : CB.textTertiary,
                      fontWeight: isPending ? FontWeight.w500 : FontWeight.w400,
                    ),
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
                if (isFailed)
                  Icon(Icons.close_rounded, size: 12, color: CB.hotPink),
                if (!isPending && !isFailed)
                  Icon(
                    Icons.check_rounded,
                    size: 12,
                    color: color.withValues(alpha: 0.45),
                  ),
                if (hasContent) ...[
                  const SizedBox(width: 4),
                  Icon(
                    _expanded
                        ? Icons.expand_less_rounded
                        : Icons.chevron_right_rounded,
                    size: 13,
                    color: CB.textTertiary.withValues(alpha: 0.6),
                  ),
                ],
              ],
            ),
            if (_expanded && hasContent)
              Padding(
                padding: const EdgeInsets.only(left: 28, top: 5, bottom: 3),
                child: Container(
                  width: double.infinity,
                  padding: const EdgeInsets.all(8),
                  constraints: const BoxConstraints(maxHeight: 200),
                  decoration: BoxDecoration(
                    color: Colors.white.withValues(alpha: 0.03),
                    borderRadius: BorderRadius.circular(6),
                    border: Border.all(color: color.withValues(alpha: 0.12)),
                  ),
                  child: SingleChildScrollView(
                    child: SelectableText(
                      widget.content!,
                      style: const TextStyle(
                        fontSize: 11,
                        fontFamily: 'monospace',
                        color: CB.textSecondary,
                        height: 1.4,
                      ),
                    ),
                  ),
                ),
              ),
          ],
        ),
      ),
    );
  }
}
