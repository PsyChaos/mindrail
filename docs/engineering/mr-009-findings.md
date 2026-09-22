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
