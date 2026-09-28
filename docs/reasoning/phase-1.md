# Phase 1 — Muhakeme Günlüğü

Bu doküman, **Phase 1 (ortak çekirdek: config, LLM, MCP, JWT, taskstore,
memory)** görevinde yaptığım inceleme, alternatif değerlendirme, karar ve
doğrulama adımlarının gerekçelerini içerir. Amaç yalnızca "hangi paket yazıldı"yı
değil, **her kararın neden o seçildiğini** ve **nelerin elendiğini** kayıt
altına almaktır. Bölüm 5'te gerçekten karşılaştığım iki arıza, kanıtlarıyla
birlikte açıkça yazılmıştır.

---

## 0. Görevin kapsamı ve genel yaklaşım

Phase 0'da iskelet ve altyapı hazırdı; Phase 1'in hedefi, tüm ajanların
paylaşacağı **ortak çekirdeği** kurmaktı: konfigürasyon, LLM soyutlaması, MCP
istemcisi, JWT kimlik doğrulama, PostgreSQL görev deposu ve pgvector hafızası.

İzlediğim genel strateji:

1. **Ajan mantığından önce çekirdek.** Ajanları yazmadan önce, onların
   üzerinde duracağı sözleşmeleri (arayüzler) sabitlemek istedim; aksi hâlde
   ajan koduna gömülü varsayımlar çekirdeği şekillendirir ve geri dönüşü zor
   kararlar doğardı.
2. **Önce SDK sözleşmesini oku, sonra kendi arayüzünü tasarla.** Özellikle
   `a2asrv/taskstore.Store` ve `a2aclient` interceptor API'lerini birebir
   inceleyip kendi tiplerimi onlara göre kurdum.
3. **Mock/dry-run'ı birinci sınıf vatandaş yap.** Kullanıcı gerçek API
   anahtarlarını hemen vermeyeceğini söyledi; bu yüzden her dış bağımlılığın
   anahtarsız çalışan bir ikizi olmalıydı (`llm.Mock`, `mcpx.NewFakeServer`,
   `memory.MockStore`).
4. **Paketleri bağımlılık yönüne göre katmanla.** Kim kimi import ediyor,
   döngüsüz ve tek yönlü olsun.
5. **Her doğrulamayı kanıta bağla.** Derleme yetmez; provider istek/yanıt
   şekillendirmesi `httptest` ile, veritabanı davranışı Docker Postgres'e karşı
   entegrasyon testleriyle doğrulandı.

Fazın çıktısı 7 feature commit'i + 1 doküman commit'i oldu
(`c8c8947` → `04014b0`).

---

## 1. İlk inceleme

Kod yazmadan önce, sonradan pahalıya patlayacak yanlış varsayımları elemek için
şu sözleşmeleri okudum:

- **`a2asrv/taskstore/api.go`:** `Store` arayüzü (`Create/Update/Get/List`),
  `StoredTask`, `UpdateRequest`, `TaskVersion` ve OCC semantiği. Özellikle
  `TaskVersionMissing = 0` ve `After` metodunun "eksik sürümü daima güncel
  kabul et" davranışı, Postgres implementasyonunu doğrudan belirledi.
- **`a2asrv/taskstore/inmemory.go`:** Referans davranış. `List`'in sıralaması
  (`lastUpdated` azalan), cursor biçimi (base64 `RFC3339Nano_id`) ve
  `historyLength`/`includeArtifacts` kırpma kurallarını **birebir** taklit
  etmeyi seçtim; aksi hâlde aynı arayüzün iki implementasyonu farklı sonuç
  üretir ve testler yanıltıcı olurdu.
- **`a2a/core.go`:** `Task`, `TaskState` (gerçek string değerleri), `Message`,
  `Part`, `ContentParts`. `TaskState.String()`'in `TASK_STATE_*` ürettiğini
  görünce DB kolonuna ham `string(state)` yazmaya karar verdim.
- **MCP Go SDK:** `Server.AddTool`, `ToolHandler`, `NewInMemoryTransports`,
  `CommandTransport`, `StreamableClientTransport`. In-process testte
  "sunucu önce bağlanır, sonra istemci" kuralını buradan öğrendim.
- **`a2aclient/middleware.go`:** `Request` struct'ı, `ServiceParams` (map,
  case-insensitive), `CallInterceptor` (Before/After sırası). İstemci
  interceptor'ının `ServiceParams`'a `authorization` ekleyeceğini buradan
  netleştirdim.
- **`a2asrv/auth.go`:** `User` ve `NewAuthenticatedUser`, `CallContext.User`
  alanı. Sunucu tarafında kullanıcıyı taşımanın doğru yolu buydu.

Bu incelemenin somut karşılığı: `internal/taskstore/postgres.go` içindeki
`Update`, `PrevVersion == TaskVersionMissing` iken sürüm kontrolünü atlıyor —
çünkü referans implementasyon da öyle yapıyor.

---

## 2. Mimari kararlar

### 2.1 Paket sınırları ve bağımlılık yönü

**Karar:** Bağımlılıklar tek yönlü: `config` (yaprağa en yakın) → `llm` →
`mcpx`/`memory`/`taskstore`/`auth` → `agent`. `llm` hiçbir uygulama paketini
import etmez.

**Alternatifler:**

- *(A)* Her şeyi birkaç büyük pakete toplamak (ör. `internal/core`).
- *(B)* İnce, tek sorumluluklu paketler.

**(B)**'yi seçtim çünkü: (i) döngüsel import riskini baştan engeller, (ii) test
edilebilirliği artırır (her paket kendi mock'uyla), (iii) Phase 2'de
`agentruntime`'ın yalnızca `tool.Provider` arayüzüne bağlanmasını mümkün kılar.

### 2.2 LLM sağlayıcı soyutlaması (`internal/llm`)

**Karar:** Tek metotlu bir `Provider` (`Complete`) + sağlayıcıdan bağımsız
`Request`/`Response`/`Message`/`ToolDef`/`ToolCall` tipleri. Model kimliği
**serbest string**.

**Değerlendirdiğim alternatifler:**

- *(A)* Hazır bir SDK (`sashabaranov/go-openai`) kullanmak.
- *(B)* Sağlayıcı başına elle HTTP istemcisi yazmak.

**(B)**'yi seçtim. Gerekçe: kullanıcı "Jev ve Laya gibi modeller" istedi;
bunlar büyük olasılıkla **OpenAI uyumlu bir ağ geçidi** üzerinden sunulacak.
Kendi HTTP katmanımı yazınca `BaseURL` + serbest model kimliği ile her ağ
geçidini tek adaptörden geçirebiliyorum; hazır SDK bu esnekliği kısıtlayabilir
ve kullanılmayan yüzey alanı getirirdi. Ayrıca **tool-calling** için gereken
alanları (OpenAI `tool_calls`, Anthropic `tool_use`/`tool_result`, Gemini
`functionCall`/`functionResponse`) kendim eşlediğim için dönüşüm davranışı
şeffaf ve test edilebilir.

**İkincil karar — streaming:** `Provider`'a `Stream` koymadım; onu ayrı bir
opsiyonel `StreamingProvider` arayüzüne koydum. Gerekçe: arayüzü şimdiden
genişletip her sağlayıcıya yarım yamalak SSE ayrıştırıcı yazmak yerine,
tamamlamayı sağlam kurup akışı ihtiyaç doğunca (Phase 4) eklemek istedim.
Böylece `Provider`'ı tüketen kod bozulmadan genişler.

**Embedding:** Ayrı bir `Embedder` arayüzü (`Dimensions`, `Embed`) ve
`MockEmbedder` + `OpenAIEmbedder`. Mock, metni `sha256` ile deterministik bir
normalize vektöre çevirir; böylece pgvector yolu anahtarsız test edilebilir.

### 2.3 MCP sarmalayıcı (`internal/mcpx`)

**Karar:** Resmi `modelcontextprotocol/go-sdk` üzerine ince bir sarmalayıcı.
Hem stdio (`CommandTransport`) hem streamable HTTP
(`StreamableClientTransport`) desteklenir; araçlar `llm.ToolDef`'e çevrilir.

**Dikkat ettiğim noktalar:**

- **İsim çakışması:** kendi paketim `mcp` olursa SDK ile çakışırdı; hem paket
  adını `mcpx` yaptım hem SDK'yı `sdk` olarak alias'ladım. Bu, okunabilirliği
  belirgin biçimde artırdı.
- **Sahte sunucu:** Test/dry-run için `NewFakeServer`, araçları düz bir stub
  ile değil **gerçek MCP protokolüyle** (in-memory transport) sunar. Böylece
  araç keşfi → çağrı akışı gerçek mekanizmayla test edilir.
- **HTTP başlıkları:** `StreamableClientTransport` doğrudan header almıyor;
  küçük bir `headerRoundTripper` ile `Authorization` enjekte ettim. Bu,
  `websearch` paketinin anahtar iletmesini sağlayan temel oldu.

### 2.4 JWT kimlik doğrulama (`internal/auth`)

**Karar:** HS256 paylaşımlı secret, access + refresh çifti; sunucu tarafında
`a2asrv.CallInterceptor`, istemci tarafında `a2aclient.CallInterceptor`.

**Değerlendirdiğim alternatifler:**

- *(A)* RS256 + JWKS (asimetrik, anahtar rotasyonu kolay, daha karmaşık).
- *(B)* HS256 paylaşımlı secret (basit, tüm süreçler aynı secret'ı bilir).

**(B)**'yi seçtim çünkü tüm ajanlar aynı güven sınırının içinde ve kullanıcı
açıkça HS256 dedi. RS256'nın sunduğu "doğrulayan taraf secret'ı bilmez"
avantajı burada gerçek bir tehdit modeli değil.

**İnce kararlar:**

- **Secret doğrulaması:** Secret boşsa (geliştirme) izin verilir; ama
  **doluysa en az 32 bayt** zorunlu (`ErrWeakSecret`). Gerekçe: `APP_ENV=development`
  durumunda kimlik doğrulamayı kapatmak istedik, fakat "yanlışlıkla zayıf
  secret" hatasını sessizce geçmek istemedik. Bu, `config.Validate`'te de
  ayrıca kontrol edilir.
- **Sunucu interceptor'ı** token'ı `CallContext.ServiceParams()`'tan çeker,
  doğrular ve `callCtx.User = a2asrv.NewAuthenticatedUser(...)` atar. Böylece
  taskstore'un `NewTaskStoreAuthenticator`'ı kullanıcıyı görebilir.
- **İstemci interceptor'ı**, `req.ServiceParams` nil ise önce başlatır; yoksa
  map'e yazmak panikler. Bunu kodda açıkça korudum.

### 2.5 PostgreSQL görev deposu (`internal/taskstore`)

**Karar:** `tasks` tablosu: `id`, `context_id`, `user_id`, `state`,
`status_timestamp`, `version`, `task jsonb`, zaman damgaları. OCC `version`
kolonuyla.

**Değerlendirdiğim alternatifler (veri modeli):**

- *(A)* `Task`'ı normalize edip alt tablolara ayırmak (history, artifacts...).
- *(B)* Tüm `Task`'ı tek `jsonb` kolonda, filtre alanlarını ayrı kolonlarda
  tutmak.

**(B)**'yi seçtim. Gerekçe: A2A `Task`'ı iç içe ve evrilebilir bir yapı;
normalize etmek SDK şeması değiştikçe migration borcu üretir. Filtreleme
gereken alanları (context, state, timestamp, user) ayrı kolonda tutunca hem
basit hem indekslenebilir kalıyor.

**OCC semantiği:** `Update`, `PrevVersion != TaskVersionMissing` ise
`AND version = $n` ile günceller; satır etkilenmezse **önce var mı/yetki var mı**
diye bakar, sonra `ErrConcurrentModification` mı `ErrTaskNotFound` mu
döneceğine karar verir. Bu ayrım önemli: SDK bu iki hatayı farklı işliyor.

**Güvenlik:** Get/Update/List sorguları `user_id` filtresi içerir; başka
kullanıcının görevi **`ErrTaskNotFound` olarak maskelenir** (A2A spec §3.3.2
gereği — varlığı sızdırmamak için). Bunu `TestPostgresOwnershipIsolation` ile
sabitledim.

### 2.6 pgvector hafızası (`internal/memory`)

**Karar:** İki katman: kısa vadeli mesaj geçmişi (`memory_messages`) ve uzun
vadeli anlamsal hafıza (`memory_semantic`, `vector(N)` kolonu). Arayüzler
`MessageStore` + `SemanticStore`.

**Teknik ince nokta — vektörü nasıl yazarım?** pgx'in `vector` tipini tanımak
için ek `pgvector-go/pgx` kaydı gerekir. Bunun yerine vektörü metne
(`[1,2,3]`) çevirip SQL'de `($1::text)::vector` ile cast ettim. Gerekçe:
parametre tipini açıkça `text` yapmak pgx'i tür çıkarımı belirsizliğinden
kurtarır ve ek bağımlılık/kayıt gerektirmez. `<=>` (cosine) operatörü soldaki
kolonu vektör olarak çözer, sağ taraf cast'li olduğu için sorun çıkmaz.

**Namespace:** Anlamsal hafıza `agent:<ad>:user:<kullanıcı>` ad alanıyla
izole edilir; `MockStore`'da da namespace filtresi test edilir. Amaç: bir
kullanıcının/ajanın hafızasının diğerine sızmaması.

### 2.7 AgentCard üreticisi (`internal/agent`)

**Karar:** `BuildCard(CardSpec)` ile kart üret; Bearer güvenlik şemasını
`RequireBearer` bayrağıyla aç/kapa.

**Gerekçe:** Kart içeriği ajanlar arasında neredeyse aynı (isim, açıklama,
skill, taşıma); elle `a2a.AgentCard` kurmak yerine tek üretici hem tutarlılık
hem de güvenlik şemasının doğru JSON şeklini garanti eder. Kartın JSON
gösterimini (ör. `httpAuthSecurityScheme`) elle kurmak hataya açık olurdu.

---

## 3. Veri modeli ve erişim desenleri

```mermaid
flowchart TB
    subgraph PG["PostgreSQL + pgvector"]
        T["a2a_tasks<br/>id · user_id · context_id · state<br/>status_timestamp · version · task jsonb"]
        M["memory_messages<br/>context_id · seq · role · content · tool_calls"]
        S["memory_semantic<br/>id · namespace · content · embedding vector(N) · metadata"]
    end
    A["LLM Provider"] --> M
    A --> S
    TS["AgentExecutor"] --> T
    C["Config / JWT"] -.-> TS
```

Ortak erişim desenleri ve neden tek motorda toplandığı:

| Desen | Tablo | Özellik |
|---|---|---|
| Görev/bağlam durumu | `a2a_tasks` | ACID, OCC (`version`), kullanıcı izolasyonu |
| Konuşma geçmişi | `memory_messages` | Sıralı ekleme, son N okuma |
| Anlamsal arama | `memory_semantic` | pgvector cosine, namespace filtresi |

Ayrı bir vektör veritabanı **kurmadım**: öğrenme projesi ölçeğinde pgvector
yeterli; ikinci bir motor operasyonel yük getirir, karşılığında ölçülebilir
fayda sağlamaz.

---

## 4. Test / doğrulama stratejisi

Bilinçli olarak **iki katman** kurdum:

1. **Ağsız birim testleri.** LLM adaptörlerinin **istek/yanıt şekillendirmesi**
   `httptest` ile doğrulandı: `TestOpenAIComplete`, `TestAnthropicComplete`,
   `TestGeminiComplete`, `TestOpenAIEmbedder`. Bunlar canlı API'ye çıkmadan
   alan eşlemesini ve hata yayılımını (`TestOpenAIErrorPropagates`) kanıtlar.
   Mock davranışı (`TestMockDefaultEcho`, `TestMockScripted`) ve safety
   kontrolleri (`TestNewRequiresAPIKey`, `TestOpenAICompatibleRequiresBaseURL`)
   ayrı test edildi.
2. **Docker Postgres'e karşı entegrasyon testleri.** `TEST_POSTGRES_DSN` yoksa
   `t.Skip` ile atlanır; varsa gerçek SQL yolu çalışır:
   `TestPostgresCreateAndGet`, `TestPostgresCreateDuplicate`,
   `TestPostgresOwnershipIsolation`, `TestPostgresUpdateOCC`,
   `TestPostgresUpdateMissing`, `TestPostgresList`,
   `TestPostgresListRequiresUser`; memory tarafında
   `TestPostgresMessageStore`, `TestPostgresSemanticStore`.

**Onay komutları:** `go build ./...`, `go vet ./...`, `gofmt -l .` ve
`TEST_POSTGRES_DSN="postgres://crew:crew@localhost:5432/crew?sslmode=disable"
go test ./...`. Ayrıca Docker imajı yeni bağımlılıklarla yeniden derlendi
(`docker build --build-arg BINARY=orchestrator ... -t a2a-research-crew/orchestrator:phase1`,
arka planda, ana süreci bloklamadan).

**Neden bu iki katman yeterliydi?** Çekirdek paketler henüz bir ajan sürecine
bağlı değildi; uçtan uca akış Phase 2'nin işiydi. Bu yüzden burada "her parça
tek başına doğru" kanıtına odaklandım. Nitekim gerçek entegrasyon hataları
(ör. embedding boyutu uyuşmazlığı) Phase 2 canlı testinde çıktı.

---

## 5. Doğrulamada çıkan sorunlar

### 5.1 Sayfalama cursor'ı — kanıt ve kök neden

**Belirti (gerçek çıktı):**

```
--- FAIL: TestPostgresList (0.01s)
    postgres_test.go:183: second page len = 0, want 1
```

**İlk (yanlış) deneme:** Sayfa sorgusunda `LIMIT pageSize+1` kullanıp
"devamı var mı" sinyalini fazladan satırdan alıyordum; cursor'ı da bu
fazladan satırdan üretiyordum. Sonuç: cursor, döndürülen sayfanın **bir
sonraki** satırını işaret ediyordu; ikinci sayfa bu yüzden boş dönüyordu.

**Kök neden:** Cursor, "sayfada son gösterilen satır" olmalı; "daha var"
sinyali için okunan satır ise sadece tetikleyicidir. İkisini karıştırdım.

**Düzeltme:** Fazladan satır okunduğunda `len(tasks) == pageSize` olur; o anda
cursor **son eklenen** satırdan (`lastTime`, `lastID`) üretilir ve okuma
kırılır. Tam olarak `pageSize` kayıt varsa cursor üretilmez (`NextPageToken`
boş) — referans `inmemory.go` davranışıyla uyumlu.

**Ders:** Sayfalama, "sınır koşulları"nın en yoğun olduğu yerdir; cursor'ın
hangi satırı temsil ettiğini tek cümleyle yazamıyorsan, test onu er geç
yakalar.

### 5.2 `go mod tidy`'nin bağımlılıkları budaması

**Belirti:** Önce `go get` ile eklediğim `modelcontextprotocol/go-sdk` ve
`pgx`, onları henüz import eden kod yazılmadan çalıştırılan `go mod tidy`
tarafından go.mod'dan çıkarıldı; sonra `go build` şu hatayı verdi:

```
no required module provides package github.com/jackc/pgx/v5/pgxpool
```

**Kök neden:** `go mod tidy` kullanılmayan modülleri temizler; sıralama
hatalıydı (önce get, sonra kod, arada tidy).

**Düzeltme:** Bağımlılıkları **onu import eden kodu yazdıktan sonra** eklemek
ve tidy'yi en sonda çalıştırmak. `pgxpool` için ayrıca geçişli bağımlılık
(`puddle`) go.sum'a girmemişti; `go mod tidy` bunu da çözdü.

**Ders:** `go.mod`'u "önceden hazırlanan" bir dosya gibi değil, kodun türevi
gibi ele almak gerekir.

---

## 6. Commit stratejisi

Kural: **bir commit = bir mantıksal değişiklik** ve her commit derlenebilir
kalmalı. Bu fazda sıralama doğal olarak bağımlılık yönünü izledi:

```
04014b0 docs: document core packages and mark phase 1 complete
397fe21 feat(agent): add agent card builder
d29ab58 feat(memory): add pgvector short-term and semantic memory
f3eb43a feat(taskstore): add postgres-backed task store with occ
b2b3894 feat(auth): add hs256 jwt manager and a2a interceptors
af181d6 feat(mcp): add mcp client wrapper and in-process fake server
9fe5d49 feat(llm): add provider abstraction with mock and cloud adapters
c8c8947 feat(config): add environment-based configuration loader
```

- Bağımlılıklar (`go.mod`/`go.sum`) ilk feature commit'ine (`c8c8947`) dahil
  edildi; böylece sonraki commit'ler derlenebilir kaldı.
- Her paket kendi commit'inde: ilgili testler o commit içinde gelir; geri almak
  istediğinde tek bir paketi hedefleyebilirsin.
- Tek dev commit atmamak bilinçli bir tercih: `git bisect` ile bir regresyonun
  hangi pakette doğduğunu bulmak mümkün kalır.

---

## 7. Bilinçli olarak ertelenenler / açık noktalar

- **Gerçek LLM/embedding anahtarları:** Adaptörler hazır ve `httptest` ile
  doğrulandı ama canlı sağlayıcıya karşı çalıştırılmadı (anahtar yok). Gerçek
  gömme modeli de aynı sebeple devrede değil; `MockEmbedder` kullanılıyor.
- **Paylaşımlı bağlantı havuzu:** `taskstore` ve `memory` her biri kendi
  `pgxpool`'unu açıyor. Doğru değil, verimli de değil; tek havuzda birleştirmek
  Phase 6'ya (sağlamlaştırma) bırakıldı.
- **LLM streaming:** `StreamingProvider` arayüzü tanımlandı ama gerçek akış
  implementasyonu Phase 4'e bırakıldı (bkz. §2.2 gerekçesi).
- **`migration` boyut kontrolü:** pgvector kolon boyutu değişirse `Migrate`
  bunu çalışma anında değil, net bir hatayla migration anında bildirmeli. Bu
  açık, Phase 2 canlı testindeki `expected 64 dimensions, not 1536` hatasıyla
  somutlaştı ve Phase 6'ya taşındı.
- **Token iptali (revocation):** Refresh akışı var ama iptal listesi yok;
  öğrenme projesi kapsamında gereksiz bulundu.

---

## 8. Kısa özet

| Karar | Seçim | Ana gerekçe |
|---|---|---|
| Paket yapısı | İnce, tek sorumluluklu paketler | Döngüsüz bağımlılık + test edilebilirlik |
| LLM adaptörleri | Elle HTTP | OpenAI-uyumlu ağ geçitleri + şeffaf tool-calling |
| Streaming | Ayrı `StreamingProvider` | Arayüzü erken şişirmemek |
| MCP | Resmi SDK + `mcpx` sarmalayıcı | Protokolü gerçek mekanizmayla test |
| Fake MCP | In-process gerçek MCP sunucusu | Stub değil, akışın kendisini test et |
| JWT | HS256 + access/refresh | Tüm ajanlar aynı güven sınırında |
| Task depolama | `jsonb` + filtre kolonları + OCC | Evrilebilir şema, doğru eşzamanlılık |
| Vektör yazımı | `($1::text)::vector` | pgx tip kaydı gerektirmez |
| Hafıza izolasyonu | `agent:...:user:...` namespace | Sızıntıyı önle |
| Doğrulama | httptest + Docker Postgres entegrasyonu | Kanıta dayalı, anahtarsız |
