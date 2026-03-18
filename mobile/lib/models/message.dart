enum MessageType {
  agentText,
  userPrompt,
  toolCall,
  toolResult,
  permissionRequest,
  permissionResolved,
  runComplete,
  interrupted,
  error,
  status,
}

class ChatMessage {
  final MessageType type;
  final String text;
  final String? toolCallId;
  final String? toolTitle;
  final String? toolKind;
  final String? toolStatus;
  final PermissionData? permission;
  final DateTime timestamp;

  ChatMessage({
    required this.type,
    this.text = '',
    this.toolCallId,
    this.toolTitle,
    this.toolKind,
    this.toolStatus,
    this.permission,
    DateTime? timestamp,
  }) : timestamp = timestamp ?? DateTime.now();

  factory ChatMessage.fromWS(Map<String, dynamic> json) {
    final type = json['type'] as String;
    final data = json['data'] as Map<String, dynamic>? ?? {};

    switch (type) {
      case 'agent_text':
        return ChatMessage(type: MessageType.agentText, text: data['text'] ?? '');
      case 'tool_call':
        return ChatMessage(
          type: MessageType.toolCall,
          toolCallId: data['toolCallId'],
          toolTitle: data['title'],
          toolKind: data['kind'],
          toolStatus: data['status'],
          text: data['content'] ?? '',
        );
      case 'tool_result':
        return ChatMessage(
          type: MessageType.toolResult,
          toolCallId: data['toolCallId'],
          text: data['content'] ?? '',
        );
      case 'permission_request':
        return ChatMessage(
          type: MessageType.permissionRequest,
          permission: PermissionData.fromJson(data),
        );
      case 'permission_resolved':
        return ChatMessage(
          type: MessageType.permissionResolved,
          text: 'Permission ${data['optionId']} for ${data['requestId']}',
        );
      case 'run_complete':
        return ChatMessage(
          type: MessageType.runComplete,
          text: data['stopReason'] ?? 'done',
        );
      case 'interrupted':
        return ChatMessage(type: MessageType.interrupted, text: 'Interrupted');
      case 'error':
        return ChatMessage(type: MessageType.error, text: data['message'] ?? 'unknown error');
      case 'status':
        return ChatMessage(type: MessageType.status, text: data['status'] ?? '');
      case 'prompt_sent':
        return ChatMessage(type: MessageType.userPrompt, text: data['text'] ?? '');
      default:
        return ChatMessage(type: MessageType.status, text: '[$type] ${data.toString()}');
    }
  }
}

class PermissionData {
  final String requestId;
  final String title;
  final String kind;
  final String command;
  final List<PermissionOption> options;
  bool resolved;

  PermissionData({
    required this.requestId,
    required this.title,
    required this.kind,
    required this.command,
    required this.options,
    this.resolved = false,
  });

  factory PermissionData.fromJson(Map<String, dynamic> json) {
    final options = (json['options'] as List? ?? [])
        .map((o) => PermissionOption.fromJson(o))
        .toList();
    return PermissionData(
      requestId: json['requestId'] ?? '',
      title: json['title'] ?? '',
      kind: json['kind'] ?? '',
      command: json['command'] ?? '',
      options: options,
    );
  }
}

class PermissionOption {
  final String optionId;
  final String name;
  final String kind;

  PermissionOption({required this.optionId, required this.name, required this.kind});

  factory PermissionOption.fromJson(Map<String, dynamic> json) {
    return PermissionOption(
      optionId: json['optionId'] ?? '',
      name: json['name'] ?? '',
      kind: json['kind'] ?? '',
    );
  }
}
