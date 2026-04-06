package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

const version = "0.1.0"

const banner = `
   ___         __  _                        _ __       
  / _ | ___   / /_(_)__ _______ __   __(_) /_ __  __
 / __ |/ _ \ / __/ / _ ` + "`" + `/ __/ _ ` + "`" + `/ / / / / __/ // /
/_/ |_/_//_/\__/_/\_, /_/  \_,_/\_,_/_/\__/\_, / 
  🌀 WhatsApp    /___/                    /___/  v%s

  AI-powered coding assistant in your pocket.
  Powered by Google Gemini • Built with Go

`

func main() {
	// ── CLI Flags ──────────────────────────────
	model := flag.String("model", "", "Gemini model override (default: gemma-4-31b-it)")
	workspace := flag.String("workspace", "", "Workspace directory for file operations")
	logLevel := flag.String("log-level", "", "Log level: debug, info, warn, error")
	showVersion := flag.Bool("version", false, "Show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("antigravity-whatsapp v%s\n", version)
		os.Exit(0)
	}

	// ── Banner ─────────────────────────────────
	fmt.Printf(banner, version)

	// ── Config ─────────────────────────────────
	cfg := LoadConfig()

	// CLI flags override env vars
	if *model != "" {
		cfg.Model = *model
	}
	if *workspace != "" {
		cfg.Workspace = *workspace
	}
	if *logLevel != "" {
		cfg.LogLevel = *logLevel
	}

	// Resolve workspace to absolute path
	absWorkspace, err := filepath.Abs(cfg.Workspace)
	if err != nil {
		slog.Error("failed to resolve workspace path", "error", err)
		os.Exit(1)
	}
	cfg.Workspace = absWorkspace

	// Validate
	if err := cfg.Validate(); err != nil {
		slog.Error("configuration error", "error", err)
		fmt.Println("\n💡 Tip: Copy .env.example to .env and fill in your GEMINI_API_KEY")
		os.Exit(1)
	}

	// ── Logging ────────────────────────────────
	setupLogging(cfg.LogLevel)

	slog.Info("configuration loaded",
		"model", cfg.Model,
		"workspace", cfg.Workspace,
		"allowedNumbers", len(cfg.AllowedNumbers),
	)

	// ── Agent ──────────────────────────────────
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	agent, err := NewAgent(ctx, cfg)
	if err != nil {
		slog.Error("failed to create agent", "error", err)
		os.Exit(1)
	}
	defer agent.Close()

	fmt.Printf("  🤖 Model: %s\n", cfg.Model)
	fmt.Printf("  📁 Workspace: %s\n", cfg.Workspace)
	if len(cfg.AllowedNumbers) > 0 {
		fmt.Printf("  🔒 Allowed numbers: %d configured\n", len(cfg.AllowedNumbers))
	} else {
		fmt.Printf("  🔓 Allowed numbers: all (no allowlist set)\n")
	}
	fmt.Println()

	// ── WhatsApp ───────────────────────────────
	wa, err := NewWhatsApp(ctx, cfg, agent)
	if err != nil {
		slog.Error("failed to create WhatsApp client", "error", err)
		os.Exit(1)
	}

	if err := wa.Start(ctx); err != nil {
		slog.Error("failed to start WhatsApp", "error", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("  ────────────────────────────────────")
	fmt.Println("  🌀 Antigravity is live on WhatsApp!")
	fmt.Println("  ────────────────────────────────────")
	fmt.Println("  Send a message to get started.")
	fmt.Println("  Press Ctrl+C to stop.")
	fmt.Println()

	// ── Graceful Shutdown ──────────────────────
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	slog.Info("shutting down", "signal", sig)

	fmt.Println("\n👋 Antigravity signing off. See you next time! 🌀")

	wa.Stop()
	agent.Close()
}

func setupLogging(level string) {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	})
	slog.SetDefault(slog.New(handler))
}
