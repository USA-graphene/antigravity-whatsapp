package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

// WhatsApp manages the WhatsApp multi-device connection.
type WhatsApp struct {
	client *whatsmeow.Client
	agent  *Agent
	config *Config
}

// NewWhatsApp creates a new WhatsApp client backed by SQLite.
func NewWhatsApp(ctx context.Context, cfg *Config, agent *Agent) (*WhatsApp, error) {
	// Set up SQLite store for session persistence
	logger := waLog.Stdout("WA", "WARN", true)
	container, err := sqlstore.New(ctx, "sqlite3", cfg.DBPath, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create session store: %w", err)
	}

	// Get existing device or create new
	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}

	client := whatsmeow.NewClient(deviceStore, logger)

	wa := &WhatsApp{
		client: client,
		agent:  agent,
		config: cfg,
	}

	// Register event handler
	client.AddEventHandler(wa.eventHandler)

	return wa, nil
}

// Start connects to WhatsApp. Shows QR code if not yet paired.
func (wa *WhatsApp) Start(ctx context.Context) error {
	if wa.client.Store.ID == nil {
		// First time — need QR code pairing
		fmt.Println()
		fmt.Println("┌───────────────────────────────────────────┐")
		fmt.Println("│  🌀 Antigravity WhatsApp — QR Pairing     │")
		fmt.Println("│                                           │")
		fmt.Println("│  1. Open WhatsApp on your phone           │")
		fmt.Println("│  2. Tap ⋮ Menu → Linked Devices           │")
		fmt.Println("│  3. Tap 'Link a Device'                   │")
		fmt.Println("│  4. Scan the QR code below                │")
		fmt.Println("└───────────────────────────────────────────┘")
		fmt.Println()

		qrChan, _ := wa.client.GetQRChannel(ctx)
		err := wa.client.Connect()
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}

		for evt := range qrChan {
			switch evt.Event {
			case "code":
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				fmt.Println("\n⏳ Waiting for scan...")
			case "success":
				fmt.Println("\n✅ Paired successfully!")
			case "timeout":
				return fmt.Errorf("QR code timed out — please restart")
			}
		}
	} else {
		// Already paired — just connect
		slog.Info("reconnecting to existing session")
		err := wa.client.Connect()
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
		slog.Info("connected", "jid", wa.client.Store.ID.String())
	}

	return nil
}

// Stop disconnects from WhatsApp.
func (wa *WhatsApp) Stop() {
	if wa.client != nil {
		wa.client.Disconnect()
	}
}

// ──────────────────────────────────────────────
// Event Handling
// ──────────────────────────────────────────────

func (wa *WhatsApp) eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		wa.handleMessage(v)
	case *events.Connected:
		slog.Info("🌀 WhatsApp connected — Antigravity is live!")
	case *events.Disconnected:
		slog.Warn("WhatsApp disconnected")
	case *events.LoggedOut:
		slog.Error("WhatsApp logged out — delete session.db and re-pair")
	}
}

func (wa *WhatsApp) handleMessage(msg *events.Message) {
	// Skip our own messages
	if msg.Info.IsFromMe {
		return
	}

	// Extract text content
	text := extractMessageText(msg)
	if text == "" {
		return // Ignore non-text messages (images, stickers, etc.)
	}

	// Sender info
	sender := msg.Info.Sender
	senderNumber := sender.User

	// Check allowlist
	if !wa.isAllowed(senderNumber) {
		slog.Warn("blocked message from non-allowed number", "sender", senderNumber)
		return
	}

	slog.Info("📩 incoming message",
		"sender", senderNumber,
		"length", len(text),
		"preview", truncate(text, 50),
	)

	// Process in goroutine to not block the event handler
	go wa.processAndReply(sender, text)
}

func (wa *WhatsApp) processAndReply(sender types.JID, text string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Show typing indicator
	wa.sendPresence(sender, true)
	defer wa.sendPresence(sender, false)

	// Run through the AI agent
	response, err := wa.agent.ProcessMessage(ctx, sender.User, text)
	if err != nil {
		slog.Error("agent error", "error", err, "sender", sender.User)
		response = fmt.Sprintf("⚠️ Error processing message: %s\n\nTry again or send /reset to start fresh.", err)
	}

	// Send response (chunked if needed)
	if err := wa.sendText(ctx, sender, response); err != nil {
		slog.Error("failed to send reply", "error", err, "sender", sender.User)
	}

	slog.Info("📤 replied",
		"sender", sender.User,
		"length", len(response),
	)
}

// ──────────────────────────────────────────────
// Message Sending
// ──────────────────────────────────────────────

func (wa *WhatsApp) sendText(ctx context.Context, to types.JID, text string) error {
	chunks := chunkMessage(text, wa.config.MaxChunkLen)

	for i, chunk := range chunks {
		msg := &waE2E.Message{
			Conversation: proto.String(chunk),
		}
		_, err := wa.client.SendMessage(ctx, to, msg)
		if err != nil {
			return fmt.Errorf("send chunk %d/%d failed: %w", i+1, len(chunks), err)
		}

		// Small delay between chunks to maintain order
		if i < len(chunks)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}
	return nil
}

func (wa *WhatsApp) sendPresence(to types.JID, composing bool) {
	ctx := context.Background()
	if composing {
		_ = wa.client.SendChatPresence(ctx, to, types.ChatPresenceComposing, types.ChatPresenceMediaText)
	} else {
		_ = wa.client.SendChatPresence(ctx, to, types.ChatPresencePaused, types.ChatPresenceMediaText)
	}
}

// ──────────────────────────────────────────────
// Authorization
// ──────────────────────────────────────────────

func (wa *WhatsApp) isAllowed(number string) bool {
	// If no allowlist configured, allow all (useful for personal use)
	if len(wa.config.AllowedNumbers) == 0 {
		return true
	}

	for _, allowed := range wa.config.AllowedNumbers {
		// Match with or without + prefix
		cleanAllowed := strings.TrimPrefix(allowed, "+")
		cleanNumber := strings.TrimPrefix(number, "+")
		if cleanAllowed == cleanNumber {
			return true
		}
	}
	return false
}

// ──────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────

func extractMessageText(msg *events.Message) string {
	if msg.Message == nil {
		return ""
	}

	// Plain text
	if conv := msg.Message.GetConversation(); conv != "" {
		return conv
	}

	// Extended text (replies, links, etc.)
	if ext := msg.Message.GetExtendedTextMessage(); ext != nil {
		return ext.GetText()
	}

	// Image/video with caption
	if img := msg.Message.GetImageMessage(); img != nil {
		return img.GetCaption()
	}
	if vid := msg.Message.GetVideoMessage(); vid != nil {
		return vid.GetCaption()
	}
	if doc := msg.Message.GetDocumentMessage(); doc != nil {
		return doc.GetCaption()
	}

	return ""
}

// chunkMessage splits a long message into WhatsApp-friendly chunks.
// It tries to split at newlines or sentence boundaries.
func chunkMessage(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}

	var chunks []string
	remaining := text

	for len(remaining) > maxLen {
		// Find a good split point
		chunk := remaining[:maxLen]
		splitAt := maxLen

		// Try to split at a double newline first
		if idx := strings.LastIndex(chunk, "\n\n"); idx > maxLen/2 {
			splitAt = idx + 2
		} else if idx := strings.LastIndex(chunk, "\n"); idx > maxLen/2 {
			splitAt = idx + 1
		} else if idx := strings.LastIndex(chunk, ". "); idx > maxLen/2 {
			splitAt = idx + 2
		}

		chunks = append(chunks, strings.TrimSpace(remaining[:splitAt]))
		remaining = remaining[splitAt:]
	}

	if trimmed := strings.TrimSpace(remaining); trimmed != "" {
		chunks = append(chunks, trimmed)
	}

	return chunks
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}
