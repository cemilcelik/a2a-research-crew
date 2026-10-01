# Geliştirme Kılavuzu

## Gereksinimler

- **Go** 1.26+ (modül `go 1.26.4`).
- **Docker + Docker Compose** (PostgreSQL + pgvector ve önizleme için).
- İsteğe bağlı: gerçek LLM anahtarları (yoksa `mock` sağlayıcı ile çalışılır).

## Make hedefleri

```bash
make fmt vet test build     # ana doğrulama akışı
make test-race              # veri yarışı testleri
make tidy                    # go mod tidy
make run-orchestrator       # tek ajanı host'ta çalıştır
make run-cli                # CLI
make up / make down         # docker compose (önizleme)
make docker-build           # tüm imajları derle
```

## Host üzerinde geliştirme

1. Yalnızca veritabanını başlat:

   ```bash
   cp .env.example .env
   docker compose up -d postgres
   ```

2. `.env` dosyasını ortama yükle (uygulama `.env`'i otomatik okumaz):

   ```bash
   set -a; source .env; set +a
   ```

3. Ajanları ayrı terminallerde çalıştır (mock LLM ile):

   ```bash
   go run ./cmd/market-scout
   go run ./cmd/competitor-analyst
   go run ./cmd/report-writer
   go run ./cmd/orchestrator
   ```

4. CLI ile konuş:

   ```bash
   go run ./cmd/cli -server http://127.0.0.1:9200 login -username admin -password admin-secret
   go run ./cmd/cli send "research the CRM market"
   ```

> Not: Host'ta çalışırken `AGENT_ADVERTISE_HOST=127.0.0.1` olmalı; compose'da
> servis adları kullanılır.

## Test yaklaşımı

- **Birim testleri ağsızdır.** `go test ./...` ile çalışır; LLM çağrıları
  `httptest` ile taklit edilir, MCP sunucuları in-process'tir.
- **Entegrasyon testleri** Postgres ister ve `TEST_POSTGRES_DSN` yoksa atlanır:

  ```bash
  docker compose up -d postgres
  TEST_POSTGRES_DSN="postgres://crew:crew@localhost:5432/crew?sslmode=disable" go test ./...
  ```

- Testlerde gerçek API anahtarı gerekmez: `LLM_PROVIDER=mock`,
  `mcpx.NewInProcessServer`, `memory.NewMockStore`.

## Yeni bir uzman ajan ekleme

1. **Spec + skill + araç seti** yaz: `cmd/<ajan>/main.go` içinde
   `agentapp.Options` doldurun (`agentruntime.Spec`, `agent.Skill`,
   `BuildTools`). Mevcut `cmd/market-scout` şablon olarak kullanılabilir.
2. **Araç sunucusu** gerekiyorsa `internal/` altında `mcpx.NewInProcessServer`
   (veya `mcpx.Connect` ile harici HTTP MCP) tabanlı bir paket ekleyin.
3. **Portları seç** ve `.env.example` ile `docker-compose.yml`'e ekleyin.
4. **Orchestrator'a tanıt**: `cmd/orchestrator/main.go` içindeki
   `agenttool.New([...])` listesine ajanı ve kart URL'sini ekleyin; config'e
   ilgili `*_URL` ortam değişkenini ekleyin.
5. **Test**: araç sunucusu testi + `agentruntime` döngüsü entegrasyon testi.

## Sık karşılaşılan durumlar

- **`JWT secret not configured; authentication is disabled` uyarısı:** `JWT_SECRET`
  boş; geliştirme için normaldir, üretimde zorunludur.
- **`expected N dimensions, not M`:** pgvector kolon boyutu farklı bir embedder
  ile oluşturulmuş; `memory.DefaultEmbeddingDimensions` tek kaynaktır, geliştirme
  DB'sini sıfırlayın (`docker compose down -v`).
- **Kart çözülemiyor:** orchestrator'ın `*_URL` değerleri uzmanların **card**
  portunu göstermelidir (`http://host:920x`), gRPC portunu değil.
- **`.env` okunmuyor:** uygulama yalnızca ortam değişkenlerini okur; `source`
  edin veya compose kullanın.

## Kod konvansiyonları

- Yorumlar ve dokümanlar **Türkçe**; identifier/log/error/commit mesajları
  **İngilizce**.
- Hata yönetimi sessizce yutulmaz; sarmalanır veya yukarı iletilir.
- Yeni dış bağımlılık eklemeden önce mevcut yardımcıların yeterliliği
  değerlendirilir.
