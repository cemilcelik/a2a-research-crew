// Package main, dış dünyaya REST arayüzü sunan organizatör ajanını başlatır.
package main

import "a2a-research-crew/internal/entrypoint"

func main() {
	opts := entrypoint.Parse("orchestrator")
	logger := entrypoint.Logger(opts)
	entrypoint.LogStartup(logger, "orchestrator")

	// TODO(phase-4): REST girişi, kimlik doğrulama uçları, gRPC upstream
	// çağrıları ve streaming burada kurulacak.
	_ = opts
}
