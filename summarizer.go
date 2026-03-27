package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	llamafileVersion = "0.9.3"
	// Default model: Qwen2.5-1.5B-Instruct Q4_K_M (~1 GB, one-time download).
	defaultModelURL  = "https://huggingface.co/Qwen/Qwen2.5-1.5B-Instruct-GGUF/resolve/main/qwen2.5-1.5b-instruct-q4_k_m.gguf"
	defaultModelFile = "qwen2.5-1.5b-instruct-q4_k_m.gguf"
)

// SummarizerConfig controls how the LLM summarizer operates.
type SummarizerConfig struct {
	// OllamaURL makes the summarizer use an existing Ollama instance instead
	// of the built-in llamafile approach. Set to e.g. "http://localhost:11434".
	OllamaURL   string
	OllamaModel string // Ollama model name; default "qwen2.5:1.5b"

	// CacheDir is where the llamafile binary and GGUF model are stored.
	// Default: ~/.cache/copilot-bridge
	CacheDir string

	// ModelURL overrides the GGUF model download URL.
	ModelURL string
}

// Summarizer manages a local LLM server to generate session titles and
// summaries. It downloads and launches a llamafile subprocess on first use,
// caching both the binary and the model in CacheDir. All operations are
// best-effort: Summarize returns ("", "") on any error.
type Summarizer struct {
	cfg      SummarizerConfig
	pingHTTP *http.Client // short timeout for health checks
	callHTTP *http.Client // longer timeout for inference calls

	mu      sync.Mutex
	proc    *exec.Cmd
	apiBase string // URL of the running server, empty until started
}

// NewSummarizer creates a Summarizer from cfg.
func NewSummarizer(cfg SummarizerConfig) *Summarizer {
	if cfg.CacheDir == "" {
		home, _ := os.UserHomeDir()
		cfg.CacheDir = filepath.Join(home, ".cache", "orbitor")
	}
	if cfg.ModelURL == "" {
		cfg.ModelURL = defaultModelURL
	}
	if cfg.OllamaModel == "" {
		cfg.OllamaModel = "qwen2.5:1.5b"
	}
	return &Summarizer{
		cfg:      cfg,
		pingHTTP: &http.Client{Timeout: 2 * time.Second},
		callHTTP: &http.Client{Timeout: 60 * time.Second},
	}
}

// Stop kills the llamafile subprocess if one is running. Called on app shutdown.
func (s *Summarizer) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.proc != nil && s.proc.Process != nil {
		_ = s.proc.Process.Kill()
		_ = s.proc.Wait()
		s.proc = nil
		s.apiBase = ""
		log.Printf("summarizer: stopped llamafile server")
	}
}

// Summarize generates a title and one-sentence summary from the session's
// message history. Returns ("", "") on any error.
func (s *Summarizer) Summarize(messages []WSMessage) (title, summary string) {
	ctx := buildSummaryContext(messages)
	if ctx == "" {
		return "", ""
	}

	apiBase, model, err := s.ensureServer()
	if err != nil {
		log.Printf("summarizer: server unavailable: %v", err)
		return "", ""
	}

	prompt := "You are summarizing an AI coding assistant session. " +
		"Given the conversation below, respond with a JSON object containing exactly two fields:\n" +
		"- \"title\": a short title of 4-7 words describing the main task\n" +
		"- \"summary\": one sentence describing what was accomplished or attempted\n" +
		"Respond ONLY with the JSON object, no other text.\n\nConversation:\n" + ctx

	return s.callLLM(apiBase, model, prompt)
}

func (s *Summarizer) callLLM(apiBase, model, prompt string) (title, summary string) {
	type chatMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	reqBody, _ := json.Marshal(map[string]any{
		"model":           model,
		"messages":        []chatMessage{{Role: "user", Content: prompt}},
		"temperature":     0.1,
		"max_tokens":      200,
		"response_format": map[string]string{"type": "json_object"},
	})

	resp, err := s.callHTTP.Post(apiBase+"/v1/chat/completions", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || len(result.Choices) == 0 {
		return "", ""
	}

	content := result.Choices[0].Message.Content

	// First try direct JSON unmarshal (best case: model returned exact JSON)
	var out struct {
		Title   string `json:"title"`
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(content), &out); err == nil {
		return strings.TrimSpace(out.Title), strings.TrimSpace(out.Summary)
	}

	// If direct unmarshal failed, try to be tolerant: strip markdown fences and extract JSON object
	orig := content
	// If content contains a markdown code fence, extract inner block
	if i := strings.Index(orig, "```"); i != -1 {
		j := strings.Index(orig[i+3:], "```")
		if j != -1 {
			inner := orig[i+3 : i+3+j]
			// If fence has a language tag like ```json\n{...}, trim first line if it looks like a tag
			if k := strings.Index(inner, "\n"); k != -1 {
				maybe := strings.TrimSpace(inner[k+1:])
				if strings.HasPrefix(strings.TrimSpace(inner[:k]), "json") || strings.HasPrefix(strings.TrimSpace(inner[:k]), "json\n") {
					inner = maybe
				}
			}
			content = strings.TrimSpace(inner)
		}
	}

	// If still not JSON, attempt to find first '{' .. last '}' and parse substring
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		// fallback: find JSON-like substring
		s := strings.Index(content, "{")
		e := strings.LastIndex(content, "}")
		if s != -1 && e != -1 && e > s {
			sub := content[s : e+1]
			if err2 := json.Unmarshal([]byte(sub), &out); err2 == nil {
				return strings.TrimSpace(out.Title), strings.TrimSpace(out.Summary)
			}
		}
		// As a last-ditch attempt, try extracting from the original response (in case fences removed useful chars)
		s = strings.Index(orig, "{")
		e = strings.LastIndex(orig, "}")
		if s != -1 && e != -1 && e > s {
			sub := orig[s : e+1]
			if err3 := json.Unmarshal([]byte(sub), &out); err3 == nil {
				return strings.TrimSpace(out.Title), strings.TrimSpace(out.Summary)
			}
		}

		// Give up and log for diagnostics
		log.Printf("summarizer: could not parse LLM output as JSON; sample: %q", shortenForLog(orig, 400))
		return "", ""
	}

	return strings.TrimSpace(out.Title), strings.TrimSpace(out.Summary)
}

// shortenForLog returns a shortened preview of s up to maxLen with ellipsis.
func shortenForLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}

// ensureServer returns the API base URL and model name for a running LLM server,
// starting one if necessary.
func (s *Summarizer) ensureServer() (apiBase, model string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Ollama mode: just verify it's reachable, no process management needed.
	if s.cfg.OllamaURL != "" {
		if pingErr := s.ping(s.cfg.OllamaURL); pingErr != nil {
			return "", "", fmt.Errorf("ollama not reachable at %s: %w", s.cfg.OllamaURL, pingErr)
		}
		return s.cfg.OllamaURL, s.cfg.OllamaModel, nil
	}

	// llamafile mode: reuse existing server if still healthy.
	if s.apiBase != "" && s.proc != nil && s.proc.ProcessState == nil {
		if s.ping(s.apiBase) == nil {
			return s.apiBase, "local", nil
		}
	}
	// Process dead or unreachable — clean up.
	if s.proc != nil {
		_ = s.proc.Process.Kill()
		_ = s.proc.Wait()
		s.proc = nil
		s.apiBase = ""
	}

	binPath, err := s.ensureBinary()
	if err != nil {
		return "", "", fmt.Errorf("binary: %w", err)
	}
	modelPath, err := s.ensureModel()
	if err != nil {
		return "", "", fmt.Errorf("model: %w", err)
	}

	// Find a free port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", "", fmt.Errorf("find port: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	newBase := fmt.Sprintf("http://127.0.0.1:%d", port)

	// Spawn the llamafile server.
	// On macOS, run via /bin/sh so the APE polyglot binary self-extracts
	// correctly without triggering Gatekeeper on the extracted native binary.
	serverArgs := []string{
		binPath,
		"-m", modelPath,
		"--server",
		"--host", "127.0.0.1",
		"--port", fmt.Sprintf("%d", port),
		"--nobrowser",
		"-c", "2048",
		"-t", fmt.Sprintf("%d", llmThreads()),
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("/bin/sh", serverArgs...)
	} else {
		cmd = exec.Command(serverArgs[0], serverArgs[1:]...)
	}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return "", "", fmt.Errorf("start llamafile: %w", err)
	}
	s.proc = cmd
	s.apiBase = newBase
	log.Printf("summarizer: loading model on port %d (may take up to 30s on first run) ...", port)

	// Poll until ready (up to 120 seconds for slow hardware / large models).
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		if cmd.ProcessState != nil {
			s.proc = nil
			s.apiBase = ""
			return "", "", fmt.Errorf("llamafile exited unexpectedly during startup")
		}
		if s.ping(newBase) == nil {
			log.Printf("summarizer: llamafile ready")
			return newBase, "local", nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	s.proc = nil
	s.apiBase = ""
	return "", "", fmt.Errorf("llamafile did not become ready within 120s")
}

func (s *Summarizer) ping(baseURL string) error {
	resp, err := s.pingHTTP.Get(baseURL + "/health")
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

// SummarizeText calls the LLM summarizer with a provided context string and
// returns a title and one-sentence summary. Returns empty strings on error.
func (s *Summarizer) SummarizeText(contextText string) (title, summary string) {
	if strings.TrimSpace(contextText) == "" {
		return "", ""
	}
	apiBase, model, err := s.ensureServer()
	if err != nil {
		log.Printf("summarizer: server unavailable for SummarizeText: %v", err)
		return "", ""
	}
	prompt := "You are summarizing an AI operations dashboard. Given the items below, respond with a JSON object containing exactly two fields:\n" +
		"- \"title\": a short title of 4-7 words describing the overall status\n" +
		"- \"summary\": one sentence describing what is happening and any important actions\n" +
		"Respond ONLY with the JSON object, no other text.\n\nItems:\n" + contextText
	return s.callLLM(apiBase, model, prompt)
}

// ensureBinary returns the path to the llamafile binary, downloading if needed.
func (s *Summarizer) ensureBinary() (string, error) {
	if err := os.MkdirAll(s.cfg.CacheDir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(s.cfg.CacheDir, "llamafile")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	url := fmt.Sprintf(
		"https://github.com/Mozilla-Ocho/llamafile/releases/download/%s/llamafile-%s",
		llamafileVersion, llamafileVersion,
	)
	log.Printf("summarizer: downloading llamafile binary to %s ...", path)
	if err := downloadFile(url, path); err != nil {
		return "", fmt.Errorf("download binary: %w", err)
	}
	if err := os.Chmod(path, 0755); err != nil {
		return "", err
	}
	// Remove macOS quarantine so /bin/sh can execute it without a Gatekeeper prompt.
	if runtime.GOOS == "darwin" {
		_ = exec.Command("xattr", "-d", "com.apple.quarantine", path).Run()
	}
	log.Printf("summarizer: llamafile binary ready")
	return path, nil
}

// ensureModel returns the path to the GGUF model file, downloading if needed.
func (s *Summarizer) ensureModel() (string, error) {
	path := filepath.Join(s.cfg.CacheDir, defaultModelFile)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	log.Printf("summarizer: downloading LLM model (~1 GB, one-time) to %s ...", path)
	if err := downloadFile(s.cfg.ModelURL, path); err != nil {
		return "", fmt.Errorf("download model: %w", err)
	}
	log.Printf("summarizer: model download complete")
	return path, nil
}

// downloadFile downloads url to destPath atomically (writes to a temp file first).
func downloadFile(url, destPath string) error {
	tmp := destPath + ".download"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		f.Close()
		if cleanup {
			os.Remove(tmp)
		}
	}()

	// No timeout: downloads can be >1 GB.
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	cleanup = false
	return os.Rename(tmp, destPath)
}

func llmThreads() int {
	n := runtime.NumCPU()
	if n > 8 {
		return 8
	}
	if n < 2 {
		return 2
	}
	return n
}

// EnhancePrompt rewrites a rough prompt into a clearer, more detailed coding
// instruction. Returns the original text unchanged on any error.
func (s *Summarizer) EnhancePrompt(text string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	apiBase, model, err := s.ensureServer()
	if err != nil {
		return text
	}
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	prompt := "You are helping a developer write a clear coding instruction for an AI coding assistant.\n" +
		"Rewrite the following rough note into a precise, actionable instruction. " +
		"Keep the same intent but add helpful specificity. " +
		"Respond with only the improved instruction text, no preamble.\n\nInput: " + text
	reqBody, _ := json.Marshal(map[string]any{
		"model":       model,
		"messages":    []msg{{Role: "user", Content: prompt}},
		"temperature": 0.3,
		"max_tokens":  300,
	})
	resp, err := s.callHTTP.Post(apiBase+"/v1/chat/completions", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return text
	}
	defer resp.Body.Close()
	var result struct {
		Choices []struct {
			Message struct{ Content string `json:"content"` } `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || len(result.Choices) == 0 {
		return text
	}
	enhanced := strings.TrimSpace(result.Choices[0].Message.Content)
	if enhanced == "" {
		return text
	}
	return enhanced
}

// Debrief generates a structured post-run summary. Returns empty string on error.
func (s *Summarizer) Debrief(messages []WSMessage) string {
	ctx := buildSummaryContext(messages)
	if ctx == "" {
		return ""
	}
	apiBase, model, err := s.ensureServer()
	if err != nil {
		return ""
	}
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	prompt := "You are summarizing a completed AI coding session. " +
		"Given the conversation below, write a brief debrief with three short sections:\n" +
		"1. What was asked (one sentence)\n" +
		"2. What was done (1-3 bullet points starting with •)\n" +
		"3. Open questions or next steps (one sentence, or 'None' if complete)\n" +
		"Be concise. No markdown headers, just plain text.\n\nConversation:\n" + ctx
	reqBody, _ := json.Marshal(map[string]any{
		"model":       model,
		"messages":    []msg{{Role: "user", Content: prompt}},
		"temperature": 0.2,
		"max_tokens":  250,
	})
	resp, err := s.callHTTP.Post(apiBase+"/v1/chat/completions", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var result struct {
		Choices []struct {
			Message struct{ Content string `json:"content"` } `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || len(result.Choices) == 0 {
		return ""
	}
	return strings.TrimSpace(result.Choices[0].Message.Content)
}

// Suggestions generates up to 3 natural follow-up prompts. Returns nil on error.
func (s *Summarizer) Suggestions(messages []WSMessage) []string {
	ctx := buildSummaryContext(messages)
	if ctx == "" {
		return nil
	}
	apiBase, model, err := s.ensureServer()
	if err != nil {
		return nil
	}
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	prompt := "Based on this AI coding session, suggest exactly 3 short follow-up prompts the developer might send next. " +
		"Respond with a JSON array of 3 strings, each under 10 words. " +
		"Respond ONLY with the JSON array, e.g. [\"Add unit tests\",\"Fix lint errors\",\"Update docs\"].\n\nConversation:\n" + ctx
	reqBody, _ := json.Marshal(map[string]any{
		"model":       model,
		"messages":    []msg{{Role: "user", Content: prompt}},
		"temperature": 0.4,
		"max_tokens":  120,
	})
	resp, err := s.callHTTP.Post(apiBase+"/v1/chat/completions", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var result struct {
		Choices []struct {
			Message struct{ Content string `json:"content"` } `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || len(result.Choices) == 0 {
		return nil
	}
	content := strings.TrimSpace(result.Choices[0].Message.Content)
	var suggestions []string
	if err := json.Unmarshal([]byte(content), &suggestions); err == nil {
		return clampSuggestions(suggestions)
	}
	var wrapped struct{ Suggestions []string `json:"suggestions"` }
	if err := json.Unmarshal([]byte(content), &wrapped); err == nil && len(wrapped.Suggestions) > 0 {
		return clampSuggestions(wrapped.Suggestions)
	}
	if i := strings.Index(content, "["); i != -1 {
		if j := strings.LastIndex(content, "]"); j > i {
			if err := json.Unmarshal([]byte(content[i:j+1]), &suggestions); err == nil {
				return clampSuggestions(suggestions)
			}
		}
	}
	return nil
}

func clampSuggestions(s []string) []string {
	if len(s) > 3 {
		return s[:3]
	}
	return s
}

// buildSummaryContext extracts user prompts and agent responses from message
// history into a compact text string suitable for the LLM prompt.
func buildSummaryContext(messages []WSMessage) string {
	const maxChars = 3000
	const maxMessages = 60

	start := 0
	if len(messages) > maxMessages {
		start = len(messages) - maxMessages
	}

	var sb strings.Builder
	for _, msg := range messages[start:] {
		var line string
		switch msg.Type {
		case "prompt_sent":
			var p struct {
				Text string `json:"text"`
			}
			if json.Unmarshal(msg.Data, &p) == nil && p.Text != "" {
				line = "User: " + truncateStr(p.Text, 500) + "\n"
			}
		case "agent_text":
			var p WSAgentText
			if json.Unmarshal(msg.Data, &p) == nil && p.Text != "" {
				line = "Assistant: " + truncateStr(p.Text, 500) + "\n"
			}
		}
		if line != "" {
			if sb.Len()+len(line) > maxChars {
				break
			}
			sb.WriteString(line)
		}
	}
	return sb.String()
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
