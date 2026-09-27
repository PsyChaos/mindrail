# Client-neutral JEV — Breaker Report

Tarih: 2026-09-27  
Rol: Breaker (parallel dual-agent audit)  
Base: `cd0d1e1e5c2fc2557ec4aa717b118f32e0d65691`  
Kapsam: `docs/engineering/client-neutral-jev-credentials-2026-09-27.md`

## 1. Sonuç

**Breaker verdict: APPROVE WITH MINOR TEST GAPS.** Çalışan sistemde güvenlik
korumasını aşan bir hata üretilmedi. İki gerçek korumanın kaldırılması mevcut paket
testlerini kırmadı; ayırt edici Breaker testleri ise iki korumanın da gerekli ve
çalışır olduğunu gösterdi. Bu nedenle bulgular ürün açığı değil, REQ-008 kapsamındaki
regresyon testi boşluklarıdır.

Denetim mevcut çalışma ağacının ayrı scratch kopyasında yürütüldü. Uygulama
dosyalarının tüm mutasyonları yalnız bu kopyada yapıldı. Kullanıcının çalışma ağacında
uygulama kodu değiştirilmedi; bu rapor tek kalıcı Breaker çıktısıdır. Gerçek kullanıcı
keyring'i kullanılmadı: A/B denemesinde DBus adresi kasıtlı olarak erişilemez yapıldı,
yükseltme denemesinde yalnız ortam override'ı kullanıldı.

## 2. Altı zorunlu hareket

| Hareket | Durum | Üretilen kanıt |
| --- | --- | --- |
| B1 — Guard mutation | Çalıştırıldı, kapsamlı otomatik tarama sınırlı | Origin ve token guard'ları silinince testler kırıldı. API key üst sınırı ve exact Host guard'ları silinince mevcut `internal/jevconnect` testleri yeşil kaldı; ayırt edici testlerle davranış farkı üretildi. Harness'in tek otomatik survivor'ı elle refute edildi. |
| B2 — Silinen davranış A/B | Çalıştırıldı | Eski ve yeni binary aynı girdide çalıştırıldı. Erişilemez credential servisi altında eski sürüm `disabled/api_key_missing`, yeni sürüm tasarımla uyumlu `fallback/credential_unavailable` döndürdü. Her iki unstamped binary `dev` bildirdi. İstenmeyen silinmiş davranış üretilmedi. |
| B3 — Yazılı threat model | Çalıştırıldı | Hostile/missing Origin ile guard silindiğinde 5/5 istek `200` ve birer store yazımı yaptı; guard varken test beklentisi `403`, sıfır yazımdı. Token guard'ı silindiğinde wrong-token isteği `200`, bir yazım yaptı; guard varken `404`, sıfır yazım bekleniyor. |
| B4 — Upgrade path | Çalıştırıldı | Base binary ile eski repo init edildi; base `status` exit `0`, aynı state üzerinde yeni binary `status` exit `0`. Yeni binary environment override ile `jev status --json` için `connected:true, source:environment` verdi. |
| B5 — Weakest satisfying input | Çalıştırıldı, tanımlı sözleşmeyle sınırlı | Credential okuyucularına `""`, `" "`, NBSP ve `" x "` verildi. İlk üçü iki okuyucuda da missing/disconnected; sonuncusu iki okuyucuda da available/connected oldu. TypeSafe anahtarının sağlayıcıya özgü biçimi gereksinimde tanımlanmadığından biçim doğrulaması iddia edilmedi. |
| B6 — Dual readers | Çalıştırıldı | `resolveJEVStatus` ve `resolveJEVKey` aynı dört ortam girdisinde bağlantı kararı bakımından 4/4 aynı sonucu verdi. Ayrıntılar aşağıdadır. |

## 3. Onaylı bulgular

### BREAKER-001 — API key üst sınırı guard'ının regresyon testi yok

**Severity:** LOW  
**Confidence:** HIGH  
**Requirement:** REQ-003, REQ-005, REQ-008  
**File:** `internal/jevconnect/connect.go`  
**Symbol:** `(*connectHandler).acceptKey`

**Deneme:** Scratch kopyada `len(key) > maxAPIKey` koşulu kaldırıldı. Önce mevcut
`go test ./internal/jevconnect -count=1` çalıştırıldı; ardından aynı handler'a
`maxAPIKey+1` baytlık anahtar gönderen ayırt edici test çalıştırıldı.

| Ölçüm | Kontrol — guard var | Senaryo — guard silinmiş | Delta |
| --- | ---: | ---: | ---: |
| Mevcut paket testi exit | 0 | 0 | 0 |
| Ayırt edici POST HTTP status | 400 | 200 | +200 |
| Credential store `Set` sayısı | 0 | 1 | +1 |

**Beklenen:** 4097 baytlık key reddedilmeli ve store çağrılmamalı.  
**Gerçek ürün:** Guard varken tam olarak bunu yapıyor.  
**Bulgu:** Mevcut test yalnız body üst sınırını aşıyor; key'e özel 4096/4097 sınırını
sabitlemiyor. Guard sessizce kaldırılabilir.

**Class sweep:** Aynı dosyada `maxRequestBody` ve `maxAPIKey` sınırları arandı.
`maxRequestBody` için oversized istek testi mevcut; `maxAPIKey+1` için başka test
bulunmadı.

**Önerilen düzeltme:** Ayırt edici 4096 kabul / 4097 ret tablosunu kalıcı
`internal/jevconnect/connect_test.go` testine eklemek. Fix uygulanmadı; Breaker
uygulama kodunu değiştirmez.

### BREAKER-002 — Exact Host guard'ının regresyon testi yok

**Severity:** LOW  
**Confidence:** HIGH  
**Requirement:** REQ-005, REQ-008  
**File:** `internal/jevconnect/connect.go`  
**Symbol:** `(*connectHandler).ServeHTTP`

**Deneme:** Scratch kopyada `r.Host != h.host` koşulu kaldırıldı. Mevcut connect,
store ve browser testleri çalıştırıldı. Ardından doğru path/token fakat
`Host: attacker.invalid` taşıyan GET aynı handler'a gönderildi.

| Ölçüm | Kontrol — guard var | Senaryo — guard silinmiş | Delta |
| --- | ---: | ---: | ---: |
| Mevcut seçili paket testi exit | 0 | 0 | 0 |
| Ayırt edici GET HTTP status | 404 | 200 | +196 |

**Beklenen:** Exact Host uyuşmazlığı `404` olmalı.  
**Gerçek ürün:** Guard varken `404` üretiyor.  
**Bulgu:** Wrong path ve wrong token testleri var; wrong Host girdisi yok.

**Class sweep:** Exact path, token ve Origin okumaları aynı handler'da tarandı.
Wrong path, wrong token ve hostile Origin testleri mevcut; test boşluğu yalnız exact
Host kolunda üretildi.

**Önerilen düzeltme:** Ayırt edici Host GET testini kalıcı suite'e eklemek. Fix
uygulanmadı.

## 4. Refute edilen bulgu

| Aday | Refuting evidence |
| --- | --- |
| Harness survivor: `if (result.Status == "disabled") == *result.Enabled` | Harness koşulu gerçekten silmek yerine `if (false) == *result.Enabled` yaptı; bu, `enabled:false` girdisinde hâlâ true'dur. Koşul elle `if false` ile gerçekten kaldırılınca `TestAgentRouteRejectsInconsistentFallbackStatus` exit `1` oldu ve inconsistent fallback passthrough ölçüldü. Bu guard testlidir. |

## 5. Kontrol / senaryo ayrıntıları

### Origin guard mutation

| Ölçüm | Kontrol | Guard silinmiş |
| --- | ---: | ---: |
| Hostile/missing metadata alt senaryosu | 5 | 5 |
| Reddedilen istek | 5 | 0 |
| Kabul edilen ve store yazan istek | 0 | 5 |
| Test exit | 0 | 1 |

### Token guard mutation

| Ölçüm | Kontrol | Guard silinmiş |
| --- | ---: | ---: |
| Wrong-token HTTP status | 404 | 200 |
| Store yazımı | 0 | 1 |
| Test exit | 0 | 1 |

### Dual-reader girdileri

| Girdi | Status reader | Route reader | Uyuşma |
| --- | --- | --- | --- |
| `""` | disconnected / none | missing / key len 0 | evet |
| `" "` | disconnected / none | missing / key len 0 | evet |
| NBSP | disconnected / none | missing / key len 0 | evet |
| `" x "` | connected / environment | available / key len 3 | evet |

## 6. Off-spec kontroller

### Cost / worst case

Tanımlı tek POST için üst sınırlar: `1 × 8192` bayt request body,
`1 × 4096` bayt credential mutation, mutation context'i `10s`, connect penceresi
`300s`; dolayısıyla başarılı bir oturumda credential store yazım fan-out'u
`1 × 1 = 1`'dir. HTTP server header/read/write/idle süreleri sırasıyla
`5s/15s/15s/15s`.

GET istek sayısına kod içi bir üst sınır yoktur; 300 saniyelik pencere içindeki
teorik sayı `300s × ulaşılabilir request_rate` olup request rate sınırı
tanımlanmadığından sonlu bir kod sınırı hesaplanamaz. Kaynak tüketimi başarısızlığı bu
turda üretilmedi; aşağıdaki open item'a taşındı.

### Mechanical rule = automated test

Loopback, Origin/Fetch Metadata, token, tek kullanım, sanitized failure, browser
environment ve credential timeout kuralları için otomatik testler gözlendi ve kritik
Origin/token örnekleri mutasyonla kırmızıya döndü. Exact Host ve key-specific 4096
bayt sınırı prose/code mekanik kuralı olmasına rağmen kalıcı testle sabitlenmemiştir;
BREAKER-001 ve BREAKER-002.

### Backward compatibility

Somut kombinasyon: base binary ile init edilmiş git repo ve runtime state, yeni binary
ile ilk `status`. Kontrol ve senaryo exit kodları `0/0`. Repo veya config migration
kırılması üretilmedi. Credential bulunamadığında base `disabled/api_key_missing`,
yeni sürüm erişilemez credential servisini `fallback/credential_unavailable` olarak
ayırt ediyor; bu REQ-006'nın typed fail-open davranışıdır ve iki durumda da agent
normal akışa devam eder.

## 7. Açık öğeler

| Öğe | Neden üretilmedi | Sonuçlandıracak tam deney |
| --- | --- | --- |
| Gerçek Linux Secret Service ve Windows Credential Manager entegrasyonu | İzole, disposable OS credential service yoktu; gerçek kullanıcı keyring'ine dokunmak B0'ı ihlal ederdi. | Disposable Linux session bus + temporary Secret Service başlat; `mindrail jev connect/status/disconnect` round-trip'ını sentinel key ile çalıştır, servis içeriğini ve stdout/stderr/repo taramasını ölç. Windows'ta disposable kullanıcı/VM üzerinde aynı deneyi Credential Manager ile tekrarla. |
| GET flood maliyeti | Bu turda yük testi başlatılmadı. | Scratch binary'yi disposable network namespace'te çalıştır; tokenlı GET'i 300s pencere içinde artan concurrency ile gönder; goroutine/RSS/FD ölçülerini boş kontrol koluyla karşılaştır ve OS limitinden önce Mindrail'in kod içi sınırı olup olmadığını kaydet. |
| Linux `xdg-open` hostile HOME seçimi | Üretilen probe'da `/usr/bin/xdg-open` exit `3` verdi ve marker çalışmadı; bu sonuç saldırıyı ispatlamadı. | Disposable masaüstü oturumunda saldırgan HOME altında `mimeapps.list` ve `.desktop` handler oluştur; aynı allowlisted env ile `openBrowser` çalıştır; kontrol gerçek HOME, senaryo hostile HOME; handler execution sayısını ölç. |
| Tam otomatik tüm-guard mutation | Go'nun çoğu guard'ı parantezsiz olduğu için sağlanan genel harness yalnız bir satırı tanıdı; kullanıcı isteğiyle denetim genişletilmeden sonlandırıldı. | Go AST tabanlı mutant ile base diff'teki her yeni `if`, switch validation ve bound check'i tek tek etkisizleştir; her mutant için ilgili paket suite'ini çalıştır ve survivor'lara ayırt edici input ekle. |

## 8. Test değerlendirmesi

Başarılı kontrol komutu:

```text
go test ./internal/credential ./internal/jevconnect ./internal/cli \
  -run 'JEV|AgentRoute|Browser|Credential|CommandSurface|RootHelp' -count=1
```

Üç paket de geçti. İlk daha geniş scratch koşusu, Go build cache'in kaynak yolunu
orijinal repo olarak taşıması nedeniyle kapsam dışı runtime-path testlerinde hata
verdi; ayrı scratch cache ile JEV odaklı kontrol yeşil oldu. Bu ilk sonuç ürün
regresyonu olarak sayılmadı.

Mutation özeti:

| Koruma | Sonuç |
| --- | --- |
| Same-origin / Fetch Metadata | killed |
| Exact token | killed |
| Adapter status/enabled tutarlılığı | killed (harness false survivor refute edildi) |
| API key 4096 sınırı | survived existing suite; BREAKER-001 |
| Exact Host | survived existing suite; BREAKER-002 |

## 9. Nihai Breaker kararı

Uygulamanın çalışan güvenlik davranışı yapılan saldırılarda korundu; upgrade ve dual
reader farkı üretilmedi. İki düşük önem dereceli mekanik test boşluğu nedeniyle
Breaker sonucu **APPROVE WITH MINOR TEST GAPS**. Bu rapor Reader sonucunu okumadan
hazırlandı.
