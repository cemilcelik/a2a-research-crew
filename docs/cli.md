# CLI

Backend'i tüketen komut satırı istemcisi (`cmd/cli`). Frontend yoktur; tüm
etkileşim CLI üzerindendir.

## Yapılandırma

| Değişken | Varsayılan | Açıklama |
|---|---|---|
| `A2A_SERVER_URL` | `http://127.0.0.1:9200` | Orchestrator'ın AgentCard taban URL'i |
| `A2A_TOKEN_FILE` | `~/.a2a-research-crew/tokens.json` | Token dosyası |
| `A2A_PASSWORD` | — | Login parolası (bayrak verilmezse) |

Ayrıca `-server` ve `-token-file` global bayrakları kullanılabilir.

## Komutlar

```bash
# 1. Giriş yap (JWT al, token dosyasına yaz)
cli -server http://127.0.0.1:9200 login -username admin -password admin-secret

# 2. Orchestrator kartını göster
cli card

# 3. Araştırma isteği gönder ve akışı (SSE) izle
cli send "research the CRM market"

# 4. Görevleri listele / tek görevi incele / artefaktı yazdır
cli tasks
cli task <task-id>
cli artifact <task-id>

# 5. Görevi iptal et
cli cancel <task-id>

# 6. Çıkış
cli logout
```

## İşleyiş

- CLI önce orchestrator'ın **public AgentCard**'ını çözer; aktif taşıma olarak
  karttaki `HTTP+JSON` arayüzünü kullanır.
- **Login**: karttan türetilen REST tabanına `POST /auth/login` yapar; access ve
  refresh token'ları diske (yalnızca sahibinin okuyabildiği izinle) yazar.
- **Otomatik yenileme**: access token sona ermek üzereyse (`refreshMargin`) bir
  sonraki komut sırasında `POST /auth/refresh` ile sessizce yenilenir.
- **Streaming**: `send`, `SendStreamingMessage` (SSE) ile gelen olayları
  (`submitted`/`working`/artefakt/`completed`) canlı yazar ve nihai artefaktı
  toplayıp en sonda basar.
- Kimlik, `Authorization: Bearer` başlığıyla taşınır; orchestrator ve uzmanlar
  aynı JWT mekanizmasıyla doğrular.
