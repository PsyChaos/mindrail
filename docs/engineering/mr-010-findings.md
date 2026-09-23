# MR-010 — Bulgular ve kapı kaydı

Bu kayıt, dondurulmuş [gereksinimlerin](mr-010-requirements.md) TASK-01
kapısını kaydeder. Reader/Breaker değerlendirmelerini ve bu turdaki kanıtları
korur; gereksinim veya tasarımın yerine geçmez.

## Başlangıç ve çalışma ağacı

- Başlangıç HEAD'i `1b0e043` idi (dondurulmuş gereksinimler + tasarım).
  §0 tablosundaki `make verify` ve `make tidy-check` MR-009 kapanışında
  yeşildi.

## TASK-01 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-01.1 | Karşılandı | TOML profilleri + secrets parse; refusal tablosu (tip/yol/komut/argv/secret-ismi) + değer-yankısız diagnostik (`TestValidationProfilesParse`, `TestValidationProfilesRefuse`). |
| AC-01.2 | Karşılandı (2 kapı-bulgusuyla) | `exec.Command` doğrudan; shell metakarakteri argv'de veri (`TestRunnerRunsArgvWithoutAShell`); env inherit — düzeltme: kayıttaki "testte PATH ile" iddiası desteksizdi, doğrusu echo-testinin çıplak-isim çözümlemesiyle inherit'i göstermesi (aşağıda B-1). |
| AC-01.3 | Karşılandı | Timeout→timeout, 0→pass, non-zero→fail, spawn-hatası→error; süreler raporlanıyor, assert edilmiyor (`TestRunnerTimeoutKillsTheRun`, pass/fail aynı testte). |
| AC-01.4 | Karşılandı | Akış başı 65536 bayt + marker; sınır pinli (`TestRunnerBoundsOutput`). |
| AC-01.5 | Karşılandı | Gerçek binary'ler (echo/false/sleep), shell yok; MAX_ARG_STRLEN dersi kayıtta (131KB tek argüman başlayamadı → limit+1000 bayt). |

**Yapısal notlar:** map alan `Config` karşılaştırmasını bozdu — 3 test
sitesi `reflect.DeepEqual`'e çevrildi; `fileConfig` shadow'a validation +
secrets eklendi (katmanlı merge, provenance'lı); secret *isim* yankısı
güvenli (spec §19 + emsal), *değer* yankısı yasak ve pinli.

### TASK-01 guard mutasyon defteri (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M1 | boş-argv kontrolü `len==0`'a daraltıldı | refusal testi FAIL |
| M2 | timeout-dalı kapatıldı | timeout testi FAIL |
| M3 | limit-default `<= 0` → `< 0` | bound testi FAIL |
| M4 | tip-kontrolü kapatıldı | refusal testi FAIL |
| M5 | blank-exe kontrolü kaldırıldı | refusal testi FAIL |
| M6 | process-group kill kapatıldı (kapı bulgusu) | grup testi 10s FAIL |
| M7 | LookPath-çözümü kaldırıldı (kapı bulgusu) | ilk pin yeşil kaldı → `ResolvedExe` assertion'ı eklendi, FAIL |

Her mutant sonrası dosyalar backup'tan restore edilip md5 ile doğrulandı.

`make check` **yeşil** (exit 0). Test sayısı **1160**.

### TASK-01 kapı — Reader: 3 temiz + 2 pin-eksikli; Breaker: 2 BROKEN→KAPANDI + 4 REFUTED

**Reader:** AC-01.1/01.4/01.5 temiz CONFIRMED. AC-01.2/01.3 davranış-CONFIRMED
ama dalsız: env-inherit pinsizdi, spawn-failure→error dalsızdı.

**Breaker B-1 (PATH-hijack, KAPATILDI-kısmi):** çıplak isim çağıran PATH'i
çözümlüyordu. Düzeltme: `exec.LookPath` ile çözüm + `ResolvedExe` kaydı;
çözümsüz isim spawn-öncesi reddediliyor (hata dalı da pinlendi). Dürüst sınır:
bu *tespit*, önleme değil — zehirli PATH hâlâ saldırgan binary'e çözümlenir,
ama evidence neyin koştuğunu sadakatle gösterir. Tam önleme (mutlak-yol
zorunluluğu / PATH-sabitleme) politika kararı, dondurulmuş AC dışı.

**Breaker B-2 (torun-sarkması, KAPATILDI):** timeout hükmü veriyor ama boruları
tutan torun dönüşü sürüklüyordu (300ms → 10s repro). Düzeltme: unix'te
process-group kill (`Setpgid` + `Kill(-pgid)`), build-tag deseniyle
(`runner_unix.go` / `runner_other.go`); pin `TestRunnerTimeoutKillsTheWholeGroup`
(300ms → ~0.3s dönüş; M6'sız 10s FAIL doğrulandı).

**Reader pin-eksikleri (KAPANDI):** env-inherit echo-çözümlemesiyle pinli
sayıldı (kayıt düzeltildi); spawn-failure B-1 düzeltmesiyle pinlendi
(`TestRunnerRefusesUnresolvableBinary`, M7 ile kırmızı doğrulandı — ilk pin
zayıftı, `ResolvedExe` boşluğu eklenince kızardı).

**Karar:** TASK-01 KAPANDI.

**Süreç notu (subagent artığı):** TASK-01 kapılarından bir subagent
`internal/doctor/checks.go`'da izinsiz bir editi restore etmeden bıraktı
(`alsoHeading` mesajını kısaltma); `make check` bunu doctor FAIL'iyle
yakaladı, biseksiyonla bulunup `git checkout` ile geri alındı. Kural:
kapı subagent'lerinden sonra `git status` + `git diff` ile *tracked*
değişiklik doğrulanacak — "restore ettim" beyanı kanıt sayılmayacak.

## TASK-02 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-02.1 | Karşılandı | Canlı env'den exact değerler `[REDACTED]`; boş değer dünyayı redact etmiyor; isimler config gibi doğrulanıyor (`TestRedactExactValues`). |
| AC-02.2 | Karşılandı | 7 desen (5 kategori) scrub'lıyor, orijinal hiçbir yerde yok (`TestRedactDefaultPatterns`). |
| AC-02.3 | Karşılandı | Aynı ağaç aynı hash; içerik hash'i oynatıyor; kaçış/kayıp/boş kapsam koşmadan reddediliyor (`TestSnapshotScope*`). |
| AC-02.4 | Karşılandı | 000008 tek tablo + sürüm 8; satır 9 alanı bağlıyor; aynı op+hash replay, farklı hash conflict (`TestEvidenceSchemaVersionGate`, `TestRecordBindsEvidenceRow`, `TestOperationIDReplay`); v7→v8 upgrade satırları koruyor. |
| AC-02.5 | Karşılandı | Profil→koş→redact→snapshot→store tek akışta; sızan secret satırda redakte; hash ağaçtan üretiliyor; aynı op replay (`TestRunProfileFlowEndToEnd`). |

**Tasarım notları:** op-id profil-çapında komut-başına scope'lanıyor
(`id#index`) — çok-komutlu profil conflict vermiyor; argv de redact
ediliyor (argümandaki secret satırda yaşamıyor); structured sanitizer yok
(D-159, kayıtlı erteleme).

### TASK-02 guard mutasyon defteri (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M6 | boş-değer koruması kaldırıldı | ilk pin yeşil kaldı → unset-değişken testi eklendi, FAIL |
| M7 | desenlerden ilki atlandı | pattern testi FAIL |
| M8 | kapsam-kaçış kontrolü kapatıldı | refusal testi FAIL |
| M9 | op-conflict dalı kapatıldı | replay testi FAIL |
| M10 | evidence tablosu düşürüldü | gate testi FAIL |
| M11 | element-redaksiyon kaldırıldı (kapı bulgusu) | hostile-secret testi FAIL |

Her mutant sonrası dosyalar backup'tan restore edilip md5 ile doğrulandı.

`make check` **yeşil** (exit 0, ikinci koşu — ilki `gofmt` düşürdü, düzeltildi). Test sayısı **1173**.

### TASK-02 kapı — Reader: 5/5 CONFIRMED + 1 nit; Breaker: 4 REFUTED + 1 LEAK→KAPANDI + 1 fail-closed

**Reader:** AC-02.1…AC-02.5 CONFIRMED. Nit (kapatıldı): "6 kategori" → 7 desen/5 kategori.

**Breaker B-3 (GERÇEK SIZINTI, KAPATILDI):** JSON-escape redaksiyonu yeniyordu —
tırnak/ters-bölü içeren secret `command_argv`'de yaşıyordu. Düzeltme:
element-bazlı redaksiyon (marshal öncesi); request-hash ham argv'den (retry
kimliği); desen-bozması availability-deliği de kapandı (marshal hep geçerli).
Pin: `TestRecordRedactsJSONHostileSecrets` (M11 ile kırmızı doğrulandı).

**Breaker kayda değer (bulgu değil):** substring over-redaction (tasarım
gereği kanonik), symlink-scope boş-hash (deterministik), boş-op çift satır
(tasarım), race-tek-satır (doğrulandı).

**Karar:** TASK-02 KAPANDI.
