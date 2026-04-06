package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all application configuration.
type Config struct {
	GeminiAPIKey   string
	Model          string
	Workspace      string
	AllowedNumbers []string
	CmdTimeout     int
	MaxChunkLen    int
	MaxHistory     int
	LogLevel       string
	DBPath         string
}

// LoadConfig reads configuration from environment variables and .env file.
func LoadConfig() *Config {
	// Attempt to load .env file (best-effort, not fatal if missing)
	loadDotEnv(".env")

	cfg := &Config{
		GeminiAPIKey:   getEnv("GEMINI_API_KEY", ""),
		Model:          getEnv("ANTIGRAVITY_MODEL", "gemma-4-31b-it"),
		Workspace:      getEnv("ANTIGRAVITY_WORKSPACE", "."),
		AllowedNumbers: splitCSV(getEnv("ANTIGRAVITY_ALLOWED", "")),
		CmdTimeout:     getEnvInt("ANTIGRAVITY_CMD_TIMEOUT", 30),
		MaxChunkLen:    getEnvInt("ANTIGRAVITY_MAX_CHUNK", 4000),
		MaxHistory:     getEnvInt("ANTIGRAVITY_MAX_HISTORY", 50),
		LogLevel:       getEnv("ANTIGRAVITY_LOG_LEVEL", "info"),
		DBPath:         getEnv("ANTIGRAVITY_DB_PATH", "file:session.db?_foreign_keys=on"),
	}

	return cfg
}

// Validate checks required config values.
func (c *Config) Validate() error {
	if c.GeminiAPIKey == "" {
		return fmt.Errorf("GEMINI_API_KEY is required — get one at https://aistudio.google.com/apikey")
	}
	if c.Workspace == "" {
		c.Workspace = "."
	}
	return nil
}

// ──────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	s := os.Getenv(key)
	if s == "" {
		return fallback
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return v
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// loadDotEnv is a minimal .env loader — no external dependency needed.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // .env is optional
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		// Strip surrounding quotes
		val = strings.Trim(val, `"'`)
		// Don't overwrite existing env vars
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}
