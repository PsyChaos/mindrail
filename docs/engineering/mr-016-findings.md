# MR-016 — Bulgular ve kapı kaydı

Bu kayıt, dondurulmuş [gereksinimlerin](mr-016-requirements.md) TASK-01
kapısını kaydeder. Reader/Breaker değerlendirmelerini ve bu turdaki kanıtları
korur; gereksinim veya tasarımın yerine geçmez.

## Başlangıç ve çalışma ağacı

- Başlangıç HEAD'i `c4d3128` idi (dondurulmuş gereksinimler + tasarım).
  §0 tablosundaki `make verify` ve `make tidy-check` MR-015 kapanışında
  yeşildi.

## TASK-01 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-01.1 | Karşılandı | İsimli profil snapshot-bağlı evidence satırlarına koşuyor (`TestValidateRunsNamedProfile`). |
| AC-01.2 | Karşılandı | Bilinmeyen/boş profil spawn-öncesi reddediliyor, mesaj-pinli (`TestValidateRunsNamedProfile`). |
| AC-01.3 | Karşılandı | Newest-first liste + statülü binding okuma; davranışlar değişmedi (`TestEvidenceForProfileOrder`, `TestBindingsWithStatusReadsAll`). |

**Süreç notu (backup disiplini):** M2/M3 oturumunda backup mutant
üzerine yazıldı; tam-suite FAIL ile yakalandı, dosyalar elle onarıldı,
M2/M3 temiz-ağaçta yeniden kırmızı doğrulandı (md5'li restore). Kural
pekişti: backup mutant-öncesi alınır, asla sonrası; restore sonrası
tam-suite koşulur.

### TASK-01 guard mutasyon defteri (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M1 | profil-kontrolü kaldırıldı | ilk varyant maskeli → mesaj-pini eklendi, FAIL |
| M2 | sıralama tersine çevrildi | order testi FAIL (yeniden-doğrulamalı) |
| M3 | statü filtresi daraltıldı | bindings testi FAIL (yeniden-doğrulamalı) |
| M4 | budget-kontrolü kapatıldı | version-error testi FAIL |

Her mutant sonrası dosyalar backup'tan restore edilip md5 ile doğrulandı.

`make check` **yeşil** (exit 0). Test sayısı **1232**.

### TASK-01 kapı (commit sonrası bağımsız değerlendirme bekleniyor)
