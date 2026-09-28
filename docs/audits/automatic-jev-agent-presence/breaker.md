# Breaker raporu — Automatic JEV ve agent presence

Tarih: 2026-09-28  
Rol: bağımsız Breaker (parallel asymmetric audit)  
Base: `21f525a`  
Denetlenen head: `dev` çalışma ağacının denetim başındaki snapshot'ı  
Sonuç: **FAIL — 1 HIGH, 3 MEDIUM ve 1 LOW üretilmiş bulgu**

Bu rapor Reader sonucunu görmeden yazıldı. Uygulama dosyaları değiştirilmedi.
Tüm bozucu mutasyonlar ve ek deney testleri
`/tmp/mindrail-breaker-k57KMk/repo` scratch kopyasında, ayrı Go cache ve testlerin
oluşturduğu disposable SQLite veritabanlarında çalıştırıldı.

## Kapsam ve çalıştırılan kanıt

- Kaynak görev: `audit-package.md` içindeki iki kullanıcı cümlesi ve JEV'in
  anahtarsız durumda opsiyonel kalacağı önceki karar.
- Sözleşme: `docs/engineering/automatic-jev-and-agent-presence-2026-09-28.md`,
  `REQ-001`–`REQ-012`.
- Yüksek risk eksenleri: kalıcı privacy, telemetry fail-closed, throttle/dedupe
  izolasyonu, concurrent provider işi, presence start/end yarışları, migration
  10→11 ve dashboard 15s/30s sınırları.
- Race probu:
  `go test -race ./internal/agent ./internal/mcp ./internal/dashboard -run 'Route|Presence|Runtime' -count=1`
  üç pakette de geçti.
- Canonical adapter probu:
  `python3 -B -m unittest discover -s internal/agent -p 'test_jev_route.py' -v`
  sonucu **46/46 PASS**.

## Üretilmiş bulgular

### BRK-001 — Caller-controlled MCP ClientInfo credential değerini kalıcı DB'ye yazabiliyor

**Severity:** HIGH  
**Confidence:** high  
**İlgili gereksinimler:** REQ-004, REQ-006  
**Yer:** `internal/mcp/automatic.go:ensurePresence`,
`internal/agent/presence_store.go:Start`

`ClientInfo.name/title/version` bounded ve control-character açısından kontrol
ediliyor, fakat sensitive değer açısından temizlenmeden `agent_runtimes` tablosuna
yazılıyor. Candidate ID'lerin caller-controlled olup secret taşıyabileceği kabul
edilerek route tablosunda özellikle saklanmamasına rağmen aynı güven sınırı
self-reported ClientInfo'ya uygulanmıyor.

Scratch testinde aynı attribution ve tek fark olarak client adı kullanıldı:

| Kol | Client adı | DB'de `SUPER_SECRET` eşleşmesi | Yazılan runtime |
| --- | --- | ---: | ---: |
| Control | `codex` | 0 | 1 |
| Scenario | `sk-live-SUPER_SECRET_123456789` | 1 | 1 |

Komut:
`go test ./internal/agent -run '^TestBreakerSelfReportedClientCanPersistCredentialShapedValue$' -count=1 -v`
sonucu PASS oldu; yani test saldırının üretildiğini doğruladı.

Dashboard sonradan bazı bilinen environment secret'larını redact etse dahi ham
değer önce kalıcı DB'ye girmiş oluyor. Ayrıca keyring credential'ı dashboard'un
environment redactor listesinde olmak zorunda değil.

**Class sweep:** Yeni route tablosunun hiçbir serbest-metin kolonu yok. Aynı şekil
`agent_runtimes.client_name`, `client_title` ve `client_version` alanlarının
üçünde de var; üçü de etkileniyor. Eski task/session serbest metinleri bu değişimin
yeni privacy vaadinin parçası değil.

### BRK-002 — Route throttle/dedupe/serialization durable session yerine transport pointer'ına bağlı

**Severity:** MEDIUM  
**Confidence:** high  
**İlgili gereksinim:** REQ-010  
**Yer:** `internal/mcp/route.go:routeState`, `routeSessions`

State anahtarı Mindrail `SessionID` değil, `*sdk.ServerSession`. Bunun iki zıt
sonucu canlı olarak üretildi:

1. Aynı MCP connection tamamlanmış işten sonra yeni Mindrail session'a geçerse
   önceki işin 10 saniyelik limiti yeni işi yanlışlıkla bastırıyor.
2. Aynı durable run/session iki MCP connection üzerinden devam ettirilirse iki
   bağımsız mutex ve sayaç oluşuyor; provider işleri eşzamanlı çalışabiliyor.

| Deney | Control | Scenario |
| --- | ---: | ---: |
| Yeni durable session'daki provider çağrısı | fresh transport: 1, `ok` | reused transport: 0, `rate_limited` |
| Aynı durable session provider çağrıları | tek transport: max concurrency 1 | iki transport: max concurrency 2 |
| Toplam scenario provider çağrısı | — | 2 |

Komutlar:

- `go test ./internal/mcp -run '^TestBreakerThrottleCrossesDurableSessionBoundary$' -count=1 -v`
- `go test ./internal/mcp -run '^TestBreakerTwoConnectionsSameDurableSessionRunProviderConcurrently$' -count=1 -v`

İkisi de saldırı beklentileriyle PASS oldu. Bu sorun izin/gate atlatmıyor; ancak
per-session maliyet ve duplicate/serialization sözleşmesini session sınırının iki
tarafında da bozuyor.

**Class sweep:** `lastProvider`, `recent` duplicate map'i ve mutex aynı
`routeSessionState` içinde olduğundan üç koruma da aynı kapsam hatasından etkileniyor.
Başka bir JEV throttle state uygulaması bulunmadı.

### BRK-003 — Presence başlangıcındaki geçici hata connection boyunca yeniden denenmiyor

**Severity:** MEDIUM  
**Confidence:** high  
**İlgili gereksinimler:** REQ-006, REQ-007  
**Yer:** `internal/mcp/automatic.go:ensurePresence`, `touchPresence`

`presence.Start` hata verince hata sessizce yutuluyor ve binding boş kalıyor.
Sonraki tool çağrılarının middleware'i yalnız mevcut binding'e `Activity` yazmayı
deniyor; `Start`ı tekrar çağırmıyor.

| Kol | İlk Start | Sonraki tool sonrası Start attempt | Aktif binding/activity hedefi |
| --- | ---: | ---: | ---: |
| Control | başarılı | 1 | 1 |
| Scenario | bir kez transient error | 1 | 0 |

Scenario komutu:
`go test ./internal/mcp -run '^TestBreakerPresenceStartFailureIsNotRetriedByLaterToolActivity$' -count=1 -v`
PASS. Control, gerçek in-memory MCP testinde 1 runtime ve pozitif heartbeat/activity
üretmiştir (`TestPresenceStartsFromBootstrapHeartbeatsTracksToolsAndEndsOnDisconnect`).

Sonuç olarak agent gerçekten bağlı ve tool çağırıyor olsa bile dashboard'da o
connection için hiçbir runtime oluşmayabilir.

**Class sweep:** `ensurePresence` bootstrap/resume yollarında çağrılıyor; binding'i
olan normal tool çağrılarında retry yolu yok. `Start`ın bütün hata türleri aynı
şekilde etkileniyor.

### BRK-004 — Presence End yazısı bir kez hata verirse `sync.Once` kalıcı olarak retry'ı engelliyor

**Severity:** MEDIUM  
**Confidence:** high  
**İlgili gereksinimler:** REQ-006, REQ-007  
**Yer:** `internal/mcp/automatic.go:stopPresence`

`binding.once.Do` içindeki DB `End` hatası yutuluyor. `Once`, yazının başarılı
olduğunu değil yalnız callback'in çalıştığını hatırladığı için ikinci kapatma
çağrısı DB'yi tekrar denemiyor.

| Kol | `stopPresence` çağrısı | DB End attempt | Kalıcı ended row |
| --- | ---: | ---: | ---: |
| Control | 1 | 1 | 1 |
| Scenario: ilk End transient error, sonra tekrar stop | 2 | 1 | 0 |

Komut:
`go test ./internal/mcp -run '^TestBreakerFailedEndIsNeverRetried$' -count=1 -v`
PASS. Başarısız row daha sonra heartbeat kesildiği için `ENDED` değil `STALE`
görünecek; bu disconnect/completion'ın runtime'ı sonlandıracağı sözleşmesini bozuyor.

**Class sweep:** `completed`, `disconnected`, `replaced` ve `shutdown` kapanışlarının
tamamı `stopPresence` üzerinden geçiyor ve aynı kayıp-yazı sınıfından etkileniyor.

### BRK-005 — Beş saniyelik heartbeat mekanik kuralını pinleyen test yok

**Severity:** LOW  
**Confidence:** high  
**İlgili gereksinimler:** REQ-006, REQ-012  
**Yer:** `internal/mcp/server.go`, `internal/mcp/automatic.go`

Control'de `internal/mcp` paketi geçti. Scratch mutasyonunda production default ve
fallback interval `5s`→`6s` yapıldı; paket yine bütünüyle geçti.

| Kol | Heartbeat default | `go test ./internal/mcp` exit | Kırmızı test |
| --- | ---: | ---: | ---: |
| Control | 5s | 0 | 0 |
| Scenario mutation | 6s | 0 | 0 |

Mevcut lifecycle testi production default'u kullanmak yerine interval'i `2ms`
olarak override ediyor. Bu nedenle sözleşmedeki mekanik sayı gelecekte sessizce
değişebilir.

**Class sweep:** Default iki yerde (`Server.New` ve sıfır/değersiz interval fallback'i)
tekrarlanıyor; mutasyon ikisini de değiştirdi. 15s/30s dashboard eşikleri ise hem
Go hem JS exact-boundary testleriyle korunuyor.

## B1 — Guard mutation özeti

Mutasyonlar yalnız scratch kopyada uygulandı ve her dosya deneyden sonra geri
yüklendi.

| # | Neutralize edilen koruma | Targeted sonuç | Yorum |
| --- | --- | --- | --- |
| M1 | Telemetry write hatasında accepted selection temizleme | RED, rc=1 | Test koruyor |
| M2 | Ended runtime heartbeat `ended_at IS NULL` filtresi | RED, rc=1 | Resurrection testi koruyor |
| M3 | Server heartbeat `>15s` sınırını `>=15s` yapma | RED, rc=1 | Exact boundary korunuyor |
| M4 | Browser heartbeat `>15s` sınırını `>=15s` yapma | RED, rc=1 | Exact boundary korunuyor |
| M5 | Per-connection route mutex'i kaldırma | RED, 5/5 run | Serialization testi koruyor |
| M6 | Caller candidate membership kontrolünü etkisizleştirme | RED, rc=1 | Out-of-set seçim testi koruyor |
| M7 | Production heartbeat 5s→6s | GREEN, rc=0 | BRK-005 |

M5'te test provider-call sayısındaki farkla kırmızı oldu; kaldırılan mutex'in gerçekten
neutralize edildiği source diff ile doğrulandı. M6'nın ilk kaba mutasyonu unused local
nedeniyle derlenmedi; kanıt sayılmadı. İkinci mutasyon değişkeni erişilebilir bırakıp
yalnız predicate'i `false` yaptı ve davranış testi kırmızı oldu.

## B2 — Silinen davranışın A/B karşılaştırması

Refactor öncesi base binary ve yeni binary ayrı dizinlerde build edildi. Gerçek keyring
ve network'e dokunmamak için ikisine de erişilemeyen disposable D-Bus adresi verildi;
aynı bounded JSON stdin kullanıldı.

| Binary | Exit | stdout byte | SHA-256 | status/reason |
| --- | ---: | ---: | --- | --- |
| `21f525a` | 0 | 131 | `96c71888d630059e7a7dc24482061eb59f501b192f2b12de0f06b3542d9aecdc` | `fallback/credential_unavailable` |
| current | 0 | 131 | `96c71888d630059e7a7dc24482061eb59f501b192f2b12de0f06b3542d9aecdc` | `fallback/credential_unavailable` |

Hidden `mindrail agent route` v1 JSON compatibility'sinde fark üretilmedi.

## B3 — Yazılı threat model denemeleri

| Tehdit | Control | Scenario | Sonuç |
| --- | --- | --- | --- |
| Provider caller seti dışından ID döndürür | allowed ID → `ok` | `shell` out-of-set → fallback, 0 selection | Savunuldu |
| Accepted advice sonrası telemetry DB hatası | telemetry ok → 1 selection | recorder error → `telemetry_failed`, 0 selection | Savunuldu |
| API key child environment'a sızar | yalnız resolved key | proxy/CA/loader/PYTHONPATH enjekte | Child env allowlist testi savundu |
| Adapter output/read hatası hassas detay taşır | normal typed result | sensitive error → sanitized fallback | Savunuldu |
| Ended runtime heartbeat ile diriltilir | pre-end write succeeds | post-end write → `ErrRuntimeEnded` | Savunuldu |
| Self-reported identity secret taşır | safe name persisted | credential-shaped name persisted | **BRK-001** |
| Aynı durable session iki connection'dan provider çağırır | max=1 | max=2 | **BRK-002** |
| Presence DB start/end transient failure | successful lifecycle | retry yok | **BRK-003/004** |

Gerçek TypeSafe ağına çağrı yapılmadı; canonical adapter'ın fake opener/deadline/
response testlerinden 46/46 geçti.

## B4 — Schema 10 upgrade

Komut:
`go test ./internal/migration -run '^TestSchemaTenUpgradesAddAgentTelemetryWithoutTouchingExistingRows$' -count=1 -v`
PASS.

| Ölçüm | Schema 10 control | Upgrade sonrası | İkinci `Up` |
| --- | ---: | ---: | ---: |
| Schema version | 10 | 11 | 11 |
| Korunan eski task row | 1 | 1 | 1 |
| Yeni telemetry table | 0 | 2 | 2 |
| Bu çağrıda uygulanan migration | 10 (ilk kurulum) | 1 (`000011`) | 0 |

Ek migration/shipped testleri de geçti. Eski satır kaybı, tekrar uygulama veya shipped
migration mutasyonu üretilmedi.

## B5 — Weakest satisfying input

Yeni predicate'ler literal en zayıf değerlerle denendi:

| Predicate | En zayıf/ayırıcı input | Sonuç |
| --- | --- | --- |
| selection caller setinde | set dışı tek ID | fallback; seçim kaçmadı |
| accepted advice ancak telemetry kayıtlıysa | recorder tek hata | selection 1→0 |
| runtime açıkken update | End'den sonra tek heartbeat | `ErrRuntimeEnded` |
| heartbeat fresh `<=15s` | tam 15.000s / +1ms | CONNECTED / STALE |
| activity fresh `<=30s` | tam 30.000s / +1ms | CONNECTED / IDLE |
| duplicate `<30s` | 29.999s / 30.000s | provider 0 / 1 |
| rate `<10s` | 9.999s / 10.000s | provider 0 / 1 |
| ClientInfo nonempty, trimmed, bounded | credential-shaped printable string | kabul ve persist; BRK-001 |
| connection-keyed route state | yeni SessionID, aynı SDK pointer | önceki limit taşındı; BRK-002 |
| `sync.Once` stop | ilk DB error, ikinci stop | ikinci DB attempt yok; BRK-004 |

Throttle/duplicate exact-boundary scratch testi beş alt durumda geçti.

## B6 — Dual readers

Agent runtime status'unu server (`runtimeStatus`) ve browser (`runtimeCard`) ayrı
okuyor. Aynı dört input her iki okuyucuya verildi:

| Input | Go collector | Browser view |
| --- | --- | --- |
| heartbeat 15s, activity 30s | CONNECTED | CONNECTED |
| heartbeat 15s, activity 30s+1ms | IDLE | IDLE |
| heartbeat 15s+1ms | STALE | STALE |
| `ended_at` mevcut | ENDED | ENDED; duration frozen |

Go exact-boundary testi ve Node client assertion dosyası ayrı ayrı PASS. Bu iki
okuyucu arasında fark üretilmedi. Browser her saniye tekrar türettiği için SSE'nin
bir sonraki 1.5s frame'ini beklemeden sınır geçişini yapıyor.

JEV “configured” startup snapshot'ı ile “used” durable accepted route sayacı da ayrı
okuyucular değil, bilinçli iki farklı soru olarak sunuluyor; fallback-only 103 row
fixture'ında `used=false`, `accepted=0`, detail bound=100 sonucu mevcut testte geçti.

## Maliyet hesabı

### JEV route

- Her MCP connection için minimum interval 10s: en fazla **6 provider call/dakika**,
  **360/saat**.
- Her call toplam provider deadline 10s olduğundan tek connection'ın izin verdiği
  worst case provider bekleme zamanı **360 × 10s = 3600s/saat**; serialization bunu
  tek sıra halinde tutuyor.
- Adapter input üst sınırı 64 KiB: **360 × 64 KiB = 22.5 MiB/saat/connection**.
- Provider response üst sınırı 256 KiB: **90 MiB/saat/connection**.
- Dört dimension × 32 = 128 aday teorik shape sınırı var; 64 KiB global request
  sınırı daha önce devreye girebilir.
- Global connection sınırı yok; N connection için değerler N ile çarpılıyor.
  REQ-010 açıkça per-session bound istediği için bu tek başına yeni bulgu sayılmadı,
  fakat BRK-002 aynı durable session'ın birden çok connection ile bu çarpanı
  kullanabildiğini gösteriyor.

### Presence ve dashboard

- Presence default 5s: **12 DB heartbeat/dakika/runtime**.
- SSE: 1.5s frame = **40 frame/dakika/stream**; max 8 stream ve max 256 KiB snapshot
  ile teorik üst sınır **8 × 40 × 256 KiB = 80 MiB/dakika** loopback payload.
- Detail listeleri frame başına 100 JEV event ve 120 runtime ile bounded.

Bu değerler local/loopback çalışma modeli içinde; yeni bir cost bulgusu üretilmedi.

## Mechanical rule = automated test

| Mekanik kural | Otomasyon durumu |
| --- | --- |
| 14 görünür tool ve `mindrail_route` discovery | Otomatik test var |
| Managed text görünür route'u CLI fallback'ten önce söyler | Otomatik test var |
| Telemetry başarısızsa accepted advice kaçmaz | Otomatik test + öldürülen mutation |
| 10s serialization/rate ve 30s duplicate | Temel test var; scratch exact-boundary probe geçti |
| 15s/30s Go ve browser inclusive sınırlar | İki ayrı exact-boundary test var |
| Schema 10→11 ve rerun | Otomatik test var |
| Default heartbeat tam 5s | **Test yok; BRK-005** |
| “Ambiguous choice'tan önce exactly once” | Metin testi var; runtime enforcement yok |

Son satır ürünün belgelenmiş hard limit'i: MCP private ambiguity'yi gözleyemez.
Bu yüzden runtime enforcement eksikliği ayrı defect yapılmadı; bunun “policy, not
observed fact” olduğu docs'ta açıkça yazıyor.

## Backward compatibility

- Hidden CLI aynı input ve credential-unavailable ortamında byte-for-byte aynı v1
  JSON'u ve exit 0'ı üretti.
- No-key/credential-unavailable yolunda provider/network çağrısı yapılmadı.
- Schema-10 data upgrade sonrası eski task row 1→1 korundu; yalnız migration 11
  uygulandı, ikinci `Up` 0 migration uyguladı.
- Tool yüzeyi additive 13→14; eski tool adları korunuyor. Eski client'ın yeni tool'u
  kullanmaması normal akışı değiştirmiyor.
- Eski binary'nin schema-11 DB'yi okuyabilmesi bu sözleşmenin forward-compatibility
  hedefi değil ve test edilmedi.

## Açık deneyler

1. **Gerçek provider:** Disposable TypeSafe test credential ve faturalandırmasız
   sandbox endpoint sağlanırsa gerçek TLS/provider round-trip'te request/body/log
   sızıntısı ile timeout tekrar ölçülmeli. Bu denetimde gerçek credential veya ağ
   kullanılmadı; canonical fake-opener testleri 46/46 geçti.
2. **Process-kill crash:** Bir MCP subprocess'i SIGKILL edip gerçek dashboard'da
   15.000s/+1ms geçişini ölçmek için wall-clock kontrollü E2E harness gerekir.
   Store ve iki okuyucu ayrı ayrı sınırı geçti; gerçek zamanlı process-kill deneyi
   bu turda üretilmedi.

Bu iki açık deney mevcut üretilmiş bulguları refute etmez.

## İzolasyon kapanışı

- Orijinal çalışma ağacında audit öncesi implementation değişiklikleri dışında
  yalnız bu `breaker.md` oluşturuldu.
- Scratch mutation/test dosyaları `/tmp/mindrail-breaker-k57KMk` altında kaldı ve
  kullanıcı runtime DB'sine bağlanmadı.
- Shared Mindrail runtime yalnız bu rapor dosyasına scoped audit task kaydı için
  kullanıldı; bozucu deneylerin tamamı disposable DB/scratch kopyada yapıldı.

