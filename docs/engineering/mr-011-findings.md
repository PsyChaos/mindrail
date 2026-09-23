# MR-011 — Bulgular ve kapı kaydı

Bu kayıt, dondurulmuş [gereksinimlerin](mr-011-requirements.md) TASK-01
kapısını kaydeder. Reader/Breaker değerlendirmelerini ve bu turdaki kanıtları
korur; gereksinim veya tasarımın yerine geçmez.

## Başlangıç ve çalışma ağacı

- Başlangıç HEAD'i `8d62503` idi (dondurulmuş gereksinimler + tasarım).
  §0 tablosundaki `make verify` ve `make tidy-check` MR-010 kapanışında
  yeşildi.

## TASK-01 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-01.1 | Karşılandı | Dokunulmamış kapsam eşit re-hash → current (`TestCheckKeepsCurrentOnUntouchedScope`). |
| AC-01.2 | Karşılandı | İçerik/silinen/okunamaz kapsam stale, gerekçe boşluğu adlandırıyor (`TestCheckStalesRelevantEdits`). |
| AC-01.3 | Karşılandı | Kapsam-dışı edit (eklenen dosya dahil) current bırakıyor — dosya-taneli kapsam yorumu, kapıda (`TestCheckIgnoresOutOfScopeEdits`). |
| AC-01.4 | Karşılandı | Satisfied/stale-only/missing + sıralı union re-run, gerekçeli (`TestCheckRequiredCoverage`). |
| AC-01.5 | Karşılandı | Check DB tutmuyor (yapısal); determinizm pinli (`TestCheckIsDeterministic`). |

**Tasarım notları:** hash çekirdeği paylaşımlı (`hashFiles` — MR-010
refaktörü, davranış aynı, mevcut testler yeşil); `Check` root'u parametre
alıyor (MR-010 provenance'ı değişmedi); helper sahte-scope kuruyordu
(genişletilmemiş dizin) → gerçek `snapshot.Scope` kullanılıyor.

### TASK-01 guard mutasyon defteri (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M1 | eşitlik `==` → `!=` | current testi FAIL |
| M2 | kaçış-kontrolü kapatıldı | escape testi FAIL |
| M3 | coverage hep-satisfied | coverage testi FAIL |
| M4 | stale-remember kapatıldı | ilk varyant maskeli (required döngüsü geri ekliyor) → required-dışı assertion eklendi, FAIL |
| M5 | eşitlik `!= ""`a gevşetildi | stale testleri FAIL |

Her mutant sonrası dosyalar backup'tan restore edilip md5 ile doğrulandı.

`make check` **yeşil** (exit 0). Test sayısı **1180**.

### TASK-01 kapı (commit sonrası bağımsız değerlendirme bekleniyor)
