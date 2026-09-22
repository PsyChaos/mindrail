# MR-008 — Bulgular ve kapı kaydı

Bu kayıt, dondurulmuş [gereksinimlerin](mr-008-requirements.md) TASK-01
kapısını kaydeder. Reader/Breaker değerlendirmelerini ve bu turdaki kanıtları
korur; gereksinim veya tasarımın yerine geçmez.

## Başlangıç ve çalışma ağacı

- Başlangıç HEAD'i `bf3cc05` idi (dondurulmuş gereksinimler + tasarım).
  §0 tablosundaki `make verify` ve `make tidy-check` yeşildi.

## TASK-01 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-01.1 | Karşılandı | `migrations/000007_scope_attribution.sql`: `scope_attributions` (key PK, FK, decider, reason, decided_at); başka nesne yok. |
| AC-01.2 | Karşılandı | `changes.TableSchemaVersion = 7`; 4 upgrade testi yeşil (schema-3/4/5/6 → 7). |
| AC-01.3 | Karşılandı | 3 kod registered, remedy'leri ayrık (`TestFindingTypesCarryCodesRemediesAndBlocking`), exit class'ları `ExitFailed`. |
| AC-01.4 | Karşılandı | shipped pin + ledger/shape/load kapsamı; knowledge dosyaları değişmedi; yeni komut/anahtar yok — yalnız `schema_version` 6→7. |
| AC-01.5 | Karşılandı | Yukarıdakilerin tümü (bilgi/komut/anahtar/zamanlama yok). |

`make check` **yeşil** (exit 0, ikinci koşu). Test sayısı **1124** (`go test -list '.*' ./... | grep -c '^Test'`).

## Reader / Breaker bulguları ve giderim — TASK-01 kapısı

(TASK-01 kapısı aşağıda.)

### TASK-01 guard mutasyon defteri (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M1 | tablo düşürüldü | pin + scope-map testleri FAIL (`TestEachMigrationCreatesOnlyItsMilestonesTables`, shape testi M1b ile ayrıca FAIL) |
| M2 | `TableSchemaVersion` 7 → 6 | gate testi FAIL |
| M3 | shipped pin çevrildi | pin testi FAIL |
| M4 | `allCodes`'tan kod çıkarıldı | exit-class testi FAIL |
| M5 | iki remedy paylaştırıldı | distinct-remedy testi FAIL (ilk deneme derlemeyi bozdu — geçersiz mutant; literal-paylaşım varyantı FAIL) |

Her mutant sonrası dosyalar backup'tan restore edilip md5 ile doğrulandı.

### TASK-01 kapı (commit sonrası bağımsız değerlendirme bekleniyor)
