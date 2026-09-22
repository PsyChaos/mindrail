# MR-006 — Bulgular ve kapı kaydı

Bu kayıt, dondurulmuş [gereksinimlerin](mr-006-requirements.md) TASK-01
kapısını kaydeder. Reader/Breaker değerlendirmelerini ve bu turdaki kanıtları
korur; gereksinim veya tasarımın yerine geçmez.

## Başlangıç ve çalışma ağacı

- Başlangıç HEAD'i `b902237` idi (dondurulmuş gereksinimler + tasarım).
  §0 tablosundaki `make verify` ve `make tidy-check` yeşildi.

## TASK-01 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-01.1 | Karşılandı | `migrations/000005_symbol_identity.sql`: `symbol_identities` (uid PK, D-94 UNIQUE key, `previous_keys` lineage), `invariant_symbol_bindings` (composite-pair UNIQUE, status CHECK), `symbol_identity_ambiguities` ve `ALTER TABLE symbols ADD COLUMN symbol_uid` (nullable); başka nesne yok. |
| AC-01.2 | Karşılandı | `index.TableSchemaVersion = 5`; `TestAnUpgradedDatabaseGainsSymbolIdentityWithoutLosingSymbols` schema-4 DB'yi 5'e yükseltir, MR-005 satırını uid-less korur. Eski schema-3 testi 5'e güncellendi. |
| AC-01.3 | Karşılandı | `SYMBOL_IDENTITY_AMBIGUOUS` + `ORPHANED_PROTECTED_SYMBOL` registered, remedy'leri ayrık (`TestIdentityCodesCarryDistinctRemedies`), exit class'ları `ExitFailed` (`TestExitClassTableCoversEveryRegisteredCode`). |
| AC-01.4 | Karşılandı | `shipped_test.go` 000005 byte checksum'ını pinler; ledger/shape testleri 3 tablo + `symbols.symbol_uid` addition'ını kapsar; `tablesPerMilestone` 5. girdiyi taşır. |
| AC-01.5 | Karşılandı | Knowledge dosyaları değişmedi; yeni komut/anahtar yok — yalnız `schema_version` 4→5 sayısı golden'ları sürdü (yerleşik precedent). |

Raporlanan doğrulama komutları: `make check`, `make tidy-check` ve
`go test -list '.*' ./... | grep -c '^Test'`. Son kabulde bağımsız delta
değerlendirmesi Reader **PASS**, Breaker **VERIFIED** sonucuna ulaştı.

## Reader / Breaker bulguları ve giderim

(TASK-01 kapısı aşağıda.)

### TASK-01 guard mutasyon defteri (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M1 | migration: `idx_identity_alloc` UNIQUE → plain INDEX | `TestIdentityAllocationKeyIsUniquePerProjectUnitLanguageKey` FAIL (ikinci uid kabul edildi) |
| M2 | `allCodes`'tan `CodeSymbolIdentityAmbiguous` çıkarıldı | `TestExitClassTableCoversEveryRegisteredCode` FAIL |
| M3 | `TableSchemaVersion` 5 → 4 | `TestIndexSchemaVersionNamesItsCreatingMigration` FAIL |
| M4 | bindings status CHECK kaldırıldı | `TestIdentityBindingStatusAndSymbolUidAreConstrained` FAIL (`maybe` kabul edildi) |

Her mutant sonrası dosyalar backup'tan restore edilip md5 ile doğrulandı;
shipped pin'i mutant-öncesi baytlardan hesaplandığı için geçerliliğini korur.

## Reader / Breaker bulguları ve giderim — TASK-01 kapısı

Bağımsız Reader **FAIL**, Breaker **BLOCKED** — ikisi de tek HIGH bulguda
birleşti, ikisi de başka engel bulamadı (Breaker 5 öz mutant + şema-kırma
probları temiz; non-finding F3: CLI yüzeyi değişmedi).

- **HIGH/F1 (kırmızı süit):** `store_test.go:466` `required_version == "4"`
  pini `TableSchemaVersion = 5` sonrasında güncellenmemişti;
  `TestStoreGatesHealthyOlderSchemaBeforeAnyIndexOperation` 5 alt testte
  düşüyordu. Giderim: assertion `"5"` + `v4` etiketleri `v5` oldu; focused
  süit yeşil.
- **MEDIUM/F2 (guard açığı):** `container_uid` FK'sinin davranış-seviyesi
  guard'ı yoktu (yalnız byte pin). Giderim:
  `TestIdentityContainerUidRejectsDanglingParent` eklendi; FK kaldırma
  mutantı FAIL.
- **LOW (ifade):** AC-01.1 "composite PK" diyordu, şema UNIQUE index
  kullanıyor (tasarım §5 yetkili, işlevsel eşdeğer). Gereksinim cümlesi kapı
  commit'inde düzeltildi; şema değişmedi.

### TASK-01 guard mutasyon defteri — ek (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M5 | `container_uid` REFERENCES kaldırıldı | `TestIdentityContainerUidRejectsDanglingParent` FAIL |

### TASK-01 kapı — Reader PASS, Breaker VERIFIED (remediasyon sonrası)

Giderim sonrası focused süitler yeşil; tam `make check` aşağıda. Test sayısı
`go test -list '.*' ./... | grep -c '^Test'` ile **1032**.
