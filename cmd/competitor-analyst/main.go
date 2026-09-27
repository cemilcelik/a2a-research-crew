// Package main, rakip analizi uzmanı ajanı başlatır.
package main

import "a2a-research-crew/internal/entrypoint"

func main() {
	opts := entrypoint.Parse("competitor-analyst")
	logger := entrypoint.Logger(opts)
	entrypoint.LogStartup(logger, "competitor-analyst")

	// TODO(phase-3): gRPC A2A sunucusu, AgentCard, LLM/memory/MCP entegrasyonu
	// burada kurulacak.
	_ = opts
}
