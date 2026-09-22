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

### TASK-01 kapı — Reader: TEMİZ (5/5 AC CONFIRMED); Breaker: 1 MİNÖR + 1 GAP

**Reader** (tüm AC CONFIRMED, kanıt dosya:satır ile): AC-01.1 migration tek tablo;
AC-01.2 sürüm 7 + v6→v7 upgrade satırları koruyarak; AC-01.3 üç kod + ayrık
remedy + ExitFailed; AC-01.4 pin + ledger/shape/load; AC-01.5 yalnız
`schema_version` 6→7. Gözlem (kapı-dışı): remedy ayrıklık testi path/key
interpolasyonu nedeniyle zayıf pin — harf karşılıyor.

**Breaker B-1 (MİNÖR, CONFIRMED — TASK-03'e ertelendi):** `decided_by = ''`
ham SQL ile kabul ediliyor; D-137 "empty deciders refused" diyor. Gerekçe:
TASK-01'de Go yazım yolu yok (tek yol ham SQL); ret AC-03.2'nin
`RecordAttribution` validasyonuna ait. TASK-03 `RecordAttribution` boş
decider'ı yazmadan reddedecek + test pinleyecek. DB CHECK için 000007
shipped/pinned olduğundan yeni migration gerekirdi — yazım-yolu validasyonu
yeterli, CHECK eklenmiyor.

**Breaker GAP-1 (KAPATILDI):** `TestChangeRowsEnforceForeignKeys`
`scope_attributions` bilinmeyen-change durumunu pinlemiyordu. Kapatıldı:
bilinmeyen change'e attribution FK reddi eklendi, PASS.

**Karar:** TASK-01 KAPANDI — AC-01.1…AC-01.5 karşılandı, B-1 TASK-03'e
ertelendi (AC-03.2), GAP-1 kapatıldı.

## TASK-02 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-02.1 | Karşılandı | Tek aday attributes, bulgu yok, çift koşu stabil (`TestAttributeTaskSingleCandidateAttributes`). |
| AC-02.2 | Karşılandı | Sıfır aday unregistered: kod + provenance + next_action + blocking (`TestAttributeTaskZeroCandidatesIsUnregistered`). |
| AC-02.3 | Karşılandı | İki aday ambiguous, iki change de adlandırıldı, seçim yok (`TestAttributeTaskTwoCandidatesIsAmbiguous`). |
| AC-02.4 | Karşılandı | Dosya-dışı drift + blocking; kapsam-içi sessiz (`TestAttributeTaskDriftFiresPerFile`). |
| AC-02.5 | Karşılandı | Her bulguda provenance + next_action; çözülemeyen uid boş dosya ile unregistered, yol uydurulmuyor (`TestAttributeTaskNeverInventsPaths`). |

**Tasarım notu (D-145 yorumu):** sembol→dosya eşleşmesi `symbol_uid`
üzerinden canlı index'ten çözülüyor (`index.Store.PathForUID`). Hüküm kümesi
hâlâ yalnızca discovery satırlarınca sürülüyor — index yalnızca dosya adını
veriyor; keşfedilmemiş iş her hükmün dışında. Çözülemeyen uid unregistered
oluyor, hata değil.

### TASK-02 guard mutasyon defteri (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M6 | aday eşitliği bozuldu (`scope == file+"?"`) | attributed + ambiguous testleri FAIL |
| M7 | drift koşulu tersine çevrildi | drift + single testleri FAIL |
| M8 | `PathForUID` hep boş döndü | attributed + PathForUID testleri FAIL |
| M9 | `ListTaskChanges` NULL-task listeledi | ambiguous testi FAIL |
| M10 | change-yokluğu corruption sayıldı | without-change testi FAIL |

Her mutant sonrası dosyalar backup'tan restore edilip md5 ile doğrulandı.

`make check` **yeşil** (exit 0). Test sayısı **1131**.

### TASK-02 kapı — Reader: TEMİZ (5/5 CONFIRMED); Breaker: 6/6 REFUTED

**Reader:** AC-02.1…AC-02.5 CONFIRMED; bulgu kaydı gerçeklikle eşleşiyor;
D-145 yorum notu dürüst. Gözlem (kapı-dışı): tasarım "symbol paketi dahil
değil" derken implementasyon uid→dosya için `internal/index`'e dayanıyor —
canlı filesystem değil, index DB tablosu; kabul edilen tasarım sapması
olarak kaydedildi.

**Breaker:** 6 sonda da REFUTED (yabancı-task ambiguity, removed satırlar,
çift-uid tiebreak, rename old_path, prefix sızıntısı, NULL-task adaylığı).
Gözlem (kapı-dışı): çift-uid tiebreak sessiz lowest-id-wins — pratikte
imkânsız (twins NULL uid alır), bulgu yok.

**Karar:** TASK-02 KAPANDI.

## TASK-03 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-03.1 | Karşılandı | Override ambiguous + unregistered sembolü çözüyor (`TestOverrideClearsOneSymbolWhileSiblingsBlock`, `TestOverrideBindsUnregisteredSymbols`). |
| AC-03.2 | Karşılandı | Boş decider/bilinmeyen change/bozuk anahtar yazmadan reddediliyor; satır kalmıyor (`TestRecordAttributionRefusesBeforeAnyWrite`). Breaker B-1 kapandı. |
| AC-03.3 | Karşılandı | Üç kod birlikte blocking set'te; temiz task boş dönüyor (`TestEvaluateTaskReturnsTheBlockingSet`). |
| AC-03.4 | Karşılandı | Tek lease adlandırılıyor, sıfır/çoklu adlandırmıyor; lease sonucu değiştirmiyor — iki yön de pinli (`TestLeaseGuidanceNamesExactlyOneHolder`). |
| AC-03.5 | Karşılandı | NULL-task unregistered bandında, isimsiz-change'siz task boş — ikisi de hatasız (`TestOwnerlessWorkEvaluatesUnregistered`). |

**D-137 sonucu (açıklandı):** override anahtar-bazında global — `a::f`
için kaydedilen atama, iki task'ın değerlendirmesinde de bulguyu kaldırır.
Test önce kardeş-task beklentisini yanlış yazdı (aynı anahtar için bulgu
bekledi); D-137 gereği düzeltildi — override'sız ikinci sembol (`a::h`)
kardeşte bloklu kalıyor, seri-çözüm davranışı onunla pinli.

### TASK-03 guard mutasyon defteri (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M11 | decider-kontrolü kapatıldı | refusal testi FAIL |
| M12 | override-danışma kapatıldı | iki override testi FAIL |
| M13 | lease eşiği `!= 1` → `< 1` | lease testi FAIL |
| M14 | ownerless sorgusu `IS NULL` → `IS NOT NULL` | ownerless testi FAIL |
| M15 | task-lease eşleşmesi hep-doğru | lease testi FAIL |

Her mutant sonrası dosyalar backup'tan restore edilip md5 ile doğrulandı.

`make check` **yeşil** (exit 0). Test sayısı **1137**.

### TASK-03 kapı (commit sonrası bağımsız değerlendirme bekleniyor)
