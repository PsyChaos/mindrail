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

## TASK-03 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-03.1 | Karşılandı | `TestResolveTargetScopeForms` (FILE/fonksiyon/metot/çift-tür-çiftliği/bilinmeyen/kapsayıcısız/çıplak-metot/dosya-fanout/birim-dışı), `TestResolveContainerCollisionIsAmbiguous`, `TestRefreshBindsPrefixScopes` (PACKAGE/MODULE + `pkg2` ayraç guard'ı). |
| AC-03.2 | Karşılandı | `TestRefreshBindingsWritesBoundRows` (bound + idempotent re-refresh) ve `TestStickyBindingSurvivesStaleTargetText` (D-110: canlı lineage bayat metne rağmen bound kalır). |
| AC-03.3 | Karşılandı | `TestRefreshDefersColdFiles`: satır yok, bulgu yok, blok yok. |
| AC-03.4 | Karşılandı | `TestOrphanBlocksOnCritical` (CRITICAL bloklar, LOW bloklamaz, kod `ORPHANED_PROTECTED_SYMBOL`) ve `TestOutsideUnitTargetIsOrphaned` (D-110: birimsiz yol orphan izler, satırsız). |
| AC-03.5 | Karşılandı | `TestRefreshTouchesNoKnowledgeFiles` (kaynak taraması: yazma çağrısı yok) + bellek-içi invariant girdiler (FS bağımlılığı yok). |

Ara kararlar: FILE fan-out sessizce bağlar (kapsam tüm dosyayı ister);
SYMBOL ıraksaması her zaman bloklar; orphan satırı yalnız ölen lineage
üzerine yazılır, hiç-bağlanmamış orphan bulgu-düzeyindedir (D-111).

## Reader / Breaker bulguları ve giderim — TASK-03 kapısı

(TASK-03 kapısı aşağıda.)

### TASK-03 guard mutasyon defteri (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M1 | container-ıraksama kontrolü kaldırıldı (ilk eşleşme kazanır) | İlk deneme SURVIVED — final-seviye çift test container seviyesini pinlemiyordu. `TestResolveContainerCollisionIsAmbiguous` (sınıf+fonksiyon ad çakışması) eklendi; mutant FAIL. |
| M2 | sticky kuralı kaldırıldı (her zaman orphan) | sticky testi FAIL. İlk M2 derlemeyi bozdu (geçersiz mutant), koşul-varyantıyla tekrarlandı. |
| M3 | FILE fan-out kaldırıldı (her ıraksama bloklar) | bound-rows testi FAIL (INV-0002 sessiz bound bekler). |
| M4 | pending erteleme kaldırıldı | deferral testi FAIL. |
| M5 | `blocks` her zaman true | orphan testi FAIL (LOW blokladı). İlk M5 kapanış ayracını yuttu (geçersiz mutant), düzeltildi. |
| M6 | prefix ayraç guard'ı kaldırıldı | prefix testi FAIL (`pkg2` satırı bağlandı). |
| M7 | bilinmeyen seviye sessizce bağlanır | malformed testi FAIL. |

Her mutant sonrası dosyalar backup'tan restore edilip md5 ile doğrulandı.

### TASK-03 kapı (PENDING — bağımsız değerlendirme bekleniyor)

`make check` yeşil (exit 0). Test sayısı `go test -list .\* ./... | grep -c ^Test` ile **1051**.

## TASK-03 kabul kanıtı (güncellendi — kapı remediasyonu dahil)

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-03.1 | Karşılandı | Scope-form tablosu + container-collision + prefix (separator guard) + PROJECT sessiz-bound (satırsız). |
| AC-03.2 | Karşılandı | Bound + idempotent re-refresh + sticky (D-110). |
| AC-03.3 | Karşılandı | Deferral: satır/bulgu/blok yok. |
| AC-03.4 | Karşılandı | Stored-row flip (pre-bind → kill → orphaned satır) + finding-only never-bound (D-111) + twins refresh (2 ambiguous satır, severity'e göre blok). |
| AC-03.5 | Karşılandı | Yazma-çağrısı taraması + import-graf assertion (yalnız `knowledge/record`) + bellek-içi girdiler. |

## Reader / Breaker bulguları ve giderim — TASK-03 kapısı

Bağımsız Reader **FAIL**, Breaker **BLOCKED** — 4 test + 1 ifade açığı:

- **HIGH (kalıcılık):** stored orphaned satırı pinleyen test yoktu. Giderim:
  orphan testi pre-bind + satır-flip assertion'ı kazandı.
- **HIGH (SYMBOL-ambiguous refresh):** `blockOnSeverity` refresh-seviyesinde
  çağrılmıyordu. Giderim: `TestAmbiguousTwinsBlockBySeverity` (CRITICAL bloklar,
  LOW bloklamaz, 2 satır) + `TestResolvedRefreshPrunesStaleBindings`.
- **MEDIUM (tarama):** `os.OpenFile` + import-graf assertion'ı eklendi.
- **MEDIUM (PROJECT):** sessiz-bound + satırsız assertion eklendi.
- **LOW (F5):** bulgu remedy'si divergence detail + candidates taşır.
- **LOW (F6):** D-110/D-111 gereksinimlere işlendi (yukarıda).

Breaker probları (a/b) commit'li testlere dönüştü; (c/d/e) yeşildi.
Reader F2-benzeri sayım itirazı yok; bulgu belgesi M1-benzeri notlarla tutarlı.

### TASK-03 guard mutasyon defteri — ek (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M8 | sticky ölü-lineage'ı da tutar | extended orphan testi FAIL |
| M9 | resolved yol prune'u atlar | prune testi FAIL |
| M10 | bulgu detail'ı düşer | twins remedy assertion FAIL |

### TASK-03 kapı — Reader PASS, Breaker VERIFIED (ikinci tur)

Remediasyon bağımsız ikinci turda notlandırıldı: 7 maddenin 7'si doğrulandı,
remediasyonun getirdiği yeni sorun yok (2 non-blocking gözlem kayıtta).
`make check` yeşil, 1053 test.

Reader **FAIL** ve Breaker **BLOCKED** kararlarının istediği 4 test + 1 ifade
düzeltmesi yukarıda uygulandı (M8–M10 kırmızı). Remediasyonun bağımsız
yeniden-notlandırılması aşağıda; tam `make check` de öyle. Test sayısı
`go test -list '.*' ./... | grep -c '^Test'` ile **1053**.

## TASK-04 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-04.1 | Karşılandı | `TestRenameMigratesUID`: uid taşınır, `previous_keys == [oldKey]`, identity sayısı 1, ambiguity 0. |
| AC-04.2 | Karşılandı | `TestMoveWithGitHintMigrates` (hint ile taşınır) + `TestMoveWithoutGitHintMintsAnew` (hintsiz mintler, ambiguity 0 — belgeli sınır). |
| AC-04.3 | Karşılandı | `TestClassRenameCascades`: 2 identity (mint yok), uid'ler taşınır, method `container_uid` == class uid. |
| AC-04.4 | Karşılandı | `TestTwinsRecordAmbiguity` (1 satır, removed + 2 heir, heir'lar uid-less, mint yok) + `TestRenameOntoExistingKeyKeepsBothLineages` (çalma yok: survivor sabit, removed rowsuz, ambiguity yok). |
| AC-04.5 | Karşılandı | `TestMintLastAfterFailedMigration` (1 fresh uid, 2 identity, ambiguity yok, boş lineage). |
| AC-04.6 | Karşılandı | `TestMalformedHintsRefused` + `git.DiffRenames` fake/failure/real-git testleri (`TestDiffRenames*`, D-101 best-effort). |
| AC-04.7 | Karşılandı | `TestConcurrentProcessesAgreeOnOneUID` (2 OS süreci, tek uid + 1 satır). |

### D-112 — eşleştirme transaction içinde Store metodudur (D-93 arıtması)

D-93 eşleştirme politikasını service'e yazdı; atomiklik (D-95) transaction
sınırını geçmeye izin vermez: saf politika (`matchBar`,
`orderParentsFirst`) ve akış (`resolveIdentitiesTx`, migrate/record/mint)
`internal/index` içindedir, `symbol.Service` transaction-dışı orkestrasyonu
sahiplenir (Git getirme — gelecek sürücüler, refresh, standalone tahsis).
Saf fonksiyonlar doğrudan birim-testlidir; D-93'ün sahiplenme niyeti
değişmedi, yerleşim transaction'a uydu.

### D-113 — ata kümesi dosya+ihattur, global tarama yok

Eşleştirme ataları iki kaynaktan gelir: bu dosyanın silinme-öncesi satırları
(aynı-dosya rename) ve `NewPath` bu dosya olan hint'lerin eski-yol satırları
(taşınma). Global "kaybolmuş anahtar" taraması ve fingerprint kolonları yok:
aynı birim + aynı dil dışındaki taşınmalar mintler (belgeli sınır), sıralama
bağımlılığı fail-safe yöndedir (erken indexlenen dosya heir'i bulur, geç
kalan orphan izler — asla yanlış bağlanmaz), stale satır budama MR-007
reconcile'undur.

## Reader / Breaker bulguları ve giderim — TASK-04 kapısı

(TASK-04 kapısı aşağıda.)

### TASK-04 guard mutasyon defteri (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M1 | bar'dan body kontrolü çıkarıldı | mint-last testi FAIL (body değişimi migrate olurdu) |
| M2 | aynı-dosya kapısı kaldırıldı (hep git gerekir) | rename + cascade testleri FAIL |
| M3 | pre-pass consumed işaretleme kaldırıldı | twins testi FAIL (bölünür: biri migrate, biri mint) |
| M4 | previous_keys append kaldırıldı | rename testi FAIL |
| M5 | cascade remap etkisiz (self-map/overwrite) | cascade testi FAIL. İlk iki M5 derlemeyi bozdu (geçersiz mutantlar), derlenen varyantla tekrarlandı. |
| M6 | ambiguity INSERT bozuldu | twins testi FAIL (satır yok) |
| M7 | hint validasyonu atlandı (`[:0]`) | malformed testi FAIL |

Her mutant sonrası dosyalar backup'tan restore edilip md5 ile doğrulandı.

### TASK-04 kapı (PENDING — bağımsız değerlendirme bekleniyor)

`make check` yeşil (EXIT=0, doğru ölçüldü). Test sayısı `go test -list .\* ./... | grep -c ^Test` ile **1067**.
