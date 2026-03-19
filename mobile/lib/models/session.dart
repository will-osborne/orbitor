class SubAgentInfo {
  final String toolCallId;
  final String title;
  final String status;
  final DateTime startedAt;

  SubAgentInfo({
    required this.toolCallId,
    this.title = '',
    this.status = 'running',
    DateTime? startedAt,
  }) : startedAt = startedAt ?? DateTime.fromMillisecondsSinceEpoch(0);

  factory SubAgentInfo.fromJson(Map<String, dynamic> json) {
    return SubAgentInfo(
      toolCallId: json['toolCallId'] ?? '',
      title: json['title'] ?? '',
      status: json['status'] ?? 'running',
      startedAt: json['startedAt'] != null
          ? DateTime.tryParse(json['startedAt'] as String) ?? DateTime.fromMillisecondsSinceEpoch(0)
          : DateTime.fromMillisecondsSinceEpoch(0),
    );
  }
}

class Session {
  final String id;
  final String workingDir;
  final String acpSessionId;
  final String status;
  final String backend;
  final String model;
  final bool skipPermissions;
  final bool planMode;
  final String lastMessage;
  final String currentTool;
  final String currentPrompt;
  final bool isRunning;
  final bool pendingPermission;
  final String title;
  final String summary;
  final String prUrl;
  final DateTime createdAt;
  final List<SubAgentInfo> subAgents;

  Session({
    required this.id,
    required this.workingDir,
    this.acpSessionId = '',
    required this.status,
    this.backend = 'copilot',
    this.model = '',
    this.skipPermissions = false,
    this.planMode = false,
    this.lastMessage = '',
    this.currentTool = '',
    this.currentPrompt = '',
    this.isRunning = false,
    this.pendingPermission = false,
    this.title = '',
    this.summary = '',
    this.prUrl = '',
    DateTime? createdAt,
    List<SubAgentInfo>? subAgents,
  })  : createdAt = createdAt ?? DateTime.fromMillisecondsSinceEpoch(0),
        subAgents = subAgents ?? [];

  /// Derived agent state for UI display.
  AgentState get agentState {
    if (status == 'starting') return AgentState.starting;
    if (status == 'error') return AgentState.error;
    if (status != 'ready') return AgentState.offline;
    if (pendingPermission) return AgentState.waitingForInput;
    if (isRunning) return AgentState.working;
    return AgentState.idle;
  }

  factory Session.fromJson(Map<String, dynamic> json) {
    return Session(
      id: json['id'] ?? '',
      workingDir: json['workingDir'] ?? '',
      acpSessionId: json['acpSessionId'] ?? '',
      status: json['status'] ?? 'unknown',
      backend: json['backend'] ?? 'copilot',
      model: json['model'] ?? '',
      skipPermissions: json['skipPermissions'] ?? false,
      planMode: json['planMode'] ?? false,
      lastMessage: json['lastMessage'] ?? '',
      currentTool: json['currentTool'] ?? '',
      currentPrompt: json['currentPrompt'] ?? '',
      isRunning: json['isRunning'] ?? false,
      pendingPermission: json['pendingPermission'] ?? false,
      title: json['title'] ?? '',
      summary: json['summary'] ?? '',
      prUrl: json['prUrl'] ?? '',
      createdAt: json['createdAt'] != null
          ? DateTime.tryParse(json['createdAt'] as String) ?? DateTime.fromMillisecondsSinceEpoch(0)
          : DateTime.fromMillisecondsSinceEpoch(0),
      subAgents: (json['subAgents'] as List<dynamic>?)
              ?.map((e) => SubAgentInfo.fromJson(e as Map<String, dynamic>))
              .toList() ??
          [],
    );
  }
}

enum AgentState {
  working,
  waitingForInput,
  idle,
  starting,
  error,
  offline,
}
