// Package agentapp, bir A2A ajanının tüm bağımlılıklarını (LLM, hafıza,
// görev deposu, MCP araçları, kimlik doğrulama) kurar ve A2A sunucusu ile
// AgentCard HTTP sunucusunu başlatır. Hem gRPC (uzmanlar) hem HTTP+JSON/REST
// (orchestrator) taşımalarını destekler.
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
	"a2a-research-crew/internal/authhttp"
	"a2a-research-crew/internal/config"
	"a2a-research-crew/internal/entrypoint"
	"a2a-research-crew/internal/llm"
	"a2a-research-crew/internal/memory"
	"a2a-research-crew/internal/taskstore"
	"a2a-research-crew/internal/tool"
	"a2a-research-crew/internal/userstore"
)

const (
	// defaultEmbeddingModel, gerçek sağlayıcılarda kullanılan gömme modelidir.
	defaultEmbeddingModel = "text-embedding-3-small"
	// shutdownTimeout, zarif kapanış için tanınan süredir.
	shutdownTimeout = 10 * time.Second
)

// Toolset, bir ajanın araçlarını ve kapatma fonksiyonunu taşır.
type Toolset struct {
	Provider tool.Provider
	Close    func() error
}

// ToolsetFactory, ajana özel araçları hazırlar.
type ToolsetFactory func(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*Toolset, error)

// Options, bir ajan sürecini başlatmak için gereken ayarlardır.
type Options struct {
	Spec        agentruntime.Spec
	Description string
	Version     string
	Skill       agent.Skill

	// GRPCPort, gRPC A2A sunucusunun portudur (0 = kapalı).
	GRPCPort int
	// RESTPort, HTTP+JSON/REST A2A sunucusunun portudur (0 = kapalı).
	RESTPort int
	// CardPort, genel AgentCard HTTP sunucusunun portudur.
	CardPort int

	// BuildTools, ajana özel araçları üretir. nil ise araçsız çalışılır.
	BuildTools ToolsetFactory

	// AuthEndpoints, /auth/login ve /auth/refresh uçlarını açar (REST gerekir).
	AuthEndpoints bool
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
	embedder := NewEmbedder(cfg)

	memStore, err := memory.NewPostgresStore(ctx, cfg.Postgres.DSN(), embedder)
	if err != nil {
		return err
	}
	defer memStore.Close()
	if err := memStore.Migrate(ctx); err != nil {
		return err
	}

	taskStore, err := taskstore.NewPostgres(ctx, cfg.Postgres.DSN(), a2asrv.NewTaskStoreAuthenticator())
	if err != nil {
		return err
	}
	defer taskStore.Close()
	if err := taskStore.Migrate(ctx); err != nil {
		return err
	}

	var tools tool.Provider
	if opts.BuildTools != nil {
		toolset, err := opts.BuildTools(ctx, cfg, logger)
		if err != nil {
			return err
		}
		tools = toolset.Provider
		if toolset.Close != nil {
			defer func() { _ = toolset.Close() }()
		}
	}

	runtime := agentruntime.New(opts.Spec, agentruntime.Deps{
		LLM:    provider,
		Memory: memStore,
		Tools:  tools,
		Logger: logger,
	})

	manager, err := buildAuthManager(cfg, logger)
	if err != nil {
		return err
	}

	handlerOptions := []a2asrv.RequestHandlerOption{
		a2asrv.WithTaskStore(taskStore),
		a2asrv.WithLogger(logger),
	}
	if manager != nil {
		handlerOptions = append(handlerOptions, a2asrv.WithCallInterceptors(auth.NewServerInterceptor(manager, true)))
	}
	requestHandler := a2asrv.NewHandler(runtime, handlerOptions...)

	authHandler, err := buildAuthEndpoints(ctx, cfg, opts, manager, logger)
	if err != nil {
		return err
	}

	card := buildCard(cfg, opts, manager != nil)
	return serve(ctx, logger, requestHandler, card, opts, authHandler)
}

func buildAuthEndpoints(
	ctx context.Context,
	cfg *config.Config,
	opts Options,
	manager *auth.Manager,
	logger *slog.Logger,
) (*authhttp.Handler, error) {
	if !opts.AuthEndpoints {
		return nil, nil
	}
	if manager == nil {
		return nil, fmt.Errorf("agentapp: auth endpoints require a JWT secret")
	}
	if opts.RESTPort == 0 {
		return nil, fmt.Errorf("agentapp: auth endpoints require a REST port")
	}

	users, err := userstore.NewPostgres(ctx, cfg.Postgres.DSN())
	if err != nil {
		return nil, err
	}
	if err := users.Migrate(ctx); err != nil {
		users.Close()
		return nil, err
	}
	if cfg.Auth.SeedEnabled() {
		if err := users.Upsert(ctx, cfg.Auth.AdminUsername, cfg.Auth.AdminPassword, cfg.Auth.AdminRoles); err != nil {
			users.Close()
			return nil, err
		}
		logger.Info("seeded admin user", "username", cfg.Auth.AdminUsername)
	}
	// Kullanıcı deposunun yaşam döngüsü süreçle birdir; kapatma process
	// sonlanırken gerçekleşir.
	return authhttp.NewHandler(manager, users), nil
}

func serve(
	ctx context.Context,
	logger *slog.Logger,
	handler a2asrv.RequestHandler,
	card *a2a.AgentCard,
	opts Options,
	authHandler *authhttp.Handler,
) error {
	group, groupCtx := errgroup.WithContext(ctx)

	var (
		grpcServer   *grpc.Server
		cardServer   *http.Server
		restServer   *http.Server
		grpcListener net.Listener
	)

	if opts.GRPCPort > 0 {
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", opts.GRPCPort))
		if err != nil {
			return fmt.Errorf("agentapp: listen grpc: %w", err)
		}
		grpcListener = listener
		grpcServer = grpc.NewServer()
		a2agrpc.NewHandler(handler).RegisterWith(grpcServer)
		group.Go(func() error {
			logger.Info("grpc server listening", "agent", opts.Spec.Name, "addr", grpcListener.Addr().String())
			if err := grpcServer.Serve(grpcListener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
				return err
			}
			return nil
		})
	}

	cardMux := http.NewServeMux()
	cardMux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(card))
	cardServer = &http.Server{Addr: fmt.Sprintf(":%d", opts.CardPort), Handler: cardMux}
	group.Go(func() error {
		logger.Info("agent card server listening", "agent", opts.Spec.Name, "addr", cardServer.Addr)
		if err := cardServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})

	if opts.RESTPort > 0 {
		restMux := http.NewServeMux()
		restMux.Handle("/", a2asrv.NewRESTHandler(handler))
		if authHandler != nil {
			authHandler.Register(restMux)
		}
		restServer = &http.Server{Addr: fmt.Sprintf(":%d", opts.RESTPort), Handler: restMux}
		group.Go(func() error {
			logger.Info("rest server listening", "agent", opts.Spec.Name, "addr", restServer.Addr)
			if err := restServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		})
	}

	group.Go(func() error {
		<-groupCtx.Done()
		logger.Info("shutting down", "agent", opts.Spec.Name)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if grpcServer != nil {
			grpcServer.GracefulStop()
		}
		_ = cardServer.Shutdown(shutdownCtx)
		if restServer != nil {
			_ = restServer.Shutdown(shutdownCtx)
		}
		return nil
	})

	return group.Wait()
}

func buildCard(cfg *config.Config, opts Options, requireBearer bool) *a2a.AgentCard {
	interfaces := make([]agent.Interface, 0, 2)
	if opts.GRPCPort > 0 {
		interfaces = append(interfaces, agent.Interface{
			URL:      fmt.Sprintf("%s:%d", cfg.AdvertiseHost, opts.GRPCPort),
			Protocol: a2a.TransportProtocolGRPC,
		})
	}
	if opts.RESTPort > 0 {
		interfaces = append(interfaces, agent.Interface{
			URL:      fmt.Sprintf("http://%s:%d", cfg.AdvertiseHost, opts.RESTPort),
			Protocol: a2a.TransportProtocolHTTPJSON,
		})
	}

	return agent.BuildCard(agent.CardSpec{
		Name:          opts.Spec.Name,
		Description:   opts.Description,
		Version:       opts.Version,
		Interfaces:    interfaces,
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

// NewEmbedder, yapılandırmaya göre gerçek ya da mock bir Embedder seçer.
// Araç setleri (ör. memory MCP) aynı boyutta gömme kullanabilmek için bunu
// paylaşır.
func NewEmbedder(cfg *config.Config) llm.Embedder {
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

func buildAuthManager(cfg *config.Config, logger *slog.Logger) (*auth.Manager, error) {
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
	return manager, nil
}
