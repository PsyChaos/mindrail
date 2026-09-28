# Reconciliation — Automatic JEV and Agent Presence

Date: 2026-09-28  
Base: `21f525a`  
Reader input: `reader.md`  
Breaker input: `breaker.md`  
Final verdict: **`NOT_VERIFIED`**

Bu turda uygulama dosyaları değiştirilmedi. Reader ve Breaker raporları yazılı
artifact olduktan sonra her önerilen bulgu için yokluk üretmeye çalışıldı. Bir
bulgunun testte “PASS” olması, saldırı senaryosunun beklenen kusurlu davranışı
ürettiği anlamındadır; aşağıdaki tablolar control/scenario sayılarını açıklar.

## 1. Sonuç özeti

| Finding | Son durum | Reconciled severity | İlgili gereksinim | Kısa sonuç |
| --- | --- | --- | --- | --- |
| BRK-001 | **CONFIRMED** | HIGH | REQ-004, REQ-006 | Self-reported üç ClientInfo alanı credential değerini ham olarak kalıcı DB'ye yazabiliyor. |
| BRK-002 | **CONFIRMED** | MEDIUM | REQ-010 | Route korumaları durable session yerine transport pointer'ına bağlı; hem yanlış cross-session throttle hem aynı-session concurrency üretiliyor. |
| BRK-003 | **CONFIRMED** | MEDIUM | REQ-006, REQ-007 | İlk presence Start hatası sonraki tool activity ile retry edilmiyor. |
| BRK-004 | **CONFIRMED** | MEDIUM | REQ-006, REQ-007 | İlk End hatası `sync.Once` yüzünden kalıcı olarak retriesiz kalıyor. |
| BRK-005 | **CONFIRMED** | LOW | REQ-006, REQ-012 | 5 s production heartbeat kuralı 6 s mutasyonunu öldüren testle pinlenmemiş. |
| RDR-001 | **CONFIRMED** | MEDIUM | REQ-005 | Validated fallback/no-match kararlarında güvenli ordinal/confidence kanıtı durable event'e taşınmıyor. |

Refuted finding yoktur. Open finding yoktur: altı bulgunun da davranışı veya test
boşluğu deterministik olarak üretildi. Breaker raporundaki gerçek-provider ve
process-kill deneyleri coverage genişletme fırsatlarıdır; bu altı finding'in
sonucunu belirlemek için gerekli değillerdir.

## 2. Üretilmiş refutation kanıtı

### BRK-001 — CONFIRMED / HIGH

**Refutation denemesi:** Bounded, printable bir credential değerinin ClientInfo
validasyonunda reddedildiğini veya DB'ye redacted yazıldığını üretmek.

**Çalıştırılan probe**

```text
GOCACHE=/tmp/reconcile-go-cache go test ./internal/agent \
  -run '^TestBreakerSelfReportedClientCanPersistCredentialShapedValue$' \
  -count=1 -v
```

**Sonuç:** rc=0; scenario saldırı beklentisini üretti.

| Kol | runtime row | DB `client_name` | `SUPER_SECRET` eşleşmesi |
| --- | ---: | --- | ---: |
| Control | 1 | `codex` | 0 |
| Scenario | 1 | `sk-live-SUPER_SECRET_123456789` | 1 |

`validClientField` yalnız byte sınırı, UTF-8, trim ve control-character kuralı
uyguluyor. `PresenceStore.Start` değeri `agent_runtimes` tablosuna olduğu gibi
yazıyor. Dashboard redaction'ı bu önceki kalıcı yazıyı ortadan kaldırmaz; keyring
credential'ının dashboard environment redactor listesinde olması da zorunlu
değildir. “Self-reported” etiketi kimliğin güven düzeyini doğru anlatır ama
credential saklama yasağını kaldırmaz.

**Severity reconciliation:** HIGH korunmuştur. Sonuç, açık REQ-004 credential
persist-etmeme sınırını ihlal eden kalıcı secret-at-rest kaydıdır. Saldırı yerel ve
caller-controlled olduğu için CRITICAL değildir; ancak dashboard/DB okuyucularına
sonradan açılabilen credential saklama davranışı LOW/MEDIUM seviyesine indirilemez.

**Class sweep closure:** Aynı root cause üç serbest metin alanını etkiler:
`client_name`, `client_title`, `client_version`. Route-event tablosunda serbest raw
metin alanı yoktur. Bu değişiklik kapsamında başka yeni self-reported persistence
alanı bulunmadı.

### BRK-002 — CONFIRMED / MEDIUM

**Refutation denemesi:** Route state'in durable `SessionID` sınırında sıfırlandığını
ve aynı durable session'a bağlı iki transport'un tek provider serialization/rate
state'i paylaştığını üretmek.

**Çalıştırılan probes**

```text
GOCACHE=/tmp/reconcile-go-cache go test ./internal/mcp \
  -run '^(TestBreakerThrottleCrossesDurableSessionBoundary|TestBreakerTwoConnectionsSameDurableSessionRunProviderConcurrently)$' \
  -count=1 -v
```

**Sonuç:** İki scenario da rc=0 ile kusuru üretti.

| Deney | Control | Scenario |
| --- | ---: | ---: |
| Yeni durable session provider call | fresh transport: 1, `ok` | reused transport: 0, `rate_limited` |
| Aynı durable session concurrency | tek transport maximum: 1 | iki transport maximum: 2 |
| Aynı-session scenario toplam provider call | — | 2 |

Public reachability de vardır: terminal automatic binding aynı connection üzerinde
yeni bootstrap ile değiştirilebilir; aynı run key restart/reconnect sırasında aynı
durable task/session olarak replay edilir. `routeSessions` ise sadece
`*sdk.ServerSession` ile anahtarlanır ve terminal binding değişiminde temizlenmez.

**Severity reconciliation:** MEDIUM korunmuştur. Gate/permission bypass yoktur;
etki yanlış fallback, gereksiz provider maliyeti ve aynı logical session'da
serialization ihlalidir.

**Class sweep closure:** Tek `routeSessionState` içindeki mutex, `lastProvider` ve
`recent` duplicate map aynı scope hatasından etkilenir. Başka route throttle/dedupe
uygulaması bulunmadı.

### BRK-003 — CONFIRMED / MEDIUM

**Refutation denemesi:** İlk `presence.Start` transient hatasından sonra normal tool
activity'nin ikinci bir Start denemesi oluşturduğunu üretmek.

```text
GOCACHE=/tmp/reconcile-go-cache go test ./internal/mcp \
  -run '^TestBreakerPresenceStartFailureIsNotRetriedByLaterToolActivity$' \
  -count=1 -v
```

**Sonuç:** rc=0 ile scenario üretildi.

| Kol | Start attempt | Başarılı binding | Activity hedefi |
| --- | ---: | ---: | ---: |
| Normal lifecycle control | 1 | 1 | 1 |
| İlk Start transient error + sonraki tool | 1 | 0 | 0 |

`ensurePresence` Start hatasını yutup boş binding bırakıyor; middleware'deki
`touchPresence` yalnız var olan binding üzerinde Activity çağırıyor. Sonraki tool
çağrısı Start retry'si değildir.

**Severity reconciliation:** MEDIUM korunmuştur. Agent çalışırken dashboard'dan
tamamen kaybolabilir; core engineering akışı durmaz ve veri/safety bypass oluşmaz.

**Class sweep closure:** Bootstrap, explicit-run resolve ve healthy retry bootstrap
çağrıları aynı `ensurePresence` yolunu kullanır. Start'ın bütün transient/permanent
hata türleri aynı retry eksikliğine düşer.

### BRK-004 — CONFIRMED / MEDIUM

**Refutation denemesi:** İlk durable End hatasından sonraki ikinci stop çağrısının
DB End'i tekrar denediğini üretmek.

```text
GOCACHE=/tmp/reconcile-go-cache go test ./internal/mcp \
  -run '^TestBreakerFailedEndIsNeverRetried$' -count=1 -v
```

**Sonuç:** rc=0 ile kayıp terminal yazısı üretildi.

| Scenario | `stopPresence` çağrısı | DB End attempt | Kalıcı ended state |
| --- | ---: | ---: | ---: |
| İlk End transient error, sonra ikinci stop | 2 | 1 | 0 |

`sync.Once` callback'in başarı durumunu değil çalıştırılmış olmasını saklıyor; End
hatası yutulduğu için heartbeat duruyor ve runtime ENDED yerine STALE kalıyor.

**Severity reconciliation:** MEDIUM korunmuştur. Disconnect/completion doğruluğu
bozulur fakat task completion/gate sonucu bozulmaz.

**Class sweep closure:** `completed`, `disconnected`, `replaced` ve `shutdown`
nedenlerinin tamamı `stopPresence` üzerinden geçer ve aynı lost-write sınıfına
tabidir.

### BRK-005 — CONFIRMED / LOW

**Refutation denemesi:** Production heartbeat default/fallback değerlerinin ikisini
5 s→6 s değiştirip mevcut `internal/mcp` testlerinin kırmızıya dönmesini beklemek.

Scratch copy:
`/tmp/mindrail-reconcile-heartbeat-YlhtpJ`

```text
GOCACHE=/tmp/reconcile-go-cache go test ./internal/mcp -count=1
```

**Sonuç:** rc=0, paket **GREEN** (`ok`, 11.616 s).

| Kol | Default | Fallback | Paket rc | Kırmızı test |
| --- | ---: | ---: | ---: | ---: |
| Control | 5 s | 5 s | 0 | 0 |
| Mutation | 6 s | 6 s | 0 | 0 |

**Severity reconciliation:** LOW korunmuştur. Bugün production değeri doğrudur;
bulgu regression guard eksikliğidir.

**Class sweep closure:** Mekanik 5 s değeri yalnız `Server.New` default'unda ve
`runPresenceHeartbeat` non-positive fallback'inde bulunur; mutasyon ikisini birlikte
değiştirdi. 15 s/30 s dashboard eşikleri ayrı exact-boundary testleriyle korunur.

### RDR-001 — CONFIRMED / MEDIUM

**Refutation denemesi:** Validated `low_confidence` ve `no_match` selection'larının
safe ordinal/confidence metadata'sının `routeDimension` çıktısında bulunduğunu
üretmek.

Scratch-only reconciliation testi:

```text
GOCACHE=/tmp/reconcile-go-cache go test ./internal/mcp \
  -run '^TestReconciliationFallbackTelemetryDropsSafeEvidence$' \
  -count=1 -v
```

**Sonuç:** rc=0 ve şu ölçümü üretti:

```text
low-confidence: count=2 ordinal=nil confidence=nil;
no-match: count=2 ordinal=nil confidence=nil
```

Provider adapter iki sonuçta da numeric confidence üretir; low-confidence/atomic
fallback'te caller-validated candidate da bulunur. Router bu confidence ve ordinal'i
typed result'ta korur. `routeDimension`, `Accepted == false` olduğunda her ikisini
erken return ile düşürür. Store/schema confidence-only shape'i zaten kabul eder.

**Severity reconciliation:** MEDIUM korunmuştur. Tavsiye yanlışlıkla tüketilmez ve
raw identifier sızmaz; sorun durable “actually invoked” kanıtının eksik olmasıdır.

**Class sweep closure:** `tool`, `agent`, `model` ve `effort` dimension'larının
tamamı aynı helper'dan geçer; no-match, low-confidence ve atomic-fallback'ın tamamı
etkilenir.

## 3. Refuted findings

Yok. Hiçbir bulgu için davranışın yokluğu üretilemedi.

## 4. Open findings

Yok. Altı finding'in tümü deterministik şekilde üretildi.

Audit coverage'ını artırabilecek fakat mevcut verdict'i değiştirmeyen iki deney:

1. Disposable gerçek TypeSafe credential/sandbox endpoint ile TLS round-trip ve
   provider-side log/disclosure ölçümü.
2. Wall-clock kontrollü E2E harness ile MCP subprocess `SIGKILL` sonrası gerçek
   dashboard'da 15.000 s / +1 ms geçişi.

## 5. Korunan kontroller ve geriye uyumluluk

Refutation turu şu savunmaları düşürmedi:

- accepted advice + telemetry write error → `fallback/telemetry_failed`, 0 selection;
- caller seti dışı provider candidate → fallback, 0 usable selection;
- ended runtime heartbeat → `ErrRuntimeEnded`, resurrection yok;
- Go/browser 15 s ve 30 s inclusive sınırları aynı sonucu veriyor;
- duplicate 29.999 s'de provider 0, 30.000 s'de provider 1;
- farklı request 9.999 s'de provider 0, 10.000 s'de provider 1;
- hidden `mindrail agent route` A/B çıktısı byte-for-byte aynı;
- schema 10→11 upgrade eski row'u 1→1 koruyor ve ikinci Up 0 migration uyguluyor;
- görünür MCP yüzeyi additive 13→14.

## 6. Final compliance decision

| Dimension | Result | Gerekçe |
| --- | --- | --- |
| Functional Requirements | PARTIAL | Route/presence/dashboard çalışıyor; BRK-003/004 ve RDR-001 bazı gerçek durumları eksik kaydediyor. |
| Technical Requirements | PARTIAL | Transport-vs-durable-session scope ve presence retry yaşam döngüsü sözleşmeden sapıyor. |
| Constraints | FAIL | BRK-001 credential-persistence yasağını ihlal ediyor. |
| Edge Cases | PARTIAL | Transient Start/End hataları ve reconnect/next-session sınırları başarısız. |
| Tests | PARTIAL | Full/targeted suite green; 5 s mekanik rule testi yok ve scenario testleri bug'ları üretiyor. |
| Scope Compliance | PASS | Audit edilen implementation alanı frozen work breakdown ile uyumlu. |
| Regression Safety | PARTIAL | Hidden CLI ve lifecycle regression'ları korunuyor; heartbeat constant regression'ı korunmuyor. |
| Backward Compatibility | PASS | CLI JSON, no-key akışı, old rows ve existing MCP tools korunuyor. |
| Cost / Worst Case | PARTIAL | Bir connection bounded; aynı durable session birden çok transport ile limiti çoğaltabiliyor. |

## 7. Final verdict

**`NOT_VERIFIED`**

Neden: BRK-001 confirmed HIGH olarak kalmıştır ve REQ-004 constraint'i başarısızdır.
Ayrıca dört MEDIUM bulgu REQ-005/006/007/010'un edge ve lifecycle doğruluğunu kısmen
bozar. Bu verdict, ana görünür route ve dashboard akışlarının çalışmadığı anlamına
gelmez; release gate'in “unresolved CRITICAL/HIGH yok” kabul koşulunun sağlanmadığı
anlamına gelir.

Bir sonraki adım audit finding'lerini implementation görevlerine dönüştürmek,
her düzeltme için control + scenario regression testini eklemek ve ardından
`make check`, `make gate` ile dual-agent audit'i yeniden çalıştırmaktır.
