# a2a-research-crew

A2A (Agent-to-Agent) protokolünü uçtan uca öğrenmek için Go ile yazılmış,
**çok ajanlı otomatik pazar ve rakip araştırması** sistemi.

Kullanıcı CLI üzerinden bir şirket/ürün verir. **Orchestrator** isteği planlar,
işi gRPC üzerinden üç uzman ajana devreder ve sonuçları tek bir araştırma
raporu **artefaktı** olarak sentezler. Tüm süreç A2A protokolünün temel
bileşenlerini (AgentCard, Task, Message, Part, Context, Artifact, Streaming)
kullanır.

> Durum: **tamamlandı (Phase 0–5, Phase 7)**. Phase 6 (edge-case
> sağlamlaştırma) öğrenme projesi kapsamında bilinçli olarak atlandı.

## Öne çıkan A2A özellikleri

- **AgentCard** ile ajan keşfi ve taşıma pazarlığı; **JWT** güvenlik şeması.
- **Binding kuralı:** dış istek → HTTP+JSON/REST, ajanlar arası → gRPC.
- **Task** yaşam döngüsü + **Streaming** (SSE), **Artifact** üretimi.
- **Context** ile oturum gruplama; **input-required** ile insan-döngüde akış.
- Her ajan: LLM + system instruction + memory + skill + **MCP** + tool-calling.

## Mimari

```mermaid
flowchart LR
    CLI["CLI"] -->|"REST + JWT (SSE)"| ORCH["orchestrator<br/>(REST + auth uçları)"]
    ORCH -->|"gRPC + JWT"| MKT["market-scout"]
    ORCH -->|"gRPC + JWT"| CMP["competitor-analyst"]
    ORCH -->|"gRPC + JWT"| WRT["report-writer"]
    MKT --> MCPW[["MCP: web-search"]]
    CMP --> MCPS[["MCP: sqlite"]]
    WRT --> MCPF[["MCP: filesystem"]]
    ORCH --> MCPM[["MCP: memory"]]
    ORCH --> PG[("PostgreSQL + pgvector")]
    MKT --> PG
    CMP --> PG
    WRT --> PG
```

## Ajanlar

| Ajan | Rol | Taşıma | MCP araçları | Artefakt |
|---|---|---|---|---|
| `orchestrator` | Planlama, delegasyon, sentez | REST (dış) | memory (`recall_memory`, `remember_note`) | `research-report` |
| `market-scout` | Pazar verisi toplama | gRPC | web-search (`web_search`) | `market-brief` |
| `competitor-analyst` | Rakip analizi | gRPC | sqlite (`run_sql`, `list_tables`) | `competitor-matrix` |
| `report-writer` | Nihai rapor | gRPC | filesystem (`write_file`, `read_file`, `list_dir`) | `final-report` |

Her ajan; LLM entegrasyonu, system instruction, memory (kısa + uzun vadeli),
skill tanımı, MCP kullanımı ve tool-calling yeteneklerine sahiptir.

## Teknoloji kararları

- **Dil:** Go (`go1.26`).
- **A2A framework:** `github.com/a2aproject/a2a-go/v2` (v2.6.0).
- **MCP:** resmi `github.com/modelcontextprotocol/go-sdk`.
- **Kimlik doğrulama:** JWT (HS256, access + refresh; bcrypt kullanıcı deposu).
- **Veritabanı:** PostgreSQL + pgvector (görev durumu, hafıza, kullanıcılar);
  SQL analizi izole sqlite'ta; rapor çalışma alanı jail'de.
- **UI yok:** yalnızca backend + CLI.

## Hızlı başlangıç (Docker Compose)

```bash
cp .env.example .env
# .env içinde JWT_SECRET ve (varsa) LLM anahtarlarını düzenleyin
docker compose up -d --build
```

CLI ile kullanım:

```bash
go run ./cmd/cli -server http://127.0.0.1:9200 \
  login -username admin -password "$AUTH_ADMIN_PASSWORD"
go run ./cmd/cli -server http://127.0.0.1:9200 send "research the CRM market"
go run ./cmd/cli tasks
```

Ayrıntılar: [CLI dokümanı](./docs/cli.md), [dağıtım](./docs/deployment.md).

## Host üzerinde geliştirme

```bash
docker compose up -d postgres
set -a; source .env; set +a
go run ./cmd/orchestrator & go run ./cmd/market-scout & \
  go run ./cmd/competitor-analyst & go run ./cmd/report-writer &
go run ./cmd/cli -server http://127.0.0.1:9200 login -username admin -password "$AUTH_ADMIN_PASSWORD"
```

Gerçek LLM anahtarı yoksa `LLM_PROVIDER=mock` ile tüm akış ağsız çalışır.

## Doğrulama

```bash
make fmt vet test build
# PostgreSQL entegrasyon testleri dahil:
TEST_POSTGRES_DSN="postgres://crew:crew@localhost:5432/crew?sslmode=disable" make test
```

## Proje yapısı

```
cmd/                     ajan ve CLI giriş noktaları
internal/agentapp        ajan sunucu iskeleti (gRPC/REST + auth)
internal/agentruntime    ortak LLM/MCP/memory yürütme döngüsü
internal/agenttool       uzman ajanları A2A aracı olarak sunma
internal/llm             LLM sağlayıcı soyutlaması + embeddings
internal/mcpx            MCP istemci sarmalayıcısı
internal/toolbox         MCP araçlarını birleştirme
internal/sqlitemcp       izole sqlite MCP sunucusu
internal/workspacefs     jail'li dosya sistemi MCP sunucusu
internal/memorymcp       hafıza MCP sunucusu
internal/websearch       web-arama MCP istemcisi
internal/auth            JWT + interceptor'lar
internal/authhttp        /auth/login, /auth/refresh
internal/userstore       bcrypt kullanıcı deposu
internal/taskstore       Postgres görev deposu (OCC)
internal/memory          kısa + uzun vadeli hafıza (pgvector)
internal/cli             komut satırı istemcisi
docs/                    mimari, akışlar, geliştirme, dağıtım, muhakeme günlükleri
```

## Dokümanlar

- [Mimari](./docs/architecture.md)
- [A2A protokol akışları](./docs/protocol-flows.md)
- [Ajan modeli](./docs/agent-model.md)
- [Çekirdek paketler](./docs/core.md)
- [CLI](./docs/cli.md)
- [Geliştirme](./docs/development.md)
- [Dağıtım](./docs/deployment.md)
- [Muhakeme günlükleri](./docs/reasoning) (faz bazlı karar kayıtları)

## Yol haritası

- [x] **Phase 0** — İskelet, bağımlılıklar, tooling, Docker artefaktları
- [x] **Phase 1** — Ortak çekirdek (config, LLM, MCP, JWT, taskstore, memory)
- [x] **Phase 2** — İlk uzman ajan (`market-scout`) uçtan uca
- [x] **Phase 3** — Diğer uzmanlar (`competitor-analyst`, `report-writer`)
- [x] **Phase 4** — Organizatör (REST girişi + gRPC + streaming)
- [x] **Phase 5** — CLI
- [~] **Phase 6** — Kalıcılık ve sağlamlaştırma *(iptal edildi: öğrenme projesi kapsamı)*
- [x] **Phase 7** — Dokümantasyon ve finalizasyon
