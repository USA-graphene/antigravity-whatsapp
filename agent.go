package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// Agent wraps the Gemini API and provides function-calling AI.
type Agent struct {
	client   *genai.Client
	model    *genai.GenerativeModel
	config   *Config
	sessions *SessionManager
}

// ──────────────────────────────────────────────
// System Prompt
// ──────────────────────────────────────────────

func systemPrompt(workspace string) string {
	return fmt.Sprintf(`You are **Antigravity** 🌀 — a powerful AI coding assistant, available over WhatsApp.

You are running on the user's local machine and have direct access to their filesystem and terminal.

## Your Tools
You have these tools to interact with the user's computer:

- **read_file** — Read contents of a file
- **write_file** — Create or modify files
- **list_directory** — Browse directory contents
- **run_command** — Execute shell commands (with safety limits)
- **search_web** — Search the web for information

## Workspace
All file operations are sandboxed to: %s

## Guidelines
1. Be concise — WhatsApp messages should be easy to read on a phone screen.
2. Use emojis strategically for scannability (✅ ❌ 📄 🔧 💡 🚀).
3. Format code with backtick blocks.
4. When running commands, briefly explain what you're doing and why.
5. For multi-step tasks, break them into numbered steps.
6. If a request is ambiguous, ask for clarification.
7. Never run destructive commands (rm -rf /, etc.) without explicit confirmation.
8. If a tool call fails, explain the error and suggest a fix.
9. Keep responses under 4000 characters when possible.
10. You can chain multiple tool calls to accomplish complex tasks.

## Commands
Users can send these special commands:
- **/reset** — Clear conversation history
- **/status** — Show system status
- **/help** — Show available commands
- **/model** — Show current model info

## Personality
You are helpful, direct, and technically excellent. You have a slight sense of humor.
Think of yourself as a brilliant engineer friend who happens to live in the user's phone. 🌀`, workspace)
}

// ──────────────────────────────────────────────
// Tool Declarations (for Gemini function calling)
// ──────────────────────────────────────────────

func toolDeclarations() []*genai.Tool {
	return []*genai.Tool{
		{
			FunctionDeclarations: []*genai.FunctionDeclaration{
				{
					Name:        "read_file",
					Description: "Read the contents of a file from the filesystem. Returns the file content, size, and last modified time.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"path": {
								Type:        genai.TypeString,
								Description: "File path (relative to workspace or absolute)",
							},
						},
						Required: []string{"path"},
					},
				},
				{
					Name:        "write_file",
					Description: "Create or overwrite a file with the given content. Parent directories are created automatically.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"path": {
								Type:        genai.TypeString,
								Description: "File path (relative to workspace or absolute)",
							},
							"content": {
								Type:        genai.TypeString,
								Description: "Content to write to the file",
							},
						},
						Required: []string{"path", "content"},
					},
				},
				{
					Name:        "list_directory",
					Description: "List all files and subdirectories in a directory. Shows names, sizes, and types.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"path": {
								Type:        genai.TypeString,
								Description: "Directory path (relative to workspace or absolute). Defaults to workspace root.",
							},
						},
					},
				},
				{
					Name:        "run_command",
					Description: "Execute a shell command in the workspace directory. Returns stdout/stderr and exit code. Commands have a timeout and dangerous patterns are blocked.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"command": {
								Type:        genai.TypeString,
								Description: "Shell command to execute (run via sh -c)",
							},
						},
						Required: []string{"command"},
					},
				},
				{
					Name:        "search_web",
					Description: "Search the web for information on a topic.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"query": {
								Type:        genai.TypeString,
								Description: "Search query",
							},
						},
						Required: []string{"query"},
					},
				},
			},
		},
	}
}

// ──────────────────────────────────────────────
// Agent Lifecycle
// ──────────────────────────────────────────────

// NewAgent creates a Gemini-powered agent with function calling.
func NewAgent(ctx context.Context, cfg *Config) (*Agent, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(cfg.GeminiAPIKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	model := client.GenerativeModel(cfg.Model)
	model.Tools = toolDeclarations()
	model.SystemInstruction = genai.NewUserContent(genai.Text(systemPrompt(cfg.Workspace)))
	model.SetTemperature(0.7)
	model.SetTopP(0.95)

	// Safety settings — allow most content since this is a coding tool
	model.SafetySettings = []*genai.SafetySetting{
		{Category: genai.HarmCategoryHarassment, Threshold: genai.HarmBlockOnlyHigh},
		{Category: genai.HarmCategoryHateSpeech, Threshold: genai.HarmBlockOnlyHigh},
		{Category: genai.HarmCategoryDangerousContent, Threshold: genai.HarmBlockOnlyHigh},
		{Category: genai.HarmCategorySexuallyExplicit, Threshold: genai.HarmBlockOnlyHigh},
	}

	return &Agent{
		client:   client,
		model:    model,
		config:   cfg,
		sessions: NewSessionManager(cfg.MaxHistory),
	}, nil
}

// Close shuts down the Gemini client.
func (a *Agent) Close() {
	if a.client != nil {
		a.client.Close()
	}
}

// ──────────────────────────────────────────────
// Message Processing
// ──────────────────────────────────────────────

// ProcessMessage handles a user message, executing tool calls as needed,
// and returns the final text response.
func (a *Agent) ProcessMessage(ctx context.Context, senderID, message string) (string, error) {
	// Handle special commands
	if resp, handled := a.handleCommand(senderID, message); handled {
		return resp, nil
	}

	// Get or create chat session
	session := a.sessions.Get(senderID)
	session.AddMessage("user", message)

	// Create a fresh Gemini chat session with history
	chat := a.model.StartChat()
	chat.History = a.buildHistory(session)

	// Send the user message
	resp, err := chat.SendMessage(ctx, genai.Text(message))
	if err != nil {
		return "", fmt.Errorf("gemini error: %w", err)
	}

	// Function calling loop (max 10 iterations to prevent infinite loops)
	for i := 0; i < 10; i++ {
		funcCalls := extractFunctionCalls(resp)
		if len(funcCalls) == 0 {
			break // No more tool calls — we have the final response
		}

		slog.Info("executing tools", "count", len(funcCalls), "sender", senderID)

		// Execute each function call and collect responses
		var funcResponses []genai.Part
		for _, fc := range funcCalls {
			slog.Info("tool call", "name", fc.Name, "args", fc.Args)
			result := a.executeTool(fc.Name, fc.Args)
			funcResponses = append(funcResponses, genai.FunctionResponse{
				Name:     fc.Name,
				Response: result,
			})
		}

		// Send tool results back to model
		resp, err = chat.SendMessage(ctx, funcResponses...)
		if err != nil {
			return "", fmt.Errorf("gemini error after tool execution: %w", err)
		}
	}

	// Extract the final text response
	text := extractText(resp)
	if text == "" {
		text = "🤔 I processed your request but didn't generate a text response. Could you rephrase?"
	}

	// Save assistant response to history
	session.AddMessage("model", text)
	session.TrimHistory(a.config.MaxHistory)

	return text, nil
}

// ──────────────────────────────────────────────
// Special Commands
// ──────────────────────────────────────────────

func (a *Agent) handleCommand(senderID, message string) (string, bool) {
	cmd := strings.TrimSpace(strings.ToLower(message))

	switch cmd {
	case "/reset", "/clear", "/new":
		a.sessions.Reset(senderID)
		return "🔄 Conversation reset. Fresh start!\n\nI'm Antigravity 🌀 — your AI coding assistant. How can I help?", true

	case "/help":
		return `🌀 *Antigravity WhatsApp* — Commands

📋 */help* — Show this message
🔄 */reset* — Clear conversation history
📊 */status* — System info
🤖 */model* — Current model info

💡 *What I can do:*
• Read & write files on your machine
• Run shell commands
• Answer coding questions
• Debug code
• Search the web
• Help with project architecture

Just type naturally — I'm here to help! 🚀`, true

	case "/status":
		return fmt.Sprintf(`📊 *Antigravity Status*

🤖 Model: %s
📁 Workspace: %s
🔒 Allowed numbers: %d
⏱️ Command timeout: %ds
📝 Max history: %d messages

✅ All systems operational 🌀`, a.config.Model, a.config.Workspace, len(a.config.AllowedNumbers), a.config.CmdTimeout, a.config.MaxHistory), true

	case "/model":
		return fmt.Sprintf("🤖 Currently using: *%s*\n\nPowered by Google Gemini API 🌀", a.config.Model), true
	}

	return "", false
}

// ──────────────────────────────────────────────
// Tool Execution
// ──────────────────────────────────────────────

func (a *Agent) executeTool(name string, args map[string]any) map[string]any {
	switch name {
	case "read_file":
		return toolReadFile(a.config.Workspace, args)
	case "write_file":
		return toolWriteFile(a.config.Workspace, args)
	case "list_directory":
		return toolListDirectory(a.config.Workspace, args)
	case "run_command":
		return toolRunCommand(a.config.Workspace, a.config.CmdTimeout, args)
	case "search_web":
		return toolSearchWeb(args)
	default:
		return toolError(fmt.Sprintf("unknown tool: %s", name))
	}
}

// ──────────────────────────────────────────────
// Response Extraction
// ──────────────────────────────────────────────

func extractFunctionCalls(resp *genai.GenerateContentResponse) []genai.FunctionCall {
	var calls []genai.FunctionCall
	if resp == nil || len(resp.Candidates) == 0 {
		return calls
	}
	for _, part := range resp.Candidates[0].Content.Parts {
		if fc, ok := part.(genai.FunctionCall); ok {
			calls = append(calls, fc)
		}
	}
	return calls
}

func extractText(resp *genai.GenerateContentResponse) string {
	if resp == nil || len(resp.Candidates) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			sb.WriteString(string(text))
		}
	}
	return strings.TrimSpace(sb.String())
}

// buildHistory converts session history to Gemini Content format.
// Excludes the latest user message since it's sent separately.
func (a *Agent) buildHistory(session *Session) []*genai.Content {
	session.mu.Lock()
	defer session.mu.Unlock()

	if len(session.History) <= 1 {
		return nil
	}

	// Exclude the last message (current user message, sent via SendMessage)
	msgs := session.History[:len(session.History)-1]
	var history []*genai.Content
	for _, m := range msgs {
		history = append(history, &genai.Content{
			Role:  m.Role,
			Parts: []genai.Part{genai.Text(m.Content)},
		})
	}
	return history
}
