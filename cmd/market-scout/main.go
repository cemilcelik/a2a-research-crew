// Package main, pazar araştırması uzmanı ajanı başlatır.
package main

import "a2a-research-crew/internal/entrypoint"

func main() {
	opts := entrypoint.Parse("market-scout")
	logger := entrypoint.Logger(opts)
	entrypoint.LogStartup(logger, "market-scout")

	// TODO(phase-2): gRPC A2A sunucusu, AgentCard, LLM/memory/MCP entegrasyonu
	// burada kurulacak.
	_ = opts
}
