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


## TASK-02 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-02.1 | Karşılandı | `TestEnsureIdentityIsStableAcrossCalls`: iki çağrı aynı `SYM-` uid, identities tablosunda 1 satır. |
| AC-02.2 | Karşılandı | `TestConcurrentFirstSightAgreesOnOneUID`: ayrı DB handle'ları üzerinden 16 yarışçı tek uid + 1 satır (UNIQUE index hakem). |
| AC-02.3 | Karşılandı | `TestBackfillStampsWithoutReparse`: SQL-tohumlu 3 satır damgalanır (anahtar başına ortak uid), ikinci koşu 0 damgalar; pakette parser inşası yok. |
| AC-02.4 | Karşılandı | `TestCompletionKeepsUIDsAcrossIdenticalRewrites`: iki rewrite aynı uid kümesi. |
| AC-02.5 | Karşılandı | `TestOverloadsSharingAKeyShareOneUID`: aynı anahtarlı 2 satır, 1 uid, 1 identity satırı. |

Plumbing: `FileFacts.ProjectID` zorunlu (fail-closed), `IndexFile` ve
scheduler (`Job`/`Enqueue`/`FillCold`/`Prioritize`) projectID thread'ler;
33 literal + 36 çağrı güncellendi. `symbol.Service` kuruldu (lookup→mint;
migration kancası TASK-04'ün).

## Reader / Breaker bulguları ve giderim — TASK-02 kapısı

(TASK-02 kapısı aşağıda.)

### TASK-02 guard mutasyon defteri (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M1 | `MintIdentity`: ON CONFLICT kaldırıldı (plain INSERT) | concurrent test FAIL (loser UNIQUE ihlali) |
| M2 | `BackfillUnit`: stamp `AND 1 = 0` ile etkisiz | backfill testi FAIL (0 damga). İlk M2 derlemeyi bozdu (geçersiz mutant), derlenen varyantla tekrarlandı. |
| M3 | completion INSERT uid yerine NULL + arg düşürüldü | overload/completion testleri FAIL |
| M4 | `validReplacement` ProjectID şartı kaldırıldı | guard testi FAIL (önce SURVIVED kaldı — eksik-project durumu dosya-satırsız test ediliyordu; test dosya-satırı tohumlayıp usage kodu denetleyecek şekilde güçlendirildi) |
| M5 | `IndexFile` project kontrolü kaldırıldı | `TestIndexerRejectsEmptyProject` FAIL (bu test M5 için eklendi) |
| M6 | `symbol.New` nil kontrolü kaldırıldı | guard testi FAIL |

Her mutant sonrası dosyalar backup'tan restore edilip md5 ile doğrulandı.

## Reader / Breaker bulguları ve giderim — TASK-02 kapısı

Bağımsız Reader **PASS** (AC-02.1…02.5 MET; §24, D-95, fail-closed projectID
doğrulandı), Breaker **VERIFIED** (32×3 yarış -race temiz; 5 öz mutanttan
4'ü kırmızı, 1'i benign-perf; churn temiz).

- **Breaker F1 (LOW, doğrulandı):** yetim uid-less satırları (dosya-satırsız)
  backfill sessizce atlıyor, yorum aksi iddiadaydı. Giderim: yetim sayımı
  fail-closed hataya çevrildi + `TestBackfillRefusesOrphanSymbols` eklendi;
  M7 (koruma kaldırma) FAIL.
- **Reader F1 (LOW):** CAS-yolu uid assertion'ı yok (yalnız construction +
  churned literal'lar). TASK-04'e devir: completion yolu zaten
  değişeceğinden CAS uid assertion'ı orada eklenecek.
- **Reader F2:** "6 test" ifadesi 7 fonksiyonu saymıyor (5 AC + 2 guard).
  Kayıt düzeltildi: yedi test (AC-02.1…02.5 + 2 guard).
- **Reader F3 (çürütüldü):** "uncommitted store.go değişikliği" — ağaç
  commit ile birebir temiz (`git status` boş); pre-existing `stash@{0}`
  MR-002 döneminden, dokunulmadı.
- **Breaker C (benign):** stamping dedup kaldırma tüm süiti yeşil bırakır —
  `ensureIdentityTx` idempotent olduğundan dedup perf-only'dir; guard açığı
  değil, kayıttadır.
- **Breaker F2/F3 (gözlem):** cross-project stamp first-project-wins; yabancı
  unit objesi stray identity mintler (FK dışı). İkisi de 0.1 tek-proje
  varsayımında zararsız; TASK-04 caller-sözleşmesi olarak kayıttadır.

### TASK-02 guard mutasyon defteri — ek (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M7 | backfill yetim-reddi kaldırıldı | `TestBackfillRefusesOrphanSymbols` FAIL |
| M2-ilk | (geçersiz) `UPDATE` sökümü derlemeyi bozdu | derlenen `AND 1 = 0` varyantı (M2) FAIL |

### TASK-02 kapı — Reader PASS, Breaker VERIFIED (remediasyon sonrası)

Giderim sonrası focused süitler yeşil; tam `make check` aşağıda. Test sayısı
`go test -list '.*' ./... | grep -c '^Test'` ile **1040**.

`make check` yeşil (exit 0). Test sayısı `go test -list .\* ./... | grep -c ^Test` ile **1039**.

### TASK-01 kapı — Reader PASS, Breaker VERIFIED (remediasyon sonrası)

Giderim sonrası focused süitler yeşil; tam `make check` aşağıda. Test sayısı
`go test -list '.*' ./... | grep -c '^Test'` ile **1032**.
