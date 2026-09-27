# Ortak Çekirdek (Phase 1)

Bu doküman, tüm ajanların paylaştığı çekirdek paketlerini özetler.

## Paketler

| Paket | Sorumluluk |
|---|---|
| `internal/config` | Ortam değişkenlerinden yapılandırma yükleme ve doğrulama |
| `internal/llm` | Sağlayıcıdan bağımsız LLM arayüzü; mock, OpenAI, Anthropic, Gemini, OpenAI-uyumlu adaptörler; embeddings |
| `internal/mcpx` | MCP istemci sarmalayıcısı (stdio + streamable HTTP) ve in-process sahte MCP sunucusu |
| `internal/auth` | HS256 JWT üretimi/doğrulaması (access + refresh) ve A2A interceptor'ları |
| `internal/taskstore` | A2A görevlerinin PostgreSQL'de saklanması (OCC + kullanıcı izolasyonu) |
| `internal/memory` | Kısa vadeli konuşma hafızası ve pgvector tabanlı anlamsal hafıza |
| `internal/agent` | AgentCard bildirim üretimi |

## LLM sağlayıcı soyutlaması

`llm.Provider` tek bir metodu vardır:

```go
Complete(ctx context.Context, req Request) (Response, error)
```

- **mock**: Ağ gerektirmez; varsayılan echo davranışı veya `NewScripted` ile
  sıralı yanıtlar. Testler ve dry-run için.
- **openai / openai-compatible**: Chat Completions; uyumlu ağ geçitleri için
  `BaseURL` verilebilir (ör. serbest model kimlikleri için).
- **anthropic**: Messages API.
- **gemini**: generateContent API.

Model kimliği serbest bir string'dir; her ajana farklı model atanabilir.

Embedding için `llm.Embedder` (`MockEmbedder`, `OpenAIEmbedder`).

## Dry-run / mock ilkesi

Gerçek API anahtarı gerekmeden doğrulama yapılabilir:

- `LLM_PROVIDER=mock` ile sahte LLM.
- `mcpx.NewFakeServer` ile in-process sahte MCP araçları.
- `memory.NewMockStore` ile veritabanısız hafıza.

## Test

```bash
make fmt vet test            # birim testleri (mock)
TEST_POSTGRES_DSN="postgres://crew:crew@localhost:5432/crew?sslmode=disable" \
  make test                  # PostgreSQL entegrasyon testleri dahil
```
