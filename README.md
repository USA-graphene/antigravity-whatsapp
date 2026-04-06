# 🌀 Antigravity-WhatsApp

**AI-powered coding assistant in your pocket.** Talk to a Gemini-powered AI agent through WhatsApp — it can read files, write code, run commands, and more, all on your local machine.

[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat&logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Powered by Gemini](https://img.shields.io/badge/Powered%20by-Gemini-4285F4?logo=google&logoColor=white)](https://ai.google.dev)

---

## ✨ Features

- 🤖 **AI Agent** — Powered by Google Gemini (Gemma 4 31B, Gemini Flash, etc.)
- 💬 **WhatsApp Native** — Works via WhatsApp multi-device (QR code pairing)
- 🔧 **Function Calling** — AI can read/write files, run commands, browse directories
- 🔒 **Sandboxed** — File operations are restricted to your workspace directory
- 🛡️ **Safe Commands** — Destructive shell commands are blocked, with timeouts
- 📱 **Phone Allowlist** — Restrict access to specific phone numbers
- 💾 **Persistent Sessions** — WhatsApp session survives restarts (SQLite)
- 🧠 **Conversation Memory** — Per-chat history with automatic cleanup
- 📦 **Single Binary** — No runtime dependencies, just one Go executable

---

## 🚀 Quick Start

### Prerequisites

- **Go 1.23+** — [Install Go](https://go.dev/dl/)
- **GCC** — Required for SQLite (pre-installed on macOS, `build-essential` on Linux)
- **Gemini API Key** — Free at [Google AI Studio](https://aistudio.google.com/apikey)
- **WhatsApp** — Active WhatsApp account on your phone

### Install & Run

```bash
# Clone
git clone https://github.com/raimis/antigravity-whatsapp.git
cd antigravity-whatsapp

# Configure
cp .env.example .env
# Edit .env and add your GEMINI_API_KEY

# Build & run
go mod tidy
go build -o antigravity-whatsapp .
./antigravity-whatsapp
```

On first run, scan the QR code with WhatsApp → **Linked Devices** → **Link a Device**.

### Or install directly

```bash
go install github.com/raimis/antigravity-whatsapp@latest
```

---

## ⚙️ Configuration

All config is via environment variables or a `.env` file:

| Variable | Default | Description |
|---|---|---|
| `GEMINI_API_KEY` | *(required)* | Google Gemini API key |
| `ANTIGRAVITY_MODEL` | `gemma-4-31b-it` | Model to use |
| `ANTIGRAVITY_WORKSPACE` | `.` | Workspace directory for file ops |
| `ANTIGRAVITY_ALLOWED` | *(empty = all)* | Comma-separated allowed phone numbers (E.164) |
| `ANTIGRAVITY_CMD_TIMEOUT` | `30` | Shell command timeout (seconds) |
| `ANTIGRAVITY_MAX_CHUNK` | `4000` | Max message chunk length |
| `ANTIGRAVITY_MAX_HISTORY` | `50` | Max messages per conversation |
| `ANTIGRAVITY_LOG_LEVEL` | `info` | Log level: debug, info, warn, error |

### CLI Flags

```bash
./antigravity-whatsapp --model gemini-2.0-flash --workspace ~/projects --log-level debug
```

---

## 💬 Usage

Once running, message the paired WhatsApp number:

### Example Conversations

**Read a file:**
> You: *"Show me the contents of main.go"*
>
> Antigravity: *📄 Here's main.go: ...*

**Run a command:**
> You: *"Run the tests in my project"*
>
> Antigravity: *🔧 Running `go test ./...`*
>
> *✅ All 42 tests passed.*

**Write code:**
> You: *"Create a Python script that converts CSV to JSON"*
>
> Antigravity: *📝 Created `csv_to_json.py`: ...*

**Debug:**
> You: *"Why is my server crashing? Check the logs"*
>
> Antigravity: *🔍 Reading server.log... Found the issue: ...*

### Slash Commands

| Command | Description |
|---|---|
| `/help` | Show available commands |
| `/reset` | Clear conversation history |
| `/status` | Show system status |
| `/model` | Show current model info |

---

## 🤖 Supported Models

Any model available via the [Google Gemini API](https://ai.google.dev/gemini-api/docs/models):

| Model | Best For |
|---|---|
| `gemma-4-31b-it` | Strongest open model (default) |
| `gemma-4-26b-it` | Efficient MoE alternative |
| `gemini-2.0-flash` | Fast responses, large context |
| `gemini-1.5-flash` | Budget-friendly |

---

## 🔒 Security

- **Workspace Sandbox** — File tools can only access files within the configured workspace
- **Command Blocklist** — Dangerous patterns (`rm -rf /`, `mkfs`, `dd`, etc.) are blocked
- **Timeouts** — Commands are killed after the configured timeout
- **Phone Allowlist** — Restrict who can send messages to the bot
- **No Cloud** — Everything runs locally on your machine

> ⚠️ **Important:** This tool executes shell commands on your machine. Always configure `ANTIGRAVITY_ALLOWED` in production to restrict access to trusted phone numbers only.

---

## 🏗️ Architecture

```
┌─────────────┐     ┌──────────────┐     ┌───────────┐
│  WhatsApp   │────▶│  Antigravity │────▶│  Gemini   │
│  (Phone)    │◀────│  (Go binary) │◀────│  API      │
└─────────────┘     └──────┬───────┘     └───────────┘
                           │
                    ┌──────┴───────┐
                    │    Tools     │
                    ├──────────────┤
                    │ 📄 read_file  │
                    │ ✏️ write_file │
                    │ 📁 list_dir   │
                    │ 🔧 run_cmd    │
                    │ 🔍 search     │
                    └──────────────┘
```

**Flow:**
1. User sends WhatsApp message
2. whatsmeow receives it via multi-device protocol
3. Message is sent to Gemini with tool declarations
4. Gemini may request tool calls (read file, run command, etc.)
5. Tools execute locally, results sent back to Gemini
6. Gemini produces final response
7. Response sent back to user via WhatsApp

---

## 🛠️ Development

```bash
# Run in watch mode (auto-restart on file changes)
go run .

# Build for current platform
go build -o antigravity-whatsapp .

# Cross-compile
GOOS=linux GOARCH=amd64 go build -o antigravity-whatsapp-linux .
GOOS=darwin GOARCH=arm64 go build -o antigravity-whatsapp-mac .
GOOS=windows GOARCH=amd64 go build -o antigravity-whatsapp.exe .

# Run tests
go test ./...
```

---

## 📜 License

MIT — see [LICENSE](LICENSE).

---

## 🙏 Credits

- [whatsmeow](https://github.com/tulir/whatsmeow) — WhatsApp multi-device Go library
- [Google Generative AI Go SDK](https://github.com/google/generative-ai-go) — Gemini API client
- [Google Gemma 4](https://ai.google.dev/gemma) — Open model powering the default config

---

<p align="center">
  Built with 🌀 by <a href="https://github.com/raimis">Raimis</a>
</p>
