// Package main, backend A2A uçlarını tüketen komut satırı istemcisini başlatır.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"a2a-research-crew/internal/cli"
)

const defaultServerURL = "http://127.0.0.1:9200"

func main() {
	server := flag.String("server", envOr("A2A_SERVER_URL", defaultServerURL), "orchestrator card base URL")
	tokenFile := flag.String("token-file", envOr("A2A_TOKEN_FILE", defaultTokenPath()), "path to the token file")
	flag.Parse()

	app := cli.New(*server, *tokenFile, os.Stdout, os.Stderr)
	if err := app.Run(context.Background(), flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func defaultTokenPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".a2a-tokens.json"
	}
	return filepath.Join(home, ".a2a-research-crew", "tokens.json")
}
