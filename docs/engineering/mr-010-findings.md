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
| AC-01.2 | Karşılandı | `exec.Command` doğrudan; shell metakarakteri argv'de veri (`TestRunnerRunsArgvWithoutAShell`); env inherit (kodda, testte PATH ile). |
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

Her mutant sonrası dosyalar backup'tan restore edilip md5 ile doğrulandı.

`make check` **yeşil** (exit 0). Test sayısı **1160**.

### TASK-01 kapı (commit sonrası bağımsız değerlendirme bekleniyor)
