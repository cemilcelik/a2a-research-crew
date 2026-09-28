# Ajanlar

Bu doküman ajan yürütme modelini ve uzman ajanları açıklar.

## Ortak yürütme döngüsü (`internal/agentruntime`)

Her ajan aynı `Runtime` üzerinden çalışır ve `a2asrv.AgentExecutor` sözleşmesini
uygular. Bir istek geldiğinde:

```mermaid
sequenceDiagram
    participant C as İstemci
    participant S as A2A Sunucusu
    participant R as Runtime
    participant M as MCP Sunucusu
    participant L as LLM
    participant DB as Postgres/pgvector

    C->>S: SendMessage (gRPC/REST)
    S->>R: Execute(execCtx)
    R-->>S: Task(submitted)
    R-->>S: status(working)
    R->>DB: kullanıcı mesajını kaydet + ilgili anıları getir
    loop Araç döngüsü (en fazla MaxToolIterations)
        R->>L: Complete(system, mesajlar, araçlar)
        alt Araç çağrısı var
            R->>M: CallTool(name, args)
            R->>DB: assistant + tool mesajlarını kaydet
            R-->>S: status(working)
        else Nihai yanıt
            R->>DB: yanıtı kaydet (kısa + anlamsal hafıza)
        end
    end
    R-->>S: Artifact(yanıt)
    R-->>S: status(completed)
```

- **System instruction** her ajana özgüdür.
- **Memory**: kısa vadeli (bağlam içi mesajlar) ve uzun vadeli (anlamsal,
  pgvector) birlikte kullanılır; geçmiş oturumlardan ilgili kayıtlar sistem
  talimatına eklenir.
- **MCP / tool-calling**: araç tanımları MCP sunucularından keşfedilir; model
  araç çağırdıkça döngü devam eder.
- **Artifact**: nihai yanıt bir A2A artefaktı olarak yayınlanır.

## Sunucu iskeleti (`internal/agentapp`)

`agentapp.Run`, bir ajan süreci için tüm kablolamayı yapar:

1. Yapılandırma + logger.
2. LLM sağlayıcısı (mock/gerçek) ve embedder.
3. PostgreSQL görev deposu (`taskstore`) ve hafıza (`memory`) + şema migration.
4. Agent'a özel MCP araçları (`ToolsetFactory`).
5. JWT doğrulama interceptor'ı (secret varsa).
6. `a2agrpc` A2A sunucusu + genel AgentCard HTTP sunucusu.
7. Sinyalle zarif kapanış.

## market-scout (Phase 2)

- **Skill**: `market_research`.
- **Araç**: `web_search` (MCP). `MCP_WEBSEARCH_URL` boşsa deterministik
  in-process sahte sunucu kullanılır.
- **Artefakt**: `market-brief`.
- **Taşıma**: yalnızca gRPC (ajanlar arası iletişim).
- **Model**: `MARKET_SCOUT_MODEL` (boşsa sağlayıcı varsayılanı).
