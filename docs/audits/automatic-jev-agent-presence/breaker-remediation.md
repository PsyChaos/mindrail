# Breaker remediation delta — Automatic JEV ve agent presence

Tarih: 2026-09-28  
Rol: bağımsız Breaker remediation doğrulaması  
Base: `21f525a`  
Denetlenen head: `dev` çalışma ağacının remediation sonrası snapshot'ı  
Sonuç: **FAIL — önceki altı bulgu giderildi; 1 MEDIUM sözleşme regresyonu ve 1 LOW telemetry amplification bulgusu üretildi**

Bu rapor Reader remediation sonucunu görmeden yazıldı. Uygulama dosyaları
değiştirilmedi. Ek deneyler ve bozucu mutasyonlar
`/tmp/mindrail-breaker-remediation-faAxwH/repo` izole kopyasında, ayrı Go cache,
geçici build dizini ve disposable SQLite veritabanlarında çalıştırıldı.

## Yönetici özeti

Önceki `BRK-001`–`BRK-005` ve `RDR-001` davranışlarının tamamı control/scenario
ile tekrarlandı ve remediation'ın hedeflenen dar davranışı değiştirdiği doğrulandı.
Sekiz yeni koruma mutasyonunun sekizi de ilgili testleri kırdı. Race, schema-10
upgrade, hidden CLI byte compatibility, Python adapter ve iki dashboard okuyucusu
geçti.

Ancak privacy düzeltmesi frozen contract'ın başka bir bölümünü fazla reddediyor:
caller'ın title/version alanları artık hiçbir zaman kalıcılaştırılmıyor ve whitelist
dışındaki güvenli client adları `unknown-client` yapılıyor. Buna rağmen dashboard
çıktısı alanları hâlâ `self-reported` olarak işaretliyor ve README/frozen contract
adı/sürümü göstereceğini söylüyor. Ayrıca 256-entry route-state cache doluyken aynı
yeni session'dan 1.000 hızlı fallback isteği 0 provider işi fakat 1.000 kalıcı
telemetry write üretti; bu yol telemetry için request-rate bound uygulamıyor.

## Önceki bulguların control/scenario delta'sı

| Bulgu | Önce | Remediation sonrası ayırıcı ölçüm | Karar |
| --- | --- | --- | --- |
| BRK-001 credential-shaped ClientInfo persistence | scenario'da DB secret eşleşmesi `1`, control `0` | name/title/version'ın her birine gerçek `TYPESAFE_API_KEY` yerleştirildi; üçünde DB secret eşleşmesi `0` | **FIXED** |
| BRK-002 transport-keyed state | reused transport + yeni durable session provider `0`; aynı session + iki transport max concurrency `2` | reused transport + yeni durable session provider `1`; aynı durable session + iki transport max concurrency `1` | **FIXED** |
| BRK-003 Start transient failure | Start attempts `1`, aktif binding/activity `0` | Start attempts `2`, activity `1`; sekiz eşzamanlı retry adayı yalnız bir runtime yayınladı | **FIXED** |
| BRK-004 End transient failure | iki terminal trigger sonrası End attempts `1`, ended `0` | ilk error ardından başarı: attempts `2`, ended `1`; persistent error: trigger başına `3`, iki trigger sonrası `6` | **FIXED** |
| BRK-005 5s literal testi | `5s→6s` mutation GREEN, rc=0 | `5s→6s` mutation RED, rc=1 | **FIXED** |
| RDR-001 fallback evidence kaybı | atomic/no-match fallback ordinal ve confidence `nil` | atomic fallback ordinal `1`, confidence `550`; no-match confidence kayıtlı, ordinal yalnız seçim varsa kayıtlı | **FIXED** |

RDR-001'in düzeltmesi tavsiyeyi tüketilebilir yapmadı: scratch probunda durable
ordinal `1` ve confidence `550` iken MCP `status=fallback` ve
`Accepted=false` kaldı.

## Yeni üretilmiş bulgular

### BRK-R01 — Privacy projection belgelenmiş client kimliğini aşırı reddediyor

**Severity:** MEDIUM  
**Confidence:** high  
**İlgili gereksinimler:** REQ-006, REQ-007  
**Yer:** `internal/agent/presence_store.go:projectClientInfo`, dashboard model/docs

Frozen public contract `ClientInfo` name/title/version'ı bounded ve self-reported
olarak taşımayı; dashboard da self-reported client name/version göstermeyi vaat
ediyor. Yeni projection raw title ve version'ı koşulsuz atıyor, yalnız sabit bir
client-name whitelist'ini canonical ad/title'a dönüştürüyor ve diğer bütün client
adlarını `unknown-client` yapıyor.

| Kol | Raw ClientInfo | Kalıcı değer | Secret sızıntısı | Sözleşme kimliği korundu mu? |
| --- | --- | --- | ---: | ---: |
| Control, bilinen | `codex / Codex / 2026.9` | `codex Codex` | 0 | version: **hayır** |
| Scenario, güvenli bilinmeyen | `my-safe-custom-agent / My Safe Agent / 2026.9` | `unknown-client` | 0 | **hayır** |
| Scenario, key name'de (`key=codex`) | `codex / Codex / 1.2.3` | `unknown-client` | 0 | privacy doğru |
| Scenario, key title'da | `claude-code / actual-key / 2.0` | `claude-code Claude Code` | 0 | privacy doğru; version da kayıp |
| Scenario, key version'da | `cursor / Cursor / 1.2.3` | `cursor Cursor` | 0 | privacy doğru |

Komut:
`go test ./internal/agent -run 'TestBreakerRemediation|TestPresenceStoreNeverPersists|TestPresenceStorePreservesOnlyKnown' -count=1 -v`
PASS; saldırı değerlerinin kesildiğini ve güvenli değerlerin de kaybolduğunu aynı
DB sınırında ölçtü.

README ve `docs/usage-tr.md` hâlâ istemcinin kendi bildirdiği ad/sürümün
gösterileceğini söylüyor. `internal/dashboard/model.go` da projection sonrası
canonical değerleri `ClientSelfReported=true` olarak sunuyor. Bu yalnız doküman
eskimesi değildir: whitelist dışındaki MCP istemcileri ayırt edilemez, bilinen
istemcilerin sürümleri gösterilemez ve kullanıcıya verinin kaynağı yanlış
nitelendirilir.

**Class sweep:** `client_name`, `client_title`, `client_version` üçü incelendi.
Her version, caller'ın her title'ı ve whitelist dışındaki her güvenli client adı
etkileniyor. Task/session attribution veya route-event serbest metinleri bu
projection'dan etkilenmiyor.

### BRK-R02 — Dolu route-state cache fallback telemetry yazılarını rate-limit etmiyor

**Severity:** LOW  
**Confidence:** high  
**İlgili gereksinim:** REQ-010  
**Yer:** `internal/mcp/route.go` cache-capacity fallback yolu

256 state slot'u doluyken yeni session provider işine sokulmuyor; bu fail-open
davranış doğru. Fakat state ayrılamadığı için aynı session'a ait sonraki her çağrı
yeniden telemetry yazıyor. Tek process içindeki ayırıcı prob:

| Ölçüm | Control: normal state | Scenario: cache 256/256 |
| --- | ---: | ---: |
| Hızlı route request | ikinci istek 10s penceresinde bound | 1.000 |
| Provider call | en çok 1 | 0 |
| Kalıcı telemetry record | rate/dedupe state ile sınırlı | **1.000** |

Komut:
`go test ./internal/mcp -run '^TestBreakerRemediationSaturatedFallbackTelemetryHasNoRequestRateBound$' -count=1 -v`
PASS ve `requests=1000 provider_calls=0 telemetry_records=1000` üretti.

Bu yol dış provider maliyeti üretmiyor ve veri shape'i bounded; bu nedenle LOW.
Yine de frozen REQ-010'un telemetry'nin bounded olması hedefi ve lokal SQLite
write-amplification açısından açık bir maliyet sınırı yok. Aynı durable session'ın
normal 10s/30s state'i cache doluyken uygulanamıyor.

## B1 — Guard mutation delta'sı

Mutasyonlar yalnız scratch kopyada uygulandı ve her deneyden sonra source geri
alındı.

| # | Neutralize edilen remediation koruması | Targeted sonuç |
| --- | --- | --- |
| RM1 | fallback ordinal/confidence kaydını yeniden `Accepted` guard'ına bağla | RED, rc=1 |
| RM2 | privacy projection yerine raw ClientInfo persist et | RED, rc=1 |
| RM3 | durable session key yerine sabit/transport-benzeri ortak key kullan | RED, rc=1 |
| RM4 | route-state active reservation increment'ini kaldır | RED, rc=1 |
| RM5 | tool activity'de failed Start retry yolunu kaldır | RED, rc=1 |
| RM6 | End deneme sayısını `3→1` indir | RED, rc=1 |
| RM7 | production heartbeat literalini `5s→6s` değiştir | RED, rc=1 |
| RM8 | reservation release sırasında active decrement'i kaldır | RED, rc=1 |

Sekiz mutasyonun tümü davranış testleri tarafından öldürüldü; compile-only red
kanıt olarak kullanılmadı.

## B2 — Silinen davranış A/B ve backward compatibility

Base `21f525a` ve remediation çalışma ağacı ayrı binary'ler olarak build edildi.
Gerçek keyring/network'e dokunmayan inaccessible disposable D-Bus ortamında aynı
geçerli JSON input verildi:

| Binary | Exit | stdout byte | SHA-256 | status/reason |
| --- | ---: | ---: | --- | --- |
| `21f525a` | 0 | 131 | `96c71888d630059e7a7dc24482061eb59f501b192f2b12de0f06b3542d9aecdc` | `fallback/credential_unavailable` |
| remediation | 0 | 131 | `96c71888d630059e7a7dc24482061eb59f501b192f2b12de0f06b3542d9aecdc` | `fallback/credential_unavailable` |

Hidden `mindrail agent route` v1 JSON contract'ında byte farkı yok. Python
canonical adapter suite **46/46 PASS**; no-key/provider-failure normal reasoning
yolu korunuyor.

## B3 — Yazılı threat model ve lifecycle/race sonuçları

| Tehdit | Control | Scenario | Sonuç |
| --- | --- | --- | --- |
| Key name/title/version'a gömülür | güvenli canonical ad | gerçek `TYPESAFE_API_KEY` üç alanda | DB secret eşleşmesi 0 |
| Key sıradan canonical ada eşittir | key yokken `codex` | key=`codex`, name=`codex` | güvenli placeholder; sızıntı 0 |
| Arbitrary opaque güvenli kimlik | bilinen client korunur | whitelist dışı client | **BRK-R01** |
| Aynı durable session, iki transport | tek mutex | iki eşzamanlı call | max provider concurrency 1 |
| Aynı transport, yeni durable session | eski session throttle | yeni session | yeni provider call 1 |
| State cache saturation | 256 state | yeni session | fail-open, provider 0; telemetry **BRK-R02** |
| Active reservation prune edilir | idle inactive evict | 30s yaşlı active state | active state korundu |
| Start fail, eşzamanlı retry | ilk Start başarılı | ilk fail + 8 aday | toplam Start 2, tek published runtime |
| Start tamamlanmadan disconnect | normal binding | late completion | late runtime `replaced`, binding dirilmedi |
| Start tamamlanmadan replace | eski run | yeni run publish olmuşken late old Start | eski runtime ended; yeni binding bozulmadı |
| End ilk çağrı fail | ilk çağrı başarılı | transient fail | ikinci attempt başarı, total 2 |
| End kalıcı fail | normal başarı | persistent error | trigger başına 3, iki trigger 6; sonsuz loop yok |
| Fallback evidence consumable olur | accepted ok | fallback ordinal/confidence kayıtlı | output fallback ve Accepted=false |

Race komutu:
`go test -race ./internal/agent ./internal/mcp ./internal/dashboard -run 'Route|Presence|Runtime' -count=1`
üç pakette PASS (`10.343s`, `3.187s`, `1.503s`).

## B4 — Schema-10 upgrade

Migration ve shipped dosya testleri remediation snapshot'ında tekrar PASS:

`go test ./internal/migration ./migrations -run 'SchemaTen|Upgrade|Shipped|Migration' -count=1`

Önceki ayırıcı sayıların sonucu değişmedi:

| Ölçüm | Schema 10 control | Upgrade sonrası | İkinci `Up` |
| --- | ---: | ---: | ---: |
| Schema version | 10 | 11 | 11 |
| Korunan eski task row | 1 | 1 | 1 |
| Yeni telemetry table | 0 | 2 | 2 |
| Uygulanan yeni migration | — | 1 (`000011`) | 0 |

Eski row kaybı, migration tekrar uygulaması veya shipped migration değişimi
üretilmedi.

## B5 — Weakest satisfying inputs ve kesin sınırlar

| Predicate | En zayıf ayırıcı değer | Sonuç |
| --- | --- | --- |
| duplicate `<30s` | 29.999s / 30.000s | provider 0 / 1 |
| rate `<10s` | 9.999s / 10.000s | provider 0 / 1 |
| cache idle eviction | 29.999s / 30.000s | tutulur / evict edilir |
| heartbeat freshness `<=15s` | 15.000s / +1ms | CONNECTED / STALE |
| activity freshness `<=30s` | 30.000s / +1ms | CONNECTED / IDLE |
| End retry | tek transient error | attempt 2 başarı |
| persistent End bound | 4. hata gereksinimi | yalnız ilk 3 attempt, trigger döner |
| Start retry | ilk tek error + sonraki tek tool | ikinci Start ile binding |
| fallback evidence | ordinal 1, confidence .550, Accepted=false | kayıtlı ama tüketilemez |
| configured secret collision | key tam `codex` | persisted `unknown-client` |
| güvenli bilinmeyen identity | tek printable bounded ad | `unknown-client`; BRK-R01 |

## B6 — Dual readers

Server (`runtimeStatus`) ve browser (`runtimeCard`) aynı kesin inputlarla yeniden
çalıştırıldı:

| Input | Go collector | Browser |
| --- | --- | --- |
| heartbeat 15s, activity 30s | CONNECTED | CONNECTED |
| heartbeat 15s, activity 30s+1ms | IDLE | IDLE |
| heartbeat 15s+1ms | STALE | STALE |
| ended_at mevcut | ENDED | ENDED; duration frozen |

`TestCollectorDerivesTruthfulRuntimePresenceAtExactBoundaries` ve
`node --test internal/dashboard/testdata/client_behavior_test.cjs` PASS. SSE
1.5s ve browser 1s yeniden türetim mekanizmasına dokunulmadı. Bu iki okuyucu
arasında yeni fark üretilmedi.

## Mechanical rule = automated test

| Mekanik kural | Remediation durumu |
| --- | --- |
| Privacy persistence boundary raw caller değerini yazmaz | Otomatik test + RM2 mutation |
| Fallback ordinal/confidence kayıtlı ama consumable değildir | Otomatik test + RM1 mutation |
| Route state durable session'a bağlı ve serialized | Otomatik test + RM3 mutation |
| Cache en çok 256, inactive 30s eviction, active reservation korunur | Otomatik test + RM4/RM8 mutation |
| Start fail sonraki tool'da retry edilir; tek winner | Otomatik test + RM5 mutation |
| Late Start disconnect/replace sonrası yayınlanmaz | Otomatik test var |
| End trigger başına en çok 3 attempt | Otomatik test + RM6 mutation |
| Production heartbeat tam 5s | Otomatik exact test + RM7 mutation |
| Rate 10s / duplicate 30s exact sınır | Scratch exact-boundary testi |
| Dashboard 15s/30s Go ve browser sınırı | İki bağımsız otomatik okuyucu |
| Self-reported client name/version gösterilir | **Kod/docs uyuşmuyor; BRK-R01** |
| Telemetry çağrı hacmi bounded | **Saturation yolunda yok; BRK-R02** |

## Maliyet delta'sı

### Route

- Provider limiti değişmedi: session başına en fazla **6/dakika**, **360/saat**.
- 10s deadline ile worst-case bekleme **3.600s/saat/session**.
- 64 KiB request bound ile **22.5 MiB/saat/session** input; 256 KiB response ile
  **90 MiB/saat/session** provider output.
- Route-state cache artık en çok **256** state ve 30s inactive retention tutuyor.
  10s provider aralığı/30s prune ile session başına steady-state yaklaşık en çok
  üç recent digest; yaklaşık **768 digest + map overhead**.
- Saturation provider maliyetini sıfırlar, fakat telemetry write sayısı request
  sayısıyla birebir büyür: 1.000 request → 1.000 write (**BRK-R02**).

### Presence/dashboard

- Heartbeat değişmedi: **12 DB write/dakika/runtime**.
- End transient recovery tek terminal trigger maliyetini 1'den en çok **3 DB
  attempt**'e çıkarır; persistent hata sonrası yeni terminal trigger üç ek attempt
  yapabilir, çağrı içinde sonsuz retry yok.
- Start fail sonrası her sonraki tool yeni bir Start attempt tetikleyebilir; concurrent
  reservation duplicate runtime'ı engeller.
- Dashboard teorik loopback bound değişmedi:
  `8 stream × 40 frame/dakika × 256 KiB = 80 MiB/dakika`.

## Backward compatibility ve class sweep

- Hidden CLI byte-for-byte aynı fallback JSON'u ve exit 0'ı üretti.
- Python adapter 46/46 ve focused Go route/presence/runtime paketleri PASS.
- Schema 10→11 additive/idempotent; eski task row 1→1.
- No-key/credential-unavailable yolunda network/provider işi yok.
- Route state'in `lastProvider`, `recent`, mutex ve active reservation alanlarının
  tamamı durable session scope'unda test edildi.
- Presence `completed`, `disconnected`, `replaced`, `shutdown` terminal sınıfları
  ortak retry/generation yolundan geçiyor; disconnect/replace late-start ve
  concurrent terminal testleri geçti.
- Privacy sweep bütün üç ClientInfo kolonunu ve environment credential'ın her
  kolondaki tam collision'ını kapsadı; güvenli unknown/known version over-refusal
  BRK-R01 olarak ayrıldı.
- Fallback evidence atomic/no-match/low-confidence sınıflarında durable kalabilir;
  yalnız output `ok + accepted` ise tüketilebilir.

## Açık deneyler

1. Gerçek disposable TypeSafe sandbox credential sağlanırsa TLS/provider
   round-trip ve dış servis log disclosure tekrar ölçülmeli; bu audit gerçek
   credential veya ağa çıkmadı.
2. `BRK-R01` için kabul edilen privacy policy netleştirilmelidir: frozen contract
   korunacaksa güvenli bounded client identity için secret-safe projection gerekir;
   whitelist-only policy istenecekse contract, docs ve `ClientSelfReported` semantiği
   birlikte değişmelidir.
3. `BRK-R02` gerçek SQLite üzerinde wall-clock/write-lock profiliyle ölçülebilir;
   control/scenario write cardinality kusuru üretmek için yeterlidir fakat bu turda
   disk throughput benchmark'ı yapılmadı.

Bu açık deneyler önceki altı remediation fix'ini refute etmez; iki yeni bulgunun
kanıtını da ortadan kaldırmaz.

## İzolasyon kapanışı

- Kullanıcı çalışma ağacında implementation dosyası değiştirilmedi; bu rapor tek
  sahip olunan çıktı dosyasıdır.
- Scratch testleri disposable DB kullandı; shared runtime DB'ye bağlanmadı.
- Shared Mindrail yalnız bu rapor yoluna scoped audit task için kullanıldı.
- Reader remediation raporu bağımsızlık korunurken okunmadı.
