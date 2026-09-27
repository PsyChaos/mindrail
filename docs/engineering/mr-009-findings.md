# MR-009 — Bulgular ve kapı kaydı

Bu kayıt, dondurulmuş [gereksinimlerin](mr-009-requirements.md) TASK-01
kapısını kaydeder. Reader/Breaker değerlendirmelerini ve bu turdaki kanıtları
korur; gereksinim veya tasarımın yerine geçmez.

## Başlangıç ve çalışma ağacı

- Başlangıç HEAD'i `1694d59` idi (dondurulmuş gereksinimler + tasarım).
  §0 tablosundaki `make verify` ve `make tidy-check` MR-008 kapanışında
  yeşildi.

## TASK-01 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-01.1 | Karşılandı | Her entry edge + confidence + depth + fallback-reason taşıyor (`TestAnalyzeFollowsResolvedEdges`). |
| AC-01.2 | Karşılandı | Ters traversal çözümlü edge'leri izliyor, her entry bindiği edge'i adlandırıyor (aynı test). |
| AC-01.3 | Karşılandı | Sıfır-değer istek depth 1; explicit 2 referrer-of-referrer'a ulaşıyor, her entry kendi depth'ini taşıyor (`TestAnalyzeDepthDefaultsToOne`). |
| AC-01.4 | Karşılandı | Yapısal kanıt TARGETED; explicit justification PACKAGE'i verbatim kaydediyor, structural kıpırdamıyor (`TestAnalyzeBreadthCapAndOverride`). |

**Tasarım notu (confidence):** satırdaki confidence okunuyor, uydurulmuyor —
indexer bugün çözümlü/çözümsüz her satıra 0.5 yazıyor (`indexer.go:284`),
bu yüzden direct ile name-match sayıyı paylaşıyor, tür ve etiketle
ayrışıyor. Gereksinim dondurulmadan önce tasarıma işlendi.

### TASK-01 guard mutasyon defteri (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M1 | depth-default `<= 0` → `< 0` | depth testi FAIL |
| M2 | dedupe-anahtarı `(referrer, target)` → `(referrer, path)` | ilk varyant ayırt etmedi → paylaşılan-referrer testi eklendi (`TestAnalyzeKeepsSharedReferrersDistinct`), mutant FAIL |
| M3 | breadth-override koşulsuz | breadth testi FAIL |
| M5 | justification-kontrolü kapatıldı (kapı bulgusu) | yeni unjustified-override pini FAIL |
| M4 | resolved-join'e `IS NULL` eklendi | direct testi FAIL |

M2 dürüstlük notu: ilk mutant kırmızı vermedi (fixture'da ayrım yoktu) —
test eklendikten sonra koşuldu, kırmızı doğrulandı.

`make check` **yeşil** (exit 0, ikinci koşu — ilki `gofmt` düşürdü, düzeltildi). Test sayısı **1144**.

### TASK-01 kapı — Reader: 4/4 CONFIRMED + 1 gap; Breaker: 5/6 REFUTED + 1 BROKEN→KAPANDI

**Reader:** AC-01.1…AC-01.4 CONFIRMED. Gap (kapı-dışı değil): override
justification gerektirmiyordu (tasarım §4'e aykırı) — Breaker B-1 ile aynı
delik.

**Breaker B-1 (KAPATILDI):** justification'sız `BreadthOverride` yükseltiyordu.
Düzeltme: override yalnız `Justification != ""` iken uygulanıyor, yoksa
yapısal cap duruyor (`impact.go`); pin eklendi (M5 ile kırmızı doğrulandı).

**Karar:** TASK-01 KAPANDI.

## TASK-02 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-02.1 | Karşılandı | Çözümsüz cross-file kullanıcı name-match ≤0.5 + gerekçe, direct'e yükselmiyor (`TestAnalyzeNameMatchFallback`). |
| AC-02.2 | Karşılandı | Değişen dosya katmanı gerekçesiyle raporlanıyor, MODULE; sembol edge'i uydurulmuyor (`TestAnalyzeFileFallbackReportsFloor`). |
| AC-02.3 | Karşılandı | İki aynı-isim tek ambiguous entry'de tüm adaylarla, seçim yok (`TestAnalyzeSameNameAmbiguityNeverCollapses`). |
| AC-02.4 | Karşılandı | Bound invariant'lar entry yanında + scope metni; bound-dışı statüler yok (`TestAnalyzeListsBoundInvariants`). |

**Tasarım notları:** tırmanış yalnız direct edge'leri izler (zayıf sinyal
yükseltilmez); `DirectOnly` AC-01.4'ün direct-only modunu korur (dondurulmuş
metinde yoktu — 3 satırlık gap-fill, kapıda); name-match/ambiguous
confidence 0.5 indexer sabitinin kelimesi, uydurma değil.

### TASK-02 guard mutasyon defteri (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M6 | name-match confidence 0.5 → 0.9 | fallback + ambiguity testleri FAIL |
| M7 | ambiguous eşiği `default` → `case 1, 2` (çökertme) | ambiguity testi FAIL |
| M8 | binding statü filtresi kaldırıldı | invariant testi FAIL (orphaned sızdı) |
| M9 | `DirectOnly` yok sayıldı | direct-only + breadth testleri FAIL |
| M10 | `SymbolsNamed` NULL-uid filtresi kaldırıldı | ambiguity testi FAIL |
| M11 | seen-anahtarı path'siz (kapı bulgusu) | cross-unit regresyon testi FAIL |
| M12 | fallback'ta override-koruma kaldırıldı (kapı bulgusu) | override-survives testi FAIL |

Her mutant sonrası dosyalar backup'tan restore edilip md5 ile doğrulandı.

`make check` **yeşil** (exit 0). Test sayısı **1150**.

### TASK-02 kapı — Reader: 4/4 CONFIRMED + 2 gözlem; Breaker: 6/6 REFUTED + 1 BROKEN→KAPANDI + 1 tasarım-ihlali→KAPANDI

**Reader:** AC-02.1…AC-02.4 CONFIRMED. Gözlemler (kapı-dışı): name-match'te
path-filtresi yok (tasarıma uygun); file confidence 1 pinsizdi → pin eklendi.

**Breaker B-2 (KAPATILDI):** `seen` anahtarı unit/path'sizdi — aynı anahtar
iki unit'te çakışınca aday sessizce düşüyordu. Düzeltme: 4'lü anahtar
(unit, path, key, target); regresyon testi eklendi (M11 ile kırmızı
doğrulandı).

**Reader/Breaker ortak gözlemi (KAPATILDI):** fallback engaged olunca
justified override MODULE'a geri yazılıyordu (tasarım §6'ya aykırı).
Düzeltme: override korunuyor, structural MODULE raporlanıyor; test eklendi
(M12 ile kırmızı doğrulandı).

**Karar:** TASK-02 KAPANDI.

## TASK-03 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-03.1 | Karşılandı | `TestAnalyzeRealRepositoryEndToEnd`: gerçek indexlenmiş repo — direct + name-match + file + invariant tek analizde, hepsi açıklamalı, MODULE, depth 1. |
| AC-03.2 | Karşılandı | Grep kanıtı: migration yok (7 dosya), impact'te `CREATE TABLE` yok, kod 43, knowledge v1, CLI/app diff'i boş. |
| AC-03.3 | Karşılandı | M1…M13 defterde, tamamı kırmızı koşuldu (aşağıda). Durum bloğu kapıda. |

**E2E bulgusu (AC-02.1 boşluğu, TASK-03'te kapatıldı):** E2E ilk koşuda
`worker → helper` çağrısını bulamadı — name-match yalnızca aynı-isim
bildirimleri arıyordu, çözümsüz satırdaki *çağıranları* değil. Oysa AC-02.1
"name users" diyor. Düzeltme: `UnresolvedReferringTo` (nötr okuma) +
çağıran katmanı (satır confidence'ı + D-90 gerekçesi, self-hariç,
seen-dedupe'li); bildirim katmanının gerekçe dili düzeltildi
("declaration sharing name"). Pin: `TestAnalyzeUnresolvedCallersArriveWeak`
+ E2E; mutant M13 kırmızı.

### TASK-03 guard mutasyon defteri

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M13 | çözümsüz-çağıran katmanı kapatıldı | caller testi + E2E FAIL |

`make verify` **yeşil** (exit 0: check + race + smoke), `make tidy-check`
**yeşil**. Test sayısı **1154**.

### TASK-03 kapı — Reader: AC-03.1/03.2 CONFIRMED, AC-03.3 ledger-CONFIRMED/Durum-NOT-YET; Breaker: 5/5 REFUTED

**Reader:** E2E + non-goal + M13 (yeniden kırmızı doğrulandı) CONFIRMED;
Durum bloğu süreç gereği kapı-sonrası — failure değil. AC-02.1 gap-fix'i
meşru, minimal, dürüst kayıtlı; TASK-02'nin AC-02.1 onayı bu kayıtla
nitelendi.

**Breaker:** E2E caller-katmanına gerçekten bağımlı (mutantla FAIL
doğrulandı, restore edildi); caller+declaration çakışması dedupe'li;
self-skip; 5/5 stabil; depth muhasebesi temiz.

**Karar:** TASK-03 KAPANDI.

---

## MR-009 kapanış

Üç görev kapandı (TASK-01…03), 13 mutant kırmızı, `make verify` +
`make tidy-check` yeşil, 1154 test. MR-010'a devir (Durum bloğundaki gibi):
entry + breadth → runner; invariant listeleri → invalidation;
justification slotu → politika.
