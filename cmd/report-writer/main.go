// Package main, rapor yazımı uzmanı ajanı başlatır.
package main

import "a2a-research-crew/internal/entrypoint"

func main() {
	opts := entrypoint.Parse("report-writer")
	logger := entrypoint.Logger(opts)
	entrypoint.LogStartup(logger, "report-writer")

	// TODO(phase-3): gRPC A2A sunucusu, AgentCard, LLM/memory/MCP entegrasyonu
	// burada kurulacak.
	_ = opts
}
