// Package workspacefs, report-writer ajanına kısıtlanmış (jail) bir dosya
// sistemi sunan in-process bir MCP sunucusu üretir. Tüm yollar çalışma alanı
// köküne hapsedilir; böylece ajan çalışma alanı dışına yazamaz.
package workspacefs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"a2a-research-crew/internal/config"
	"a2a-research-crew/internal/mcpx"
)

// Araç adları.
const (
	ToolWriteFile = "write_file"
	ToolReadFile  = "read_file"
	ToolListDir   = "list_dir"
)

// maxReadBytes, tek bir okumada döndürülecek en fazla bayt sayısıdır.
const maxReadBytes = 1 << 20 // 1 MiB

// New, çalışma alanı köküne hapsedilmiş bir dosya sistemi MCP istemcisi döner.
func New(ctx context.Context, cfg *config.Config) (*mcpx.Client, func(), error) {
	root, err := filepath.Abs(cfg.MCP.FSRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("workspacefs: resolve root: %w", err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, nil, fmt.Errorf("workspacefs: create workspace: %w", err)
	}

	return mcpx.NewInProcessServer(ctx, "filesystem", []mcpx.InProcessTool{
		{
			Name:        ToolWriteFile,
			Description: "Writes content to a file inside the workspace. Paths are relative to the workspace root and cannot escape it.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": "relative path inside the workspace"},
					"content": map[string]any{"type": "string", "description": "file content"},
				},
				"required": []any{"path", "content"},
			},
			Handler: func(_ context.Context, args map[string]any) (string, error) {
				return writeFile(root, args)
			},
		},
		{
			Name:        ToolReadFile,
			Description: "Reads a file from the workspace.",
			Schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"path": map[string]any{"type": "string"}},
				"required":   []any{"path"},
			},
			Handler: func(_ context.Context, args map[string]any) (string, error) {
				return readFile(root, args)
			},
		},
		{
			Name:        ToolListDir,
			Description: "Lists files and directories inside a workspace directory.",
			Schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"path": map[string]any{"type": "string", "description": "relative directory path (default '.')"}},
			},
			Handler: func(_ context.Context, args map[string]any) (string, error) {
				return listDir(root, args)
			},
		},
	})
}

func writeFile(root string, args map[string]any) (string, error) {
	rel, _ := args["path"].(string)
	content, _ := args["content"].(string)
	if strings.TrimSpace(rel) == "" {
		return "", fmt.Errorf("path is required")
	}
	full, err := resolve(root, rel)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", fmt.Errorf("create parent directory: %w", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(content), rel), nil
}

func readFile(root string, args map[string]any) (string, error) {
	rel, _ := args["path"].(string)
	if strings.TrimSpace(rel) == "" {
		return "", fmt.Errorf("path is required")
	}
	full, err := resolve(root, rel)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", fmt.Errorf("stat file: %w", err)
	}
	if info.Size() > maxReadBytes {
		return "", fmt.Errorf("file too large to read (%d bytes)", info.Size())
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	return string(data), nil
}

func listDir(root string, args map[string]any) (string, error) {
	rel, _ := args["path"].(string)
	if rel == "" {
		rel = "."
	}
	full, err := resolve(root, rel)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(full)
	if err != nil {
		return "", fmt.Errorf("list directory: %w", err)
	}
	var b strings.Builder
	for _, entry := range entries {
		if entry.IsDir() {
			b.WriteString(entry.Name() + "/\n")
			continue
		}
		b.WriteString(entry.Name() + "\n")
	}
	return b.String(), nil
}

// resolve, göreli bir yolu çalışma alanı kökü altına çözer ve kök dışına
// çıkışı engeller.
func resolve(root, rel string) (string, error) {
	// Baştaki "/" ile Clean, ".." bileşenlerini yok eder ve yolu köke sabitler.
	cleaned := filepath.Clean("/" + rel)
	full := filepath.Join(root, cleaned)

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	fullAbs, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	if fullAbs != rootAbs && !strings.HasPrefix(fullAbs, rootAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes workspace: %s", rel)
	}
	return fullAbs, nil
}
