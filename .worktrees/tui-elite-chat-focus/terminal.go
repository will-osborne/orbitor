package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
)

// TerminalManager handles terminal/* ACP requests from the agent.
type TerminalManager struct {
	mu        sync.Mutex
	terminals map[string]*Terminal
	nextID    int
}

type Terminal struct {
	ID     string
	cmd    *exec.Cmd
	output bytes.Buffer
	mu     sync.Mutex
	done   chan struct{}
	exited bool
	code   int
}

func NewTerminalManager() *TerminalManager {
	return &TerminalManager{
		terminals: make(map[string]*Terminal),
	}
}

func (tm *TerminalManager) Create(params json.RawMessage) (any, error) {
	var p TerminalCreateParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}

	tm.mu.Lock()
	tm.nextID++
	id := fmt.Sprintf("term_%d", tm.nextID)
	tm.mu.Unlock()

	args := p.Args
	cmd := exec.Command(p.Command, args...)
	if p.CWD != "" {
		cmd.Dir = p.CWD
	}

	t := &Terminal{ID: id, cmd: cmd, done: make(chan struct{})}
	cmd.Stdout = &t.output
	cmd.Stderr = &t.output

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start command: %w", err)
	}

	go func() {
		err := cmd.Wait()
		t.mu.Lock()
		t.exited = true
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				t.code = exitErr.ExitCode()
			} else {
				t.code = -1
			}
		}
		t.mu.Unlock()
		close(t.done)
	}()

	tm.mu.Lock()
	tm.terminals[id] = t
	tm.mu.Unlock()

	return map[string]string{"terminalId": id}, nil
}

func (tm *TerminalManager) Output(params json.RawMessage) (any, error) {
	var p TerminalOutputParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}

	tm.mu.Lock()
	t, ok := tm.terminals[p.TerminalID]
	tm.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("terminal %s not found", p.TerminalID)
	}

	t.mu.Lock()
	out := t.output.String()
	t.output.Reset()
	t.mu.Unlock()

	return map[string]string{"output": out}, nil
}

func (tm *TerminalManager) WaitForExit(params json.RawMessage) (any, error) {
	var p TerminalWaitParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}

	tm.mu.Lock()
	t, ok := tm.terminals[p.TerminalID]
	tm.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("terminal %s not found", p.TerminalID)
	}

	<-t.done

	t.mu.Lock()
	code := t.code
	out := t.output.String()
	t.mu.Unlock()

	return map[string]any{"exitCode": code, "output": out}, nil
}

func (tm *TerminalManager) Kill(params json.RawMessage) (any, error) {
	var p TerminalKillParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}

	tm.mu.Lock()
	t, ok := tm.terminals[p.TerminalID]
	tm.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("terminal %s not found", p.TerminalID)
	}

	if t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
	}
	return struct{}{}, nil
}

func (tm *TerminalManager) Release(params json.RawMessage) (any, error) {
	var p TerminalReleaseParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}

	tm.mu.Lock()
	t, ok := tm.terminals[p.TerminalID]
	if ok {
		delete(tm.terminals, p.TerminalID)
	}
	tm.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("terminal %s not found", p.TerminalID)
	}

	if t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
	}
	return struct{}{}, nil
}
