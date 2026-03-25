package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

// ACPClient is a bidirectional JSON-RPC 2.0 client using NDJSON framing.
// The transport is abstract: TCP for Copilot, stdio pipes for Claude.
type ACPClient struct {
	reader  io.Reader
	writer  io.Writer
	closeFn func() error
	mu      sync.Mutex

	nextID  int
	pending map[int]chan *JSONRPCMessage // outstanding client→agent requests

	// Handlers for agent→client requests and notifications
	onNotification func(method string, params json.RawMessage)
	onRequest      func(method string, id *json.RawMessage, params json.RawMessage)

	done chan struct{}
}

// NewACPClient creates an ACP client over any read/write transport.
// For TCP (Copilot): pass conn, conn, conn.Close
// For stdio (Claude): pass stdout, stdin, combined closer
func NewACPClient(r io.Reader, w io.Writer, closeFn func() error) *ACPClient {
	return &ACPClient{
		reader:  r,
		writer:  w,
		closeFn: closeFn,
		nextID:  1,
		pending: make(map[int]chan *JSONRPCMessage),
		done:    make(chan struct{}),
	}
}

// Start begins reading messages from the transport.
func (c *ACPClient) Start() {
	go c.readLoop()
}

func (c *ACPClient) readLoop() {
	defer close(c.done)
	scanner := bufio.NewScanner(c.reader)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB max line
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		log.Printf("acp <<< %s", string(line))
		var msg JSONRPCMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			log.Printf("acp: invalid json: %v", err)
			continue
		}
		c.dispatch(&msg)
	}
	if err := scanner.Err(); err != nil {
		log.Printf("acp: read error: %v", err)
	}
}

func (c *ACPClient) dispatch(msg *JSONRPCMessage) {
	if msg.ID != nil && msg.Method == "" {
		// Response to one of our requests
		var id int
		if err := json.Unmarshal(*msg.ID, &id); err != nil {
			log.Printf("acp: bad response id: %v", err)
			return
		}
		c.mu.Lock()
		ch, ok := c.pending[id]
		if ok {
			delete(c.pending, id)
		}
		c.mu.Unlock()
		if ok {
			ch <- msg
		}
		return
	}

	if msg.Method != "" && msg.ID != nil {
		// Incoming request from agent (permission, fs, terminal)
		if c.onRequest != nil {
			c.onRequest(msg.Method, msg.ID, msg.Params)
		}
		return
	}

	if msg.Method != "" && msg.ID == nil {
		// Notification from agent
		if c.onNotification != nil {
			c.onNotification(msg.Method, msg.Params)
		}
		return
	}
}

func (c *ACPClient) write(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	log.Printf("acp >>> %s", string(data))
	_, err := c.writer.Write(data)
	return err
}

// Call sends a JSON-RPC request and waits for the response.
func (c *ACPClient) Call(method string, params any) (*JSONRPCMessage, error) {
	return c.CallContext(context.Background(), method, params)
}

// CallContext sends a JSON-RPC request and waits for the response, honouring ctx cancellation.
func (c *ACPClient) CallContext(ctx context.Context, method string, params any) (*JSONRPCMessage, error) {
	c.mu.Lock()
	id := c.nextID
	c.nextID++
	ch := make(chan *JSONRPCMessage, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	data, err := newRequest(id, method, params)
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}
	data = append(data, '\n')

	if err := c.write(data); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("acp: write error: %w", err)
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("acp: rpc error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	case <-c.done:
		return nil, fmt.Errorf("acp: connection closed")
	}
}

// Notify sends a JSON-RPC notification (no response expected).
func (c *ACPClient) Notify(method string, params any) error {
	data, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return c.write(data)
}

// Respond sends a JSON-RPC response to an incoming agent request.
func (c *ACPClient) Respond(id *json.RawMessage, result any) error {
	data, err := newResponse(id, result)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return c.write(data)
}

// RespondError sends an error response to an incoming agent request.
func (c *ACPClient) RespondError(id *json.RawMessage, code int, message string) error {
	data, err := newErrorResponse(id, code, message)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return c.write(data)
}

// Close shuts down the transport.
func (c *ACPClient) Close() error {
	return c.closeFn()
}

// Done returns a channel that is closed when the read loop exits.
func (c *ACPClient) Done() <-chan struct{} {
	return c.done
}

// Initialize performs the ACP handshake.
func (c *ACPClient) Initialize() (*InitializeResult, error) {
	resp, err := c.Call("initialize", InitializeParams{
		ProtocolVersion: 1,
		ClientCapabilities: ClientCapabilities{
			FS:       &FSCapabilities{ReadTextFile: true, WriteTextFile: true},
			Terminal: true,
		},
		ClientInfo: ClientInfo{
			Name:    "copilot-bridge",
			Title:   "Copilot Bridge",
			Version: "0.1.0",
		},
	})
	if err != nil {
		return nil, err
	}
	var result InitializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("acp: bad initialize result: %w", err)
	}
	return &result, nil
}

// SessionNew creates a new ACP session with optional MCP server configurations.
func (c *ACPClient) SessionNew(cwd string, mcpServers []any) (string, error) {
	if mcpServers == nil {
		mcpServers = []any{}
	}
	resp, err := c.Call("session/new", SessionNewParams{CWD: cwd, MCPServers: mcpServers})
	if err != nil {
		return "", err
	}
	var result SessionNewResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("acp: bad session/new result: %w", err)
	}
	return result.SessionID, nil
}

// SessionResume attempts to resume an existing ACP session by its ID.
// Returns the session ID on success; falls back to SessionNew if the agent
// does not support resume or the session no longer exists.
func (c *ACPClient) SessionResume(sessionID, cwd string) (string, error) {
	resp, err := c.Call("session/resume", map[string]any{"sessionId": sessionID, "cwd": cwd})
	if err != nil {
		return "", err
	}
	var result SessionNewResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("acp: bad session/resume result: %w", err)
	}
	return result.SessionID, nil
}

// SessionSetConfigOption sets a config option (e.g. model, mode, reasoning_effort)
// on an existing ACP session via the session/set_config_option method.
func (c *ACPClient) SessionSetConfigOption(sessionID, configID string, value any) error {
	_, err := c.Call("session/set_config_option", map[string]any{
		"sessionId": sessionID,
		"configId":  configID,
		"value":     value,
	})
	return err
}

// SessionPrompt sends a prompt and blocks until the run completes.
func (c *ACPClient) SessionPrompt(sessionID, text string) (*SessionPromptResult, error) {
	return c.SessionPromptContext(context.Background(), sessionID, text)
}

// SessionPromptContext sends a prompt and blocks until the run completes or ctx is cancelled.
func (c *ACPClient) SessionPromptContext(ctx context.Context, sessionID, text string) (*SessionPromptResult, error) {
	resp, err := c.CallContext(ctx, "session/prompt", SessionPromptParams{
		SessionID: sessionID,
		Prompt:    []PromptContent{{Type: "text", Text: text}},
	})
	if err != nil {
		return nil, err
	}
	var result SessionPromptResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("acp: bad prompt result: %w", err)
	}
	return &result, nil
}

// handleFSRead handles fs/read_text_file requests from the agent.
func handleFSRead(params json.RawMessage) (any, error) {
	var p FSReadParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p.Path, err)
	}
	content := string(data)
	if p.Line > 0 || p.Limit > 0 {
		lines := splitLines(content)
		start := 0
		if p.Line > 0 {
			start = p.Line - 1
		}
		if start > len(lines) {
			start = len(lines)
		}
		end := len(lines)
		if p.Limit > 0 && start+p.Limit < end {
			end = start + p.Limit
		}
		content = joinLines(lines[start:end])
	}
	return map[string]string{"content": content}, nil
}

// handleFSWrite handles fs/write_text_file requests from the agent.
func handleFSWrite(params json.RawMessage) (any, error) {
	var p FSWriteParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	if err := os.WriteFile(p.Path, []byte(p.Content), 0644); err != nil {
		return nil, fmt.Errorf("write %s: %w", p.Path, err)
	}
	return struct{}{}, nil
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i+1])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func joinLines(lines []string) string {
	var result string
	for _, l := range lines {
		result += l
	}
	return result
}
