// Package agenttool, uzak A2A uzman ajanlarını yerel bir tool.Provider gibi
// sunar. Orchestrator'ın LLM'i böylece "araç çağırır gibi" diğer ajanlara
// gRPC üzerinden görev devredebilir.
package agenttool

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
	"github.com/a2aproject/a2a-go/v2/a2agrpc/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"a2a-research-crew/internal/auth"
	"a2a-research-crew/internal/llm"
	"a2a-research-crew/internal/tool"
)

// querySchema, tüm ajan araçlarının ortak girdi şemasıdır.
var querySchema = []byte(`{
  "type": "object",
  "properties": {
    "query": {"type": "string", "description": "the task or question to delegate to this agent"}
  },
  "required": ["query"]
}`)

// Agent, araç olarak sunulacak uzak bir A2A ajanıdır.
type Agent struct {
	// Name, araç adıdır (LLM'e bu adla sunulur). Tire yerine alt çizgi kullanın.
	Name string
	// URL, ajanın AgentCard taban URL'sidir.
	URL string
	// Description, modele aracı tanıtan metindir.
	Description string
}

// Registry, uzak ajanları keşfeder, istemcilerini önbelleğe alır ve tool.Provider
// sözleşmesini uygular.
type Registry struct {
	agents  []Agent
	manager *auth.Manager

	mu      sync.Mutex
	clients map[string]*a2aclient.Client
}

var _ tool.Provider = (*Registry)(nil)

// New, verilen ajan tanımlarıyla bir Registry oluşturur. manager nil ise giden
// çağrılara kullanıcının token'ı aynen iletilir.
func New(agents []Agent, manager *auth.Manager) *Registry {
	return &Registry{
		agents:  agents,
		manager: manager,
		clients: make(map[string]*a2aclient.Client),
	}
}

// ToolDefs implements tool.Provider.
func (r *Registry) ToolDefs() []llm.ToolDef {
	defs := make([]llm.ToolDef, 0, len(r.agents))
	for _, agent := range r.agents {
		defs = append(defs, llm.ToolDef{
			Name:        agent.Name,
			Description: agent.Description,
			Parameters:  querySchema,
		})
	}
	return defs
}

// CallTool implements tool.Provider.
func (r *Registry) CallTool(ctx context.Context, name string, args map[string]any) (tool.Result, error) {
	agent, ok := r.agent(name)
	if !ok {
		return tool.Result{}, fmt.Errorf("agenttool: unknown agent %q", name)
	}

	query, _ := args["query"].(string)
	if strings.TrimSpace(query) == "" {
		return tool.Result{Text: "query is required", IsError: true}, nil
	}

	client, err := r.client(ctx, agent)
	if err != nil {
		return tool.Result{Text: fmt.Sprintf("could not connect to %s: %v", name, err), IsError: true}, nil
	}

	message := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(query))
	result, err := client.SendMessage(ctx, &a2a.SendMessageRequest{Message: message})
	if err != nil {
		return tool.Result{Text: fmt.Sprintf("agent %s failed: %v", name, err), IsError: true}, nil
	}

	return tool.Result{Text: resultText(result)}, nil
}

// Close, önbellekteki tüm istemcileri kapatır.
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var firstErr error
	for name, client := range r.clients {
		if err := client.Destroy(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("agenttool: close %s: %w", name, err)
		}
	}
	r.clients = make(map[string]*a2aclient.Client)
	return firstErr
}

func (r *Registry) agent(name string) (Agent, bool) {
	for _, agent := range r.agents {
		if agent.Name == name {
			return agent, true
		}
	}
	return Agent{}, false
}

func (r *Registry) client(ctx context.Context, agent Agent) (*a2aclient.Client, error) {
	r.mu.Lock()
	cached := r.clients[agent.Name]
	r.mu.Unlock()
	if cached != nil {
		return cached, nil
	}

	card, err := agentcard.DefaultResolver.Resolve(ctx, agent.URL)
	if err != nil {
		return nil, fmt.Errorf("resolve card for %s: %w", agent.Name, err)
	}

	client, err := a2aclient.NewFromCard(ctx, card,
		a2agrpc.WithGRPCTransport(grpc.WithTransportCredentials(insecure.NewCredentials())),
		a2aclient.WithCallInterceptors(auth.NewClientInterceptor(r.tokenSource)),
	)
	if err != nil {
		return nil, fmt.Errorf("create client for %s: %w", agent.Name, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if existing := r.clients[agent.Name]; existing != nil {
		_ = client.Destroy()
		return existing, nil
	}
	r.clients[agent.Name] = client
	return client, nil
}

// tokenSource, giden çağrıya eklenecek token'ı belirler: önce gelen isteğin
// token'ını iletir; yoksa orchestrator adına yeni bir servis token'ı üretir.
func (r *Registry) tokenSource(ctx context.Context) (string, error) {
	if token := auth.BearerFromCallContext(ctx); token != "" {
		return token, nil
	}
	if r.manager == nil {
		return "", nil
	}
	pair, err := r.manager.Issue("orchestrator", []string{"service"})
	if err != nil {
		return "", err
	}
	return pair.AccessToken, nil
}

// resultText, bir A2A sonucundan okunabilir metni çıkarır.
func resultText(result a2a.SendMessageResult) string {
	switch value := result.(type) {
	case *a2a.Task:
		return taskText(value)
	case *a2a.Message:
		return messageText(value)
	default:
		return ""
	}
}

func taskText(task *a2a.Task) string {
	if task == nil {
		return ""
	}
	if len(task.Artifacts) > 0 {
		parts := make([]string, 0, len(task.Artifacts))
		for _, artifact := range task.Artifacts {
			if text := messageText(&a2a.Message{Parts: artifact.Parts}); text != "" {
				parts = append(parts, text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n\n")
		}
	}
	if task.Status.Message != nil {
		return messageText(task.Status.Message)
	}
	if task.Status.State != a2a.TaskStateCompleted {
		return fmt.Sprintf("agent did not complete the task (state: %s)", task.Status.State)
	}
	return ""
}

func messageText(message *a2a.Message) string {
	if message == nil {
		return ""
	}
	var out strings.Builder
	for _, part := range message.Parts {
		if text := part.Text(); text != "" {
			out.WriteString(text)
		}
	}
	return out.String()
}

// ErrNoAgents, kayıtlı ajan olmadığında döner.
var ErrNoAgents = errors.New("agenttool: no agents configured")
