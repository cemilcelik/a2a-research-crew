// Package main, backend A2A uçlarını tüketen komut satırı istemcisini başlatır.
package main

import "a2a-research-crew/internal/entrypoint"

func main() {
	opts := entrypoint.Parse("cli")
	logger := entrypoint.Logger(opts)
	entrypoint.LogStartup(logger, "cli")

	// TODO(phase-5): login/refresh, mesaj gönderme (streaming), görev listeleme
	// ve artefakt indirme komutları burada kurulacak.
	_ = opts
}
