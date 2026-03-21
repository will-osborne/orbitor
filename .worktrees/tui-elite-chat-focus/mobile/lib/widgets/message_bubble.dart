import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_markdown/flutter_markdown.dart';
import '../theme.dart';

class MessageBubble extends StatelessWidget {
  final String text;
  final bool isUser;

  const MessageBubble({super.key, required this.text, required this.isUser});

  @override
  Widget build(BuildContext context) {
    return Align(
      alignment: isUser ? Alignment.centerRight : Alignment.centerLeft,
      child: Container(
        constraints: BoxConstraints(maxWidth: MediaQuery.of(context).size.width * 0.85),
        margin: const EdgeInsets.symmetric(vertical: 5),
        child: isUser ? _userBubble() : _agentBubble(context),
      ),
    );
  }

  Widget _userBubble() {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      decoration: BoxDecoration(
        gradient: CB.accentGradient,
        borderRadius: const BorderRadius.only(
          topLeft: Radius.circular(20),
          topRight: Radius.circular(20),
          bottomLeft: Radius.circular(20),
          bottomRight: Radius.circular(6),
        ),
        boxShadow: [
          BoxShadow(
            color: CB.cyan.withValues(alpha: 0.15),
            blurRadius: 16,
            offset: const Offset(0, 4),
          ),
        ],
      ),
      child: SelectableText(
        text,
        style: const TextStyle(
          color: CB.black,
          fontSize: 15,
          fontWeight: FontWeight.w500,
          height: 1.45,
        ),
      ),
    );
  }

  Widget _agentBubble(BuildContext context) {
    return Container(
      decoration: BoxDecoration(
        gradient: LinearGradient(
          colors: [
            CB.cyan.withValues(alpha: 0.25),
            CB.purple.withValues(alpha: 0.12),
            Colors.transparent,
          ],
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
        ),
        borderRadius: const BorderRadius.only(
          topLeft: Radius.circular(20),
          topRight: Radius.circular(20),
          bottomLeft: Radius.circular(6),
          bottomRight: Radius.circular(20),
        ),
      ),
      padding: const EdgeInsets.all(1),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
        decoration: const BoxDecoration(
          color: Color(0xFF0C0C18),
          borderRadius: BorderRadius.only(
            topLeft: Radius.circular(19),
            topRight: Radius.circular(19),
            bottomLeft: Radius.circular(5),
            bottomRight: Radius.circular(19),
          ),
        ),
        child: MarkdownBody(
          data: text,
          selectable: true,
          shrinkWrap: true,
          styleSheet: _markdownStyle(context),
          builders: {'code': _CodeBlockBuilder()},
        ),
      ),
    );
  }

  MarkdownStyleSheet _markdownStyle(BuildContext context) {
    final baseText = TextStyle(
      color: Colors.white.withValues(alpha: 0.92),
      fontSize: 15,
      height: 1.5,
    );
    return MarkdownStyleSheet(
      p: baseText,
      h1: baseText.copyWith(fontSize: 22, fontWeight: FontWeight.w700, height: 1.3),
      h2: baseText.copyWith(fontSize: 19, fontWeight: FontWeight.w700, height: 1.3),
      h3: baseText.copyWith(fontSize: 17, fontWeight: FontWeight.w600, height: 1.3),
      strong: baseText.copyWith(fontWeight: FontWeight.w700),
      em: baseText.copyWith(fontStyle: FontStyle.italic),
      code: TextStyle(
        color: CB.cyan,
        backgroundColor: Colors.white.withValues(alpha: 0.06),
        fontFamily: 'monospace',
        fontSize: 13.5,
      ),
      codeblockDecoration: BoxDecoration(
        color: const Color(0xFF06060E),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: Colors.white.withValues(alpha: 0.08)),
      ),
      codeblockPadding: const EdgeInsets.all(14),
      codeblockAlign: WrapAlignment.start,
      blockquoteDecoration: BoxDecoration(
        border: Border(
          left: BorderSide(color: CB.cyan.withValues(alpha: 0.4), width: 3),
        ),
      ),
      blockquotePadding: const EdgeInsets.only(left: 14, top: 4, bottom: 4),
      listBullet: baseText.copyWith(color: CB.cyan),
      tableHead: baseText.copyWith(fontWeight: FontWeight.w700),
      tableBody: baseText,
      tableBorder: TableBorder.all(color: Colors.white.withValues(alpha: 0.1)),
      tableCellsPadding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      horizontalRuleDecoration: BoxDecoration(
        border: Border(top: BorderSide(color: Colors.white.withValues(alpha: 0.1))),
      ),
      a: baseText.copyWith(color: CB.cyan, decoration: TextDecoration.underline),
    );
  }
}

/// Custom builder for code blocks — adds a copy button and language label.
class _CodeBlockBuilder extends MarkdownElementBuilder {
  @override
  Widget? visitElementAfterWithContext(
    BuildContext context,
    element,
    TextStyle? preferredStyle,
    TextStyle? parentStyle,
  ) {
    if (element.tag != 'code') return null;
    // Only handle fenced code blocks (inside <pre>)
    final parent = element.attributes['class'];
    if (parent == null && element.textContent.contains('\n') == false) return null;

    final lang = parent?.replaceFirst('language-', '') ?? '';
    final code = element.textContent.trimRight();

    return _CodeBlockWidget(code: code, language: lang);
  }
}

class _CodeBlockWidget extends StatelessWidget {
  final String code;
  final String language;

  const _CodeBlockWidget({required this.code, required this.language});

  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      decoration: BoxDecoration(
        color: const Color(0xFF06060E),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: Colors.white.withValues(alpha: 0.08)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // Header with language and copy button
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 8),
            decoration: BoxDecoration(
              border: Border(
                bottom: BorderSide(color: Colors.white.withValues(alpha: 0.06)),
              ),
            ),
            child: Row(
              children: [
                if (language.isNotEmpty)
                  Text(
                    language,
                    style: TextStyle(
                      fontSize: 11,
                      fontWeight: FontWeight.w600,
                      color: CB.cyan.withValues(alpha: 0.7),
                      fontFamily: 'monospace',
                    ),
                  ),
                const Spacer(),
                GestureDetector(
                  onTap: () {
                    Clipboard.setData(ClipboardData(text: code));
                    ScaffoldMessenger.of(context).showSnackBar(
                      const SnackBar(
                        content: Text('Copied to clipboard'),
                        duration: Duration(seconds: 1),
                      ),
                    );
                  },
                  child: Icon(
                    Icons.copy_rounded,
                    size: 14,
                    color: Colors.white.withValues(alpha: 0.35),
                  ),
                ),
              ],
            ),
          ),
          // Code content
          SingleChildScrollView(
            scrollDirection: Axis.horizontal,
            padding: const EdgeInsets.all(14),
            child: SelectableText(
              code,
              style: TextStyle(
                fontFamily: 'monospace',
                fontSize: 13,
                height: 1.5,
                color: Colors.white.withValues(alpha: 0.85),
              ),
            ),
          ),
        ],
      ),
    );
  }
}
