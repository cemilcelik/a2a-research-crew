// Package entrypoint, tüm ajan süreçlerinin paylaştığı komut satırı
// ayrıştırma ve başlangıç loglama yardımcılarını içerir.
package entrypoint

import (
	"flag"
	"log/slog"
	"os"

	"a2a-research-crew/internal/buildinfo"
)

// Options, tüm ajan süreçlerinin ortak komut satırı seçenekleridir.
type Options struct {
	// GRPCPort, ajanlar arası A2A gRPC sunucusunun dinleyeceği porttur (0 = kapalı).
	GRPCPort int
	// RESTPort, HTTP+JSON/REST sunucusunun dinleyeceği porttur (0 = kapalı).
	RESTPort int
	// CardPort, genel AgentCard HTTP sunucusunun dinleyeceği porttur.
	CardPort int
	// LogLevel, slog seviyesini belirler (debug, info, warn, error).
	LogLevel string
}

// Parse, verilen ajan adı için ortak bayrakları ayrıştırır ve seçenekleri döner.
func Parse(agentName string) Options {
	var opts Options
	flag.IntVar(&opts.GRPCPort, "grpc-port", 9000, "A2A gRPC sunucusunun portu")
	flag.IntVar(&opts.CardPort, "card-port", 9001, "AgentCard HTTP sunucusunun portu")
	flag.StringVar(&opts.LogLevel, "log-level", "info", "Log seviyesi (debug|info|warn|error)")
	flag.Parse()
	return opts
}

// ServerDefaults, bir ajan sunucusu için varsayılan portlardır.
type ServerDefaults struct {
	GRPCPort int
	RESTPort int
	CardPort int
}

// ParseServer, varsayılan portları uygulayarak sunucu bayraklarını ayrıştırır.
func ParseServer(agentName string, defaults ServerDefaults) Options {
	var opts Options
	flag.IntVar(&opts.GRPCPort, "grpc-port", defaults.GRPCPort, "A2A gRPC sunucusunun portu (0 = kapalı)")
	flag.IntVar(&opts.RESTPort, "rest-port", defaults.RESTPort, "A2A HTTP+JSON/REST sunucusunun portu (0 = kapalı)")
	flag.IntVar(&opts.CardPort, "card-port", defaults.CardPort, "AgentCard HTTP sunucusunun portu")
	flag.StringVar(&opts.LogLevel, "log-level", "info", "Log seviyesi (debug|info|warn|error)")
	flag.Parse()
	return opts
}

// Logger, seçeneklerdeki log seviyesine göre bir slog.Logger oluşturur.
func Logger(opts Options) *slog.Logger {
	return NewLogger(opts.LogLevel)
}

// NewLogger, verilen seviye için JSON çıktılı bir slog.Logger oluşturur.
func NewLogger(levelName string) *slog.Logger {
	var level slog.Level
	switch levelName {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}

// LogStartup, süreç başlangıcında standart bir bilgi satırı yazar.
func LogStartup(logger *slog.Logger, agentName string) {
	logger.Info("agent process starting",
		"agent", agentName,
		"app_version", buildinfo.Version,
		"a2a_protocol_version", buildinfo.A2AProtocolVersion,
	)
}
