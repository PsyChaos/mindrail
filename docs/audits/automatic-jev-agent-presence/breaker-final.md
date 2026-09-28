# Final Breaker closure — Automatic JEV ve agent presence

Tarih: 2026-09-28  
Rol: bağımsız final Breaker  
Base: `21f525a`  
Denetlenen head: `dev` çalışma ağacının final remediation snapshot'ı  
Sonuç: **APPROVE — üretilmiş açık bulgu yok**

Bu rapor Reader final sonucu görülmeden yazıldı. Uygulama dosyaları
değiştirilmedi. Deneyler `/tmp/mindrail-breaker-final-32T1M2/repo` izole
kopyasında, ayrı Go cache/build dizini ve disposable SQLite veritabanlarıyla
çalıştırıldı. Orijinal `.git/mindrail` sandbox'ta read-only olduğundan audit
task'i aynı runtime'ın `/tmp/mindrail-breaker-final-runtime` disposable kopyasında,
yalnız bu rapor yoluna scoped olarak bootstrap edildi.

## Kapanış özeti

- `BRK-R01` artık bulgu değildir: explicit post-audit security amendment ve
  operator dokümanları güvenli davranışı canonical family olarak tanımlıyor;
  unknown/custom client'ın `unknown-client` olması bilinçli security policy'dir.
- `BRK-R02` giderildi: 256-entry cache doluyken 1.000 overflow isteği **0 provider
  call, 0 telemetry write** üretti.
- Admitted bir session'a 2.001 hızlı istek yalnız **1 provider call ve 3 telemetry
  write** üretti.
- Telemetry DB hatası altındaki 1.001 istek **1 provider call, 2 bounded write
  attempt ve 0 consumable selection** üretti.
- Normal provider success yolu hâlâ kanıt üretiyor: **1 provider call, 1 telemetry
  write, 1 selection, `telemetry_recorded=true`**.
- Önceki HIGH/MEDIUM saldırılarından credential persistence, durable-session
  isolation, Start retry ve End retry yeniden üretilemedi.

## Final control/scenario tablosu

| Saldırı | Control | Scenario | Final ölçüm | Karar |
| --- | --- | --- | --- | --- |
| ClientInfo credential | bilinen `codex` → `codex Codex` | gerçek key name/title/version'da | üç scenario'da DB key eşleşmesi `0` | Savunuldu |
| Güvenli unknown client | bilinen family ayırt edilir | `my-safe-custom-agent` | `unknown-client`, runtime yine oluşturuldu | Amendment ile amaçlanan davranış |
| Cache overflow | admitted state provider/telemetry çalışır | cache `256/256`, 1.000 request | provider `0`, writes `0`, recorded response `0` | BRK-R02 fixed |
| Admitted spam | ilk valid request | +1.000 duplicate +1.000 farklı rate-limited | total request `2.001`, provider `1`, writes `3` | Bounded |
| Telemetry failure spam | recorder başarılı | recorder error +1.000 duplicate | request `1.001`, provider `1`, write attempts `2`, selections `0` | Fail-closed + bounded |
| Provider success proof | recorder error → fallback, selection `0` | recorder başarılı | provider `1`, write `1`, selection `1`, recorded `true` | Kanıt korunuyor |
| Durable session | farklı durable session bağımsız | aynı durable session iki transport | max provider concurrency `1`; yeni durable session provider `1` | Savunuldu |
| Presence Start | ilk Start başarılı | ilk Start error + sonraki tool/concurrency | attempts `2`, tek published runtime/activity | Savunuldu |
| Presence End | ilk End başarılı | ilk error ve sonraki terminal trigger | transient recovery `2`; persistent bound trigger başına `3` | Savunuldu |

## BRK-R01 kapanışı — over-refusal değerlendirmesi

Scratch DB probunun sayıları:

| Input | Persist edilen client | Credential eşleşmesi |
| --- | --- | ---: |
| `codex / Codex Desktop / 2026.9` | `codex Codex` | 0 |
| key name'de | `unknown-client` | 0 |
| key title'da, `claude-code` name | `claude-code Claude Code` | 0 |
| key version'da, `cursor` name | `cursor Cursor` | 0 |
| güvenli `my-safe-custom-agent` | `unknown-client` | 0 |

Önceki Breaker bunu frozen requirement'ın raw name/version vaadine göre
over-refusal saymıştı. Final contract'a eklenen **Post-audit security amendment**
bu çatışmayı explicit çözüyor: arbitrary caller text'in hem verbatim korunup hem
credential-free olduğunun garanti edilemeyeceğini kabul ediyor; yalnız
self-reported `ClientInfo.name` üzerinden server-owned canonical family üretmeye,
unknown/custom adları `unknown-client` yapmaya ve raw title/version'ı hiçbir zaman
persist/display etmemeye karar veriyor.

README, Türkçe usage doc ve UI artık aynı policy'yi söylüyor. Kart wording'i
`CANONICAL CLIENT FAMILY · SOURCE: SELF-REPORTED CLIENTINFO NAME`; raw
name/version vaadi yok. Runtime ID unknown connection'ları birbirinden ayırmaya
devam ediyor. Bu nedenle safe unknown değerinin korunmaması, kabul edilmiş privacy
policy'si altında site-kıran over-refusal değil; beklenen veri minimizasyonudur.

**Class sweep:** `client_name`, `client_title`, `client_version`, Go snapshot model,
browser presentation, README ve `docs/usage-tr.md` birlikte incelendi. Raw title
ve version hiçbir presentation yolunda kullanılmıyor. Canonical family kaynağı
her okuyucuda self-reported name olarak açıklanıyor.

## BRK-R02 kapanışı — telemetry amplification

Yeni iki suppression katmanı ölçüldü:

1. Cache doluysa state kabul edilmiyor ve fallback telemetry hiç yazılmıyor.
2. Admitted state'te duplicate ve rate-limited telemetry, ayrı 30s/10s
   pencerelerinde yalnız birer kez kabul ediliyor.

| Kol | Request | Provider | Telemetry write/attempt | Tool output'ta recorded |
| --- | ---: | ---: | ---: | ---: |
| Normal success | 1 | 1 | 1 | 1 |
| Cache overflow | 1.000 | 0 | 0 | 0 |
| Admitted spam | 2.001 | 1 | 3 | 3 |
| Recorder sürekli error | 1.001 | 1 | 2 | 0 |

Telemetry failure ilk başarılı tavsiyeyi `telemetry_failed` fallback'e çevirip
selection'ı `1→0` temizledi. Sonraki duplicate spam yalnız ilk bounded proof
attempt'ini yaptı; DB hata durumunda her request tekrar yazmadı. Öte yandan
başarılı recorder kolunda accepted tavsiye kanıtsız bırakılmadı.

**Class sweep:** overflow, duplicate ve rate-limited sınıfları; başarılı ve hatalı
recorder; aynı/farklı payload; admitted/unadmitted session birlikte denendi.
Provider-disabled/fallback sonuçlarının output'u normal reasoning'e dönmeye devam
ediyor.

## B1 — Guard mutation

Mutasyonlar yalnız scratch source üzerinde uygulandı ve her biri sonrasında dosya
hash'i orijinal snapshot ile eşitlendi.

| # | Neutralize edilen guard | Beklenen kırmızı | Ölçülen sonuç |
| --- | --- | --- | --- |
| FM1 | cache-full branch'inde telemetry suppression'ı kaldır | overflow 0/0 olmamalı | RED: writes `0→1000`, recorded `0→1000` |
| FM2 | `admitFallbackTelemetry` sonucunu yok say | admitted/failure spam büyümeli | RED: admitted writes `3→2001`; failure attempts `2→1001` |
| FM3 | UI canonical-family privacy wording'ini raw self-reported name/version vaadine çevir | contract testi kırılmalı | RED: iki UI contract testi başarısız |

Üç koruma da davranış tarafından pinleniyor; green surviving mutation yok.

## B2 — Silinen davranış A/B ve compatibility

Base `21f525a` ve final snapshot ayrı binary olarak build edildi. Aynı bounded JSON,
boş environment override ve erişilemeyen disposable D-Bus/keyring ile çalıştırıldı:

| Binary | Exit | Output |
| --- | ---: | --- |
| `21f525a` | 0 | `fallback/credential_unavailable`, selections 0, version 1 |
| final | 0 | byte-for-byte aynı JSON |

Hidden `mindrail agent route` v1 no-key davranışı değişmedi. Canonical Python adapter
suite **46/46 PASS**. Schema-10 upgrade/shipped migration testleri PASS; additive
schema-11 ve eski row/idempotence davranışı önceki audit ölçümleriyle aynı kaldı.

## B3 — Threat model

| Tehdit | Yapılan saldırı | Sonuç |
| --- | --- | --- |
| Gerçek credential durable ClientInfo'ya girer | key name/title/version'ın her birine kondu | DB eşleşmesi 0 |
| Credential sıradan bilinen ada eşittir | `TYPESAFE_API_KEY=codex`, name=`codex` | safe placeholder; eşleşme 0 |
| Opaque/free-text channel | custom name/title/version ve high-entropy credential shapes | canonical/unknown dışında raw persistence yok |
| Cache-capacity write amplification | 256 active/inactive slot + 1.000 overflow | provider 0, write 0 |
| Admitted request flood | 2.001 request, clock ilerlemeden | provider 1, write 3 |
| Broken telemetry backend | ilk provider success + 1.000 duplicate | accepted advice kaçmadı; 2 attempt |
| Transport hopping | aynı durable session iki transport | serialization max 1 |
| Presence transient DB error | Start ve End ilk attempt fail | bounded retry ile recovery |

Gerçek TypeSafe ağına veya gerçek credential'a çıkılmadı; key olarak yalnız
disposable sentinel kullanıldı. Provider behavior canonical fake runner/opener
üzerinden ve 46 Python protocol testiyle çalıştırıldı.

## B4 — Upgrade path

`go test ./internal/migration ./migrations -run 'SchemaTen|Upgrade|Shipped|Migration' -count=1`
PASS.

Schema 10 old data → migration 11 → first read/write ve ikinci `Up` yolu yeniden
çalıştı. Önceki ayırıcı sonuç korunuyor: schema `10→11→11`, eski task row
`1→1→1`, telemetry table `0→2→2`, ikinci `Up` yeni migration uygulamadı.

## B5 — Weakest satisfying inputs

| Predicate | En zayıf ayırıcı input | Sonuç |
| --- | --- | --- |
| cache capacity `<256` | tam `256` dolu + tek yeni session | fallback, provider/write 0 |
| duplicate telemetry `<30s` | aynı payload, aynı timestamp | yalnız ilk duplicate write |
| rate telemetry `<10s` | farklı payload, aynı timestamp | yalnız ilk rate write |
| exact admission | 10.000s / 30.000s | provider yeniden kabul |
| credential collision | key tam canonical ad `codex` | canonical raw persist edilmedi |
| unknown canonical lookup | tek bounded printable custom name | `unknown-client`; amendment uyumlu |
| proof before use | recorder tek error | selection `1→0`, fallback |
| Start retry | ilk tek transient error + tek sonraki tool | ikinci Start başarılı |
| End retry | ilk tek transient error | ikinci attempt başarılı |

Amaç dışı accept veya site-kıran reject üretilmedi.

## B6 — Dual readers

- Store projection, Go dashboard collector ve browser presentation aynı canonical
  family inputuyla karşılaştırıldı.
- Bilinen alias `openai-codex` store'da `codex/Codex`; browser'da yalnız family
  `codex` gösteriyor.
- Unknown store'da `unknown-client`; browser'da `unknown-client`; runtime ID ayrı.
- Raw title/version store'da NULL ve browser presentation object'inde yok.
- Go ve browser 15s/30s status okuyucuları exact boundary'de yine aynı sonucu
  verdi: 15s/30s `CONNECTED`, +1ms sırasıyla `STALE`/`IDLE`, ended duration frozen.

`go test ./internal/dashboard` ve
`node --test internal/dashboard/testdata/client_behavior_test.cjs` PASS. Okuyucu
ayrışması üretilmedi.

## Mechanical rule = automated test

| Mekanik kural | Final otomasyon |
| --- | --- |
| Credential/raw ClientInfo durable alana girmez | Store attack tests |
| UI yalnız canonical family ve truthful source wording kullanır | Go embedded-contract + Node behavior tests; FM3 killed |
| Cache overflow telemetry yazmaz | 1.000-request test; FM1 killed |
| Admitted fallback telemetry pencere başına bounded | 2.001-request test; FM2 killed |
| Telemetry failure accepted advice'ı temizler | success/failure proof tests |
| Durable session transport'tan bağımsızdır | cross-transport tests |
| Presence Start/End transient error recovery bounded'dır | retry/concurrency tests |
| Heartbeat 5s, dashboard 15s/30s | exact literal ve dual-reader boundary tests |
| Hidden CLI v1/no-key uyumluluğu | base/final executable A/B |

## Maliyet

### Tek admitted session

- Provider: 10s minimum interval → **6/dakika, 360/saat**.
- Provider deadline: `360 × 10s = 3.600s` maksimum bekleme bütçesi/saat.
- 64 KiB input → **22.5 MiB/saat**; 256 KiB response → **90 MiB/saat**.
- Durable telemetry üst sınırı yaklaşık provider `6` + rate fallback `6` +
  duplicate fallback `2` = **14 write/dakika/session**.

### Process üst sınırı

- Cache en çok **256** admitted session tutar.
- Worst-case provider: `256 × 6 = 1.536/dakika`, **92.160/saat**.
- Worst-case route telemetry: `256 × 14 = 3.584 write/dakika`.
- Cache dışındaki session: provider **0**, telemetry **0**; request CPU'su hâlâ
  caller request sayısıyla büyür fakat external work/durable writes büyümez.

### Presence/dashboard

- Presence heartbeat: **12 DB write/dakika/runtime**.
- End: terminal trigger başına en çok **3 attempt**.
- Dashboard loopback bound değişmedi:
  `8 stream × 40 frame/dakika × 256 KiB = 80 MiB/dakika`.

Bu sınırlar ürünün local MCP/loopback modeli için önceki bulguyu yeniden üretmedi.
Process-wide 256-session provider çarpanı yüksek ama explicit cache ve per-session
contract'ın doğrudan sonucudur; bu auditte yeni regression değildir.

## Backward compatibility

- Existing no-key CLI output byte-for-byte aynı ve exit 0.
- Python adapter 46/46; normal reasoning fallback korunuyor.
- Schema 10 old DB upgrade/idempotence ve shipped migrations PASS.
- Eski client yeni 14. tool'u çağırmasa yaşam döngüsü bozulmuyor.
- Existing unknown/custom ClientInfo artık canonical `unknown-client`; bu raw
  identity compatibility kaybı explicit security amendment ve operator docs'ta
  kabul edilmiş breaking telemetry-policy değişimidir, gizli regresyon değildir.
- Focused packages tam çalıştırıldı:
  `go test ./internal/agent ./internal/mcp ./internal/dashboard ./internal/migration ./migrations -count=1`
  PASS.
- Race probu üç pakette PASS: agent `3.774s`, MCP `3.224s`, dashboard `1.452s`.

## Açık deneyler

Gerçek disposable TypeSafe sandbox credential sağlanırsa dış TLS/provider log ve
timeout davranışı ayrıca ölçülebilir. Bu turda gerçek ağa çıkılmadı. Local adapter,
provider-success proof, deadline/protocol suite ve persistence sınırları geçtiği
için bu release-blocking bir açık bulgu değildir.

## Final karar

**APPROVE.** Final remediation `BRK-R01` policy çatışmasını explicit güvenlik
amendment'ı ve truthful UI/docs ile kapatıyor; `BRK-R02` telemetry amplification'ı
hem overflow hem admitted/failing-recorder yollarında mekanik testlerle bounded
hale getiriyor. Temsilî eski HIGH/MEDIUM saldırılar yeniden üretilemedi. Critical,
HIGH, MEDIUM veya LOW açık bulgu kalmadı.

## İzolasyon kapanışı

- Orijinal çalışma ağacında yalnız bu `breaker-final.md` oluşturuldu.
- Implementation ve Reader final dosyaları değiştirilmedi; Reader final sonucu bu
  rapor yazılırken okunmadı.
- Scratch mutasyonları disposable copy ve DB'lerde kaldı.
- Shared runtime DB değiştirilmedi; audit task disposable runtime kopyasında tutuldu.
