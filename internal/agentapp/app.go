// Package agentapp, bir A2A ajanının tüm bağımlılıklarını (LLM, hafıza,
// görev deposu, MCP araçları, kimlik doğrulama) kurar ve gRPC A2A sunucusu ile
// genel AgentCard HTTP sunucusunu başlatır. Bu iskelet tüm uzman ajanlar
// tarafından paylaşılır.
package agentapp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2agrpc/v1"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"

	"a2a-research-crew/internal/agent"
	"a2a-research-crew/internal/agentruntime"
	"a2a-research-crew/internal/auth"
	"a2a-research-crew/internal/config"
	"a2a-research-crew/internal/entrypoint"
	"a2a-research-crew/internal/llm"
	"a2a-research-crew/internal/memory"
	"a2a-research-crew/internal/taskstore"
	"a2a-research-crew/internal/tool"
)

const (
	// defaultEmbeddingModel, gerçek sağlayıcılarda kullanılan gömme modelidir.
	defaultEmbeddingModel = "text-embedding-3-small"
	// shutdownTimeout, zarif kapanış için tanınan süredir.
	shutdownTimeout = 10 * time.Second
)

// Toolset, bir ajanın MCP araçlarını ve kapatma fonksiyonunu taşır.
type Toolset struct {
	Provider tool.Provider
	Close    func() error
}

// ToolsetFactory, ajana özel araçları hazırlar.
type ToolsetFactory func(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*Toolset, error)

// Options, bir ajan sürecini başlatmak için gereken ayarlardır.
type Options struct {
	Spec           agentruntime.Spec
	Description    string
	Version        string
	Skill          agent.Skill
	ListenGRPCPort int
	ListenCardPort int

	// BuildTools, ajana özel MCP araçlarını üretir. nil ise araçsız çalışılır.
	BuildTools ToolsetFactory
}

// Run, ajan sürecini başlatır ve bağlam iptal edilene kadar çalıştırır.
func Run(ctx context.Context, cfg *config.Config, opts Options) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := entrypoint.NewLogger(cfg.LogLevel)

	provider, err := buildLLM(cfg)
	if err != nil {
		return err
	}
	embedder := buildEmbedder(cfg)

	memStore, err := newMemoryStore(ctx, cfg, embedder)
	if err != nil {
		return err
	}
	defer memStore.Close()
	if err := memStore.Migrate(ctx); err != nil {
		return err
	}

	taskStore, err := newTaskStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer taskStore.Close()
	if err := taskStore.Migrate(ctx); err != nil {
		return err
	}

	var tools tool.Provider
	var toolCloser func() error
	if opts.BuildTools != nil {
		toolset, err := opts.BuildTools(ctx, cfg, logger)
		if err != nil {
			return err
		}
		tools = toolset.Provider
		toolCloser = toolset.Close
		defer func() {
			if toolCloser != nil {
				_ = toolCloser()
			}
		}()
	}

	runtime := agentruntime.New(opts.Spec, agentruntime.Deps{
		LLM:    provider,
		Memory: memStore,
		Tools:  tools,
		Logger: logger,
	})

	interceptor, err := buildAuthInterceptor(cfg, logger)
	if err != nil {
		return err
	}

	card := buildCard(cfg, opts, interceptor != nil)

	handlerOptions := []a2asrv.RequestHandlerOption{
		a2asrv.WithTaskStore(taskStore),
		a2asrv.WithLogger(logger),
	}
	if interceptor != nil {
		handlerOptions = append(handlerOptions, a2asrv.WithCallInterceptors(interceptor))
	}
	requestHandler := a2asrv.NewHandler(runtime, handlerOptions...)

	return serve(ctx, logger, requestHandler, card, opts)
}

func serve(ctx context.Context, logger *slog.Logger, handler a2asrv.RequestHandler, card *a2a.AgentCard, opts Options) error {
	grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%d", opts.ListenGRPCPort))
	if err != nil {
		return fmt.Errorf("agentapp: listen grpc: %w", err)
	}

	grpcServer := grpc.NewServer()
	a2agrpc.NewHandler(handler).RegisterWith(grpcServer)

	cardMux := http.NewServeMux()
	cardMux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(card))
	cardServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", opts.ListenCardPort),
		Handler: cardMux,
	}

	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		logger.Info("grpc server listening", "agent", opts.Spec.Name, "addr", grpcListener.Addr().String())
		if err := grpcServer.Serve(grpcListener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			return err
		}
		return nil
	})
	group.Go(func() error {
		logger.Info("agent card server listening", "agent", opts.Spec.Name, "addr", cardServer.Addr)
		if err := cardServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
	group.Go(func() error {
		<-groupCtx.Done()
		logger.Info("shutting down", "agent", opts.Spec.Name)
		grpcServer.GracefulStop()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		_ = cardServer.Shutdown(shutdownCtx)
		return nil
	})

	return group.Wait()
}

func buildCard(cfg *config.Config, opts Options, requireBearer bool) *a2a.AgentCard {
	grpcURL := fmt.Sprintf("%s:%d", cfg.AdvertiseHost, opts.ListenGRPCPort)
	return agent.BuildCard(agent.CardSpec{
		Name:        opts.Spec.Name,
		Description: opts.Description,
		Version:     opts.Version,
		Interfaces: []agent.Interface{
			{URL: grpcURL, Protocol: a2a.TransportProtocolGRPC},
		},
		Skills:        []agent.Skill{opts.Skill},
		Streaming:     true,
		RequireBearer: requireBearer,
	})
}

func buildLLM(cfg *config.Config) (llm.Provider, error) {
	options := llm.Options{Provider: cfg.LLM.Provider}
	switch cfg.LLM.Provider {
	case llm.ProviderOpenAI:
		options.APIKey = cfg.LLM.OpenAIAPIKey
	case llm.ProviderAnthropic:
		options.APIKey = cfg.LLM.AnthropicAPIKey
	case llm.ProviderGemini:
		options.APIKey = cfg.LLM.GeminiAPIKey
	case llm.ProviderOpenAICompatible:
		options.APIKey = cfg.LLM.CompatAPIKey
		options.BaseURL = cfg.LLM.CompatBaseURL
	}
	return llm.New(options)
}

func buildEmbedder(cfg *config.Config) llm.Embedder {
	switch cfg.LLM.Provider {
	case llm.ProviderOpenAI:
		if cfg.LLM.OpenAIAPIKey != "" {
			return llm.NewOpenAIEmbedder(llm.Options{APIKey: cfg.LLM.OpenAIAPIKey}, defaultEmbeddingModel, memory.DefaultEmbeddingDimensions)
		}
	case llm.ProviderOpenAICompatible:
		if cfg.LLM.CompatAPIKey != "" {
			return llm.NewOpenAIEmbedder(llm.Options{APIKey: cfg.LLM.CompatAPIKey, BaseURL: cfg.LLM.CompatBaseURL}, defaultEmbeddingModel, memory.DefaultEmbeddingDimensions)
		}
	}
	return llm.NewMockEmbedder(memory.DefaultEmbeddingDimensions)
}

func newMemoryStore(ctx context.Context, cfg *config.Config, embedder llm.Embedder) (*memory.PostgresStore, error) {
	return memory.NewPostgresStore(ctx, cfg.Postgres.DSN(), embedder)
}

func newTaskStore(ctx context.Context, cfg *config.Config) (*taskstore.Postgres, error) {
	return taskstore.NewPostgres(ctx, cfg.Postgres.DSN(), a2asrv.NewTaskStoreAuthenticator())
}

func buildAuthInterceptor(cfg *config.Config, logger *slog.Logger) (*auth.ServerInterceptor, error) {
	if len(cfg.JWT.Secret) == 0 {
		logger.Warn("JWT secret not configured; authentication is disabled")
		return nil, nil
	}
	manager, err := auth.NewManager(auth.Config{
		Secret:     cfg.JWT.Secret,
		Issuer:     cfg.JWT.Issuer,
		AccessTTL:  cfg.JWT.AccessTTL,
		RefreshTTL: cfg.JWT.RefreshTTL,
	})
	if err != nil {
		return nil, fmt.Errorf("agentapp: build auth manager: %w", err)
	}
	return auth.NewServerInterceptor(manager, true), nil
}
