# Dağıtım (Docker / Compose)

Bu doküman `docker/` ve `docker-compose.yml` dosyalarının nasıl kullanılacağını
ve üretim önizlemesi için notları açıklar.

## İmaj

Tek, çok aşamalı bir Dockerfile vardır: `docker/Dockerfile`. `BINARY` build
argümanı hangi `cmd/` ikilisinin derleneceğini seçer. İmaj `distroless/static`
üzerinde `nonroot` kullanıcıyla çalışır ve `CGO_ENABLED=0` ile derlenir.

```bash
docker build --build-arg BINARY=orchestrator -f docker/Dockerfile -t a2a-research-crew/orchestrator .
```

## Compose yığını

`docker-compose.yml` şu servisleri içerir:

| Servis | İmaj | Portlar | Not |
|---|---|---|---|
| `postgres` | `pgvector/pgvector:pg16` | 5432 | healthcheck'li |
| `orchestrator` | derlenen imaj | 9100 (REST), 9200 (card) | dış giriş + auth uçları |
| `market-scout` | derlenen imaj | 9101 (gRPC), 9201 (card) | |
| `competitor-analyst` | derlenen imaj | 9102, 9202 | sqlite `/tmp/a2a/sqlite` |
| `report-writer` | derlenen imaj | 9103, 9203 | workspace `/tmp/a2a/workspace` |
| `minio` | `minio/minio` | 9000/9001 | **opsiyonel**, `minio` profili |

Başlatma:

```bash
cp .env.example .env      # JWT_SECRET ve LLM anahtarlarını düzenleyin
docker compose up -d --build
docker compose logs -f orchestrator
```

Servis adları iç DNS üzerinden çözülür; orchestrator uzmanlara
`http://market-scout:9201` gibi adreslerle ulaşır (`AGENT_ADVERTISE_HOST` da
servis adına ayarlıdır).

## Ortam değişkenleri

Tüm ayarlar ortam değişkenleridir; tam liste için `.env.example`. Öne çıkanlar:

- `JWT_SECRET` (>= 32 bayt), `JWT_ACCESS_TTL`, `JWT_REFRESH_TTL`
- `AUTH_ADMIN_USERNAME` / `AUTH_ADMIN_PASSWORD` (başlangıç yöneticisi)
- `LLM_PROVIDER` (`mock` | `openai` | `anthropic` | `gemini` | `openai-compatible`)
- `*_MODEL` ajan bazlı model kimlikleri
- `MCP_WEBSEARCH_URL`, `MCP_SQLITE_DIR`, `MCP_FS_ROOT`

## Üretim önizlemesi notları

- **Sırlar:** `.env` repoya girmez; üretimde secret manager kullanın.
- **Kalıcılık:** `postgres` volume kalıcıdır. `competitor-analyst` ve
  `report-writer` şu an `/tmp` kullanır (önizleme); üretimde bunlara kalıcı
  volume bağlayın.
- **TLS:** Ajanlar arası gRPC şu an `insecure`'dır; üretimde TLS/mTLS gerekir.
- **Ağ:** Dışa açılan tek servis `orchestrator`'dır; uzmanlar yalnızca iç ağda
  olabilir.
- **Ölçekleme:** Ajanlar durumsuz süreçlerdir; görev durumu Postgres'te
  olduğundan yatay ölçeklenebilirler.

## Doğrulama

Doğrulama ana akışta Go toolchain ile yapılır; Docker imajları ortam provası
içindir:

```bash
go build ./... && go vet ./... && go test ./...
docker compose config -q
```
