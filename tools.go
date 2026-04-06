package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ──────────────────────────────────────────────
// Tool: read_file
// ──────────────────────────────────────────────

func toolReadFile(workspace string, args map[string]any) map[string]any {
	path, _ := args["path"].(string)
	if path == "" {
		return toolError("path is required")
	}

	fullPath := resolvePath(workspace, path)
	if !isInsideWorkspace(workspace, fullPath) {
		return toolError("access denied: path is outside workspace")
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return toolError(fmt.Sprintf("file not found: %s", err))
	}

	// Guard against huge files (100 KB limit)
	if info.Size() > 100*1024 {
		return toolError(fmt.Sprintf("file too large: %d bytes (max 100KB). Use run_command with head/tail instead.", info.Size()))
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return toolError(fmt.Sprintf("read error: %s", err))
	}

	return map[string]any{
		"content":  string(data),
		"path":     fullPath,
		"size":     info.Size(),
		"modified": info.ModTime().Format(time.RFC3339),
	}
}

// ──────────────────────────────────────────────
// Tool: write_file
// ──────────────────────────────────────────────

func toolWriteFile(workspace string, args map[string]any) map[string]any {
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)

	if path == "" {
		return toolError("path is required")
	}

	fullPath := resolvePath(workspace, path)
	if !isInsideWorkspace(workspace, fullPath) {
		return toolError("access denied: path is outside workspace")
	}

	// Create parent directories
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return toolError(fmt.Sprintf("failed to create directory: %s", err))
	}

	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return toolError(fmt.Sprintf("write error: %s", err))
	}

	return map[string]any{
		"status": "ok",
		"path":   fullPath,
		"bytes":  len(content),
	}
}

// ──────────────────────────────────────────────
// Tool: list_directory
// ──────────────────────────────────────────────

func toolListDirectory(workspace string, args map[string]any) map[string]any {
	path, _ := args["path"].(string)
	if path == "" {
		path = "."
	}

	fullPath := resolvePath(workspace, path)
	if !isInsideWorkspace(workspace, fullPath) {
		return toolError("access denied: path is outside workspace")
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return toolError(fmt.Sprintf("read dir error: %s", err))
	}

	var items []map[string]any
	for _, e := range entries {
		item := map[string]any{
			"name":  e.Name(),
			"isDir": e.IsDir(),
		}
		if info, err := e.Info(); err == nil {
			item["size"] = info.Size()
			item["modified"] = info.ModTime().Format(time.RFC3339)
		}
		items = append(items, item)
	}

	// Build a readable summary
	var sb strings.Builder
	for _, item := range items {
		prefix := "📄"
		if isDir, ok := item["isDir"].(bool); ok && isDir {
			prefix = "📁"
		}
		fmt.Fprintf(&sb, "%s %s", prefix, item["name"])
		if size, ok := item["size"].(int64); ok && !item["isDir"].(bool) {
			fmt.Fprintf(&sb, " (%s)", humanSize(size))
		}
		sb.WriteString("\n")
	}

	return map[string]any{
		"path":    fullPath,
		"count":   len(items),
		"listing": sb.String(),
	}
}

// ──────────────────────────────────────────────
// Tool: run_command
// ──────────────────────────────────────────────

func toolRunCommand(workspace string, timeout int, args map[string]any) map[string]any {
	command, _ := args["command"].(string)
	if command == "" {
		return toolError("command is required")
	}

	// Safety check — block obviously destructive patterns
	blocked := []string{
		"rm -rf /", "rm -rf /*", "mkfs", "dd if=",
		":(){", "fork bomb", "> /dev/sd", "chmod -R 777 /",
		"curl | sh", "wget | sh", "curl | bash", "wget | bash",
	}
	lower := strings.ToLower(command)
	for _, b := range blocked {
		if strings.Contains(lower, b) {
			return toolError(fmt.Sprintf("🚫 blocked: command contains dangerous pattern '%s'", b))
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = workspace
	cmd.Env = append(os.Environ(), "PAGER=cat")

	output, err := cmd.CombinedOutput()

	result := map[string]any{
		"command": command,
	}

	// Truncate output if too long
	out := string(output)
	if len(out) > 8000 {
		out = out[:4000] + "\n\n... [truncated] ...\n\n" + out[len(out)-4000:]
		result["truncated"] = true
	}
	result["output"] = out

	if ctx.Err() == context.DeadlineExceeded {
		result["error"] = fmt.Sprintf("command timed out after %d seconds", timeout)
		result["exitCode"] = -1
	} else if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result["exitCode"] = exitErr.ExitCode()
		}
		result["error"] = err.Error()
	} else {
		result["exitCode"] = 0
	}

	return result
}

// ──────────────────────────────────────────────
// Tool: search_web
// ──────────────────────────────────────────────

func toolSearchWeb(args map[string]any) map[string]any {
	query, _ := args["query"].(string)
	if query == "" {
		return toolError("query is required")
	}

	// Web search is delegated to the model's built-in knowledge.
	// For a production system, integrate a search API here
	// (Google Custom Search, SerpAPI, Brave Search, etc.)
	return map[string]any{
		"note":  "Web search executed via model knowledge. For real-time results, integrate a search API.",
		"query": query,
	}
}

// ──────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────

func resolvePath(workspace, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(workspace, path))
}

func isInsideWorkspace(workspace, path string) bool {
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	return strings.HasPrefix(absPath, absWorkspace)
}

func toolError(msg string) map[string]any {
	return map[string]any{"error": msg}
}

func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
