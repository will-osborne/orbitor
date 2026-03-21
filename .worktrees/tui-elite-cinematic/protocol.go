package main

import (
	"encoding/json"
	"time"
)

// --- JSON-RPC 2.0 ---

type JSONRPCMessage struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *JSONRPCError    `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func newRequest(id int, method string, params any) ([]byte, error) {
	raw, err := json.Marshal(id)
	if err != nil {
		return nil, err
	}
	idRaw := json.RawMessage(raw)
	p, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      &idRaw,
		Method:  method,
		Params:  p,
	})
}

func newResponse(id *json.RawMessage, result any) ([]byte, error) {
	r, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return json.Marshal(JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      id,
		Result:  r,
	})
}

func newErrorResponse(id *json.RawMessage, code int, message string) ([]byte, error) {
	return json.Marshal(JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &JSONRPCError{Code: code, Message: message},
	})
}

// --- ACP Types ---

type InitializeParams struct {
	ProtocolVersion    int                `json:"protocolVersion"`
	ClientCapabilities ClientCapabilities `json:"clientCapabilities"`
	ClientInfo         ClientInfo         `json:"clientInfo"`
}

type ClientCapabilities struct {
	FS       *FSCapabilities `json:"fs,omitempty"`
	Terminal bool            `json:"terminal,omitempty"`
}

type FSCapabilities struct {
	ReadTextFile  bool `json:"readTextFile"`
	WriteTextFile bool `json:"writeTextFile"`
}

type ClientInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Version string `json:"version"`
}

type InitializeResult struct {
	ProtocolVersion   int             `json:"protocolVersion"`
	AgentCapabilities json.RawMessage `json:"agentCapabilities,omitempty"`
	AgentInfo         json.RawMessage `json:"agentInfo,omitempty"`
	AuthMethods       json.RawMessage `json:"authMethods,omitempty"`
}

type SessionNewParams struct {
	CWD        string `json:"cwd"`
	MCPServers []any  `json:"mcpServers"`
}

type SessionNewResult struct {
	SessionID string `json:"sessionId"`
}

type PromptContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type SessionPromptParams struct {
	SessionID string          `json:"sessionId"`
	Prompt    []PromptContent `json:"prompt"`
}

type SessionPromptResult struct {
	StopReason string `json:"stopReason"`
}

type SessionUpdateParams struct {
	SessionID string          `json:"sessionId"`
	Update    json.RawMessage `json:"update"`
}

type SessionUpdate struct {
	SessionUpdate string `json:"sessionUpdate"`
}

type AgentMessageChunk struct {
	SessionUpdate string        `json:"sessionUpdate"`
	Content       PromptContent `json:"content"`
}

// ToolCallFlat is the flat structure sent by ACP for both "tool_call" and
// "tool_call_update" session updates. All fields live at the top level.
type ToolCallFlat struct {
	SessionUpdate string          `json:"sessionUpdate"`
	ToolCallID    string          `json:"toolCallId"`
	Title         string          `json:"title,omitempty"`
	Kind          string          `json:"kind,omitempty"`
	Status        string          `json:"status,omitempty"`
	Content       json.RawMessage `json:"content,omitempty"`
	RawInput      json.RawMessage `json:"rawInput,omitempty"`
	RawOutput     json.RawMessage `json:"rawOutput,omitempty"`
}

type ToolCallResultUpdate struct {
	SessionUpdate string          `json:"sessionUpdate"`
	ToolCallID    string          `json:"toolCallId"`
	Result        json.RawMessage `json:"result,omitempty"`
}

type PermissionRequestParams struct {
	SessionID string             `json:"sessionId"`
	ToolCall  PermissionToolCall `json:"toolCall"`
	Options   []PermissionOption `json:"options"`
}

type PermissionToolCall struct {
	ToolCallID string          `json:"toolCallId"`
	Title      string          `json:"title,omitempty"`
	Kind       string          `json:"kind,omitempty"`
	Status     string          `json:"status,omitempty"`
	RawInput   json.RawMessage `json:"rawInput,omitempty"`
}

type PermissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

type PermissionOutcome struct {
	Outcome struct {
		Outcome  string `json:"outcome"`
		OptionID string `json:"optionId,omitempty"`
	} `json:"outcome"`
}

type FSReadParams struct {
	SessionID string `json:"sessionId"`
	Path      string `json:"path"`
	Line      int    `json:"line,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type FSWriteParams struct {
	SessionID string `json:"sessionId"`
	Path      string `json:"path"`
	Content   string `json:"content"`
}

type TerminalCreateParams struct {
	SessionID string   `json:"sessionId"`
	Command   string   `json:"command"`
	Args      []string `json:"args,omitempty"`
	CWD       string   `json:"cwd,omitempty"`
}

type TerminalOutputParams struct {
	SessionID  string `json:"sessionId"`
	TerminalID string `json:"terminalId"`
}

type TerminalWaitParams struct {
	SessionID  string `json:"sessionId"`
	TerminalID string `json:"terminalId"`
}

type TerminalKillParams struct {
	SessionID  string `json:"sessionId"`
	TerminalID string `json:"terminalId"`
}

type TerminalReleaseParams struct {
	SessionID  string `json:"sessionId"`
	TerminalID string `json:"terminalId"`
}

// --- WebSocket Protocol ---

type WSMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

type WSHistoryMessage struct {
	Type     string      `json:"type"`
	Messages []WSMessage `json:"messages"`
}

type WSPrompt struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type WSPermissionResponse struct {
	Type      string `json:"type"`
	RequestID string `json:"requestId"`
	OptionID  string `json:"optionId"`
}

type WSAgentText struct {
	Text string `json:"text"`
}

type WSToolCall struct {
	ToolCallID string `json:"toolCallId"`
	Title      string `json:"title"`
	Kind       string `json:"kind"`
	Status     string `json:"status"`
	Content    string `json:"content,omitempty"`
}

type WSToolResult struct {
	ToolCallID string `json:"toolCallId"`
	Content    string `json:"content"`
}

type WSPermissionRequest struct {
	RequestID string             `json:"requestId"`
	Title     string             `json:"title"`
	Kind      string             `json:"kind"`
	Command   string             `json:"command,omitempty"` // extracted from rawInput for shell commands
	Options   []PermissionOption `json:"options"`
}

type WSError struct {
	Message string `json:"message"`
}

type WSSessionInfo struct {
	ID                string    `json:"id"`
	WorkingDir        string    `json:"workingDir"`
	ACPSession        string    `json:"acpSessionId"`
	Status            string    `json:"status"`
	Backend           string    `json:"backend"`
	Model             string    `json:"model,omitempty"`
	SkipPermissions   bool      `json:"skipPermissions"`
	PlanMode          bool      `json:"planMode"`
	LastMessage       string    `json:"lastMessage,omitempty"`
	CurrentTool       string    `json:"currentTool,omitempty"`
	CurrentPrompt     string    `json:"currentPrompt,omitempty"`
	IsRunning         bool      `json:"isRunning"`
	QueueDepth        int       `json:"queueDepth"`
	PendingPermission bool      `json:"pendingPermission"`
	Title             string    `json:"title,omitempty"`
	Summary           string    `json:"summary,omitempty"`
	PRURL             string    `json:"prUrl,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
}
