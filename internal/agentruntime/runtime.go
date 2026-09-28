// Package agentruntime, A2A ajanlarının ortak yürütme döngüsünü sağlar: hafıza
// yükleme, LLM çağrısı, MCP araç çağrıları (tool-calling), hafızaya yazma ve
// sonucun A2A olaylarına (event) dönüştürülmesi.
package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"log/slog"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"a2a-research-crew/internal/llm"
	"a2a-research-crew/internal/memory"
	"a2a-research-crew/internal/tool"
)

// Spec, bir ajanın yürütme davranışını tanımlar.
type Spec struct {
	Name              string
	SystemInstruction string
	Model             string
	Temperature       *float64
	MaxTokens         int
	// MaxToolIterations, art arda yapılabilecek en fazla araç çağrısı turudur.
	MaxToolIterations int
	// MemoryWindow, konuşma geçmişinden yüklenecek en fazla mesaj sayısıdır.
	MemoryWindow int
	// RecallLimit, anlamsal hafızadan çekilecek en fazla kayıt sayısıdır.
	RecallLimit int
	// ArtifactName, üretilen artefaktın insan-okur adıdır.
	ArtifactName string
}

// Deps, Runtime'ın dış bağımlılıklarıdır.
type Deps struct {
	LLM    llm.Provider
	Memory memory.Store
	Tools  tool.Provider
	Logger *slog.Logger
}

// Runtime, a2asrv.AgentExecutor uygulamasıdır.
type Runtime struct {
	spec Spec
	deps Deps
}

var _ a2asrv.AgentExecutor = (*Runtime)(nil)

// New, verilen spec ve bağımlılıklarla bir Runtime oluşturur.
func New(spec Spec, deps Deps) *Runtime {
	if spec.MaxToolIterations <= 0 {
		spec.MaxToolIterations = 4
	}
	if spec.MemoryWindow <= 0 {
		spec.MemoryWindow = 20
	}
	if spec.RecallLimit <= 0 {
		spec.RecallLimit = 3
	}
	if spec.ArtifactName == "" {
		spec.ArtifactName = "output"
	}
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	return &Runtime{spec: spec, deps: deps}
}

// Cancel implements a2asrv.AgentExecutor.
func (r *Runtime) Cancel(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCanceled, nil), nil)
	}
}

// Execute implements a2asrv.AgentExecutor.
func (r *Runtime) Execute(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		if execCtx.StoredTask == nil {
			if !yield(a2a.NewSubmittedTask(execCtx, execCtx.Message), nil) {
				return
			}
		}
		if !yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateWorking, nil), nil) {
			return
		}

		answer, err := r.answer(ctx, execCtx, yield)
		if err != nil {
			if errors.Is(err, errConsumerStopped) {
				return
			}
			r.deps.Logger.Error("agent execution failed", "agent", r.spec.Name, "task_id", execCtx.TaskID, "error", err)
			message := a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart(err.Error()))
			yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateFailed, message), nil)
			return
		}

		artifactEvent := a2a.NewArtifactEvent(execCtx, a2a.NewTextPart(answer))
		artifactEvent.Artifact.Name = r.spec.ArtifactName
		if !yield(artifactEvent, nil) {
			return
		}
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCompleted, nil), nil)
	}
}

// answer, LLM + araç döngüsünü çalıştırır ve nihai metni döner.
func (r *Runtime) answer(ctx context.Context, execCtx *a2asrv.ExecutorContext, emit func(a2a.Event, error) bool) (string, error) {
	contextID := execCtx.ContextID
	namespace := memoryNamespace(r.spec.Name, execCtx.User)

	userText := messageText(execCtx.Message)
	if err := r.deps.Memory.Append(ctx, contextID, llm.Message{Role: llm.RoleUser, Content: userText}); err != nil {
		return "", fmt.Errorf("append user message: %w", err)
	}

	system := r.spec.SystemInstruction
	recalled, err := r.deps.Memory.Recall(ctx, namespace, userText, r.spec.RecallLimit)
	if err != nil {
		return "", fmt.Errorf("recall memory: %w", err)
	}
	if len(recalled) > 0 {
		system += "\n\nRelevant knowledge from previous sessions:\n"
		for _, mem := range recalled {
			system += "- " + mem.Content + "\n"
		}
	}

	messages, err := r.deps.Memory.Recent(ctx, contextID, r.spec.MemoryWindow)
	if err != nil {
		return "", fmt.Errorf("load memory: %w", err)
	}

	tools := []llm.ToolDef(nil)
	if r.deps.Tools != nil {
		tools = r.deps.Tools.ToolDefs()
	}

	for i := 0; i < r.spec.MaxToolIterations; i++ {
		resp, err := r.deps.LLM.Complete(ctx, llm.Request{
			Model:       r.spec.Model,
			System:      system,
			Messages:    messages,
			Tools:       tools,
			Temperature: r.spec.Temperature,
			MaxTokens:   r.spec.MaxTokens,
		})
		if err != nil {
			return "", fmt.Errorf("llm completion: %w", err)
		}

		if len(resp.ToolCalls) == 0 {
			final := llm.Message{Role: llm.RoleAssistant, Content: resp.Content}
			if err := r.deps.Memory.Append(ctx, contextID, final); err != nil {
				return "", fmt.Errorf("append assistant message: %w", err)
			}
			if _, err := r.deps.Memory.Remember(ctx, namespace, resp.Content, map[string]any{
				"agent":      r.spec.Name,
				"task_id":    string(execCtx.TaskID),
				"context_id": contextID,
			}); err != nil {
				return "", fmt.Errorf("remember answer: %w", err)
			}
			return resp.Content, nil
		}

		assistant := llm.Message{Role: llm.RoleAssistant, Content: resp.Content, ToolCalls: resp.ToolCalls}
		messages = append(messages, assistant)
		if err := r.deps.Memory.Append(ctx, contextID, assistant); err != nil {
			return "", fmt.Errorf("append tool-call message: %w", err)
		}

		for _, call := range resp.ToolCalls {
			result, err := r.callTool(ctx, call)
			if err != nil {
				return "", err
			}
			toolMessage := llm.Message{
				Role:       llm.RoleTool,
				Content:    result.Text,
				ToolCallID: call.ID,
				Name:       call.Name,
			}
			messages = append(messages, toolMessage)
			if err := r.deps.Memory.Append(ctx, contextID, toolMessage); err != nil {
				return "", fmt.Errorf("append tool result: %w", err)
			}

			status := a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart(fmt.Sprintf("called tool %q", call.Name)))
			if !emit(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateWorking, status), nil) {
				return "", errConsumerStopped
			}
		}
	}

	return "", fmt.Errorf("exceeded %d tool iterations", r.spec.MaxToolIterations)
}

func (r *Runtime) callTool(ctx context.Context, call llm.ToolCall) (tool.Result, error) {
	if r.deps.Tools == nil {
		return tool.Result{Text: "no tools available", IsError: true}, nil
	}
	args := map[string]any{}
	if len(call.Arguments) > 0 {
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return tool.Result{Text: fmt.Sprintf("invalid tool arguments: %v", err), IsError: true}, nil
		}
	}
	result, err := r.deps.Tools.CallTool(ctx, call.Name, args)
	if err != nil {
		return tool.Result{Text: fmt.Sprintf("tool call failed: %v", err), IsError: true}, nil
	}
	return result, nil
}

// errConsumerStopped, olay tüketicisi akışı durdurduğunda kullanılır.
var errConsumerStopped = fmt.Errorf("event consumer stopped")

// messageText, bir A2A mesajındaki metin parçalarını birleştirir.
func messageText(msg *a2a.Message) string {
	if msg == nil {
		return ""
	}
	var out string
	for _, part := range msg.Parts {
		if text := part.Text(); text != "" {
			out += text
		}
	}
	return out
}

// memoryNamespace, uzun vadeli hafıza için ajan + kullanıcı ad alanını üretir.
func memoryNamespace(agentName string, user *a2asrv.User) string {
	username := "anonymous"
	if user != nil && user.Name != "" {
		username = user.Name
	}
	return "agent:" + agentName + ":user:" + username
}
