# MR-012 — Bulgular ve kapı kaydı

Bu kayıt, dondurulmuş [gereksinimlerin](mr-012-requirements.md) TASK-01
kapısını kaydeder. Reader/Breaker değerlendirmelerini ve bu turdaki kanıtları
korur; gereksinim veya tasarımın yerine geçmez.

## Başlangıç ve çalışma ağacı

- Başlangıç HEAD'i `b0a868e` idi (dondurulmuş gereksinimler + tasarım).
  §0 tablosundaki `make verify` ve `make tidy-check` MR-011 kapanışında
  yeşildi.

## TASK-01 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-01.1 | Karşılandı | Sayı-azalma + sıfır-kalan (removal/noop) özetli bulunuyor (`TestPythonAssertionDecrease`, `TestPythonAssertionRemoved`). |
| AC-01.2 | Karşılandı | Spec-listesi marker'lar + yakın-akrabalar negatif pinli (`TestPythonSkipMarkers`). |
| AC-01.3 | Karşılandı | Kaldırılan fonksiyon before-özetli; eklenen sessiz (`TestPythonRemovedAndAddedTests`). |
| AC-01.4 | Karşılandı | Kod kayıtlı + exit-class; bulgu AC-adlı alanları taşıyor (`TestGuardCodeRegistered`, decrease testi). |
| AC-01.5 | Karşılandı | String/comment içi assert sayılmıyor — node eşleşmesi (`TestPythonStringsAndCommentsDoNotCount`). |

**Tasarım notu (pointer API):** go-tree-sitter v0.25 düğümleri pointer
dönüyor; capture değerleri kopyalanıp adresleniyor (döngü-değişkeni
tuzağına dikkat, kodda yorumlu değil — basit kopya deseni).

### TASK-01 guard mutasyon defteri (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M1 | azalma `<` → `<=` | ilk küme yeşil kaldı → marker testi FAIL (eşit-sayı sessizliği) |
| M2 | test-ismi filtresi kapatıldı | python testleri FAIL |
| M3 | kod kayıt dışı bırakıldı | registry + exit-class testleri FAIL |
| M4 | decorator allowlist kaldırıldı | marker testi FAIL (akrabalar yakalandı) |
| M5 | unmapped-removal sinyali değiştirildi | removal testi FAIL |

Her mutant sonrası dosyalar backup'tan restore edilip md5 ile doğrulandı.

`make check` **yeşil** (exit 0, ikinci koşu — ilki `gofmt` düşürdü, düzeltildi). Test sayısı **1191**.

### TASK-01 kapı — Reader: 5/5 CONFIRMED + 3 gözlem; Breaker: 6/6 REFUTED

**Reader:** AC-01.1…AC-01.5 CONFIRMED. Gözlemler (kapatıldı): call-form
marker pinleri eklendi; "8 alan" dili düzeltildi.

**`test` prefix kararı (D-171 şerhi):** `isTestName` prefix-`test`
kullanıyor, D-171 `test_*` diyor. Runtime haklı çıkarıyor: pytest
varsayılan `test*` topluyor (`testimony` gerçekten koşar), guard'ın onu
koruması doğru — daraltma yanlış-negatif üretirdi. D-171 bu kayıtla
nitelendi.

**Breaker:** 6 sonda REFUTED (nested/last-wins/stacked/CRLF/partial-parse/
module-assert hepsi tutarlı).

**Karar:** TASK-01 KAPANDI.

## TASK-02 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-02.1 | Karşılandı | Expect-azalma/silme + skip varyantları + suite-skip + removal; cousin'lar sessiz (`TestEcma*`). |
| AC-02.2 | Karşılandı | CRITICAL removal/disable blokluyor (suite-disable dahil); skip/decrease/high/inactive uyarıyor; bilinmeyen eşleme reddediliyor (`TestMapped*`, `TestPolicyMatrixBlocking`, `TestUnknown*`). |
| AC-02.3 | Karşılandı | 4 trigger'da aynı bulgular, yalnız provenance farklı (`TestSharedServiceAcrossTriggers`). |
| AC-02.4 | Karşılandı | Marker'lı bastırma listeli; markersiz sessiz (`TestEscapeHatchSuppressesLoudly`). |
| AC-02.5 | Karşılandı | Politika matrisi testte; cousin-dışı bırakmalar belgeli (D-171 + bulgu). |

**Tasarım notları:** suite-skip `disabled` sayılıyor (spec "removed/disabled"
sertliği) — M6 ölü-kodu açığa çıkardı, politika yarısı canlandı; `it.only`/
`xdescribe` şekil-dışı (sıkı `splitMember` + excluded-kümesi); bastırma
ancak bastırılacak bulgu varsa listeleniyor.

**Süreç notu (backup zamanlaması):** M6/M9/M10 restore'ları TASK-02
ortasında alınmış backup'tan yapıldı; suite-disable satırı geri alındı,
tam-suite FAIL ile yakalandı, satır yeniden uygulandı, backup tazelendi.
Kural: oturum-içi davranış değişince backup tazelenir; restore sonrası
*hedef test değil tam-suite* koşulur.

### TASK-02 guard mutasyon defteri (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M6 | policy'den TEST_DISABLED çıkarıldı | ilk varyant ölü-koddu (yeşil) → suite-disable canlı bağlantısı sonrası FAIL |
| M7 | bilinmeyen-eşleme reddi kapatıldı | refusal testi FAIL |
| M8 | bastırma-listesi kapatıldı | hatch testi FAIL |
| M9 | şekil-kısıtı gevşetildi | cousin testi FAIL |
| M10 | excluded-kümesi kapatıldı | cousin testi FAIL |
| M11 | unescape kaldırıldı (kapı bulgusu) | escaped-mapping testi FAIL |
| M12 | aggregation kaldırıldı (kapı bulgusu) | duplicate testi FAIL |

Her mutant sonrası dosyalar backup'tan restore edilip md5 ile doğrulandı.

`make check` **yeşil** (exit 0, ikinci koşu — ilki `gofmt` düşürdü, düzeltildi). Test sayısı **1199**.

### TASK-02 kapı — Reader: 5/5 CONFIRMED + 4 artık; Breaker: 4 REFUTED + 2 BROKEN→KAPANDI

**Reader artıkları (kapatıldı):** mapped-xfail-warn, ECMA-unmapped-warn,
ECMA-hatch, ECMA-added-quiet pinleri eklendi.

**Breaker B-4 (kaçış-tırnak, KAPATILDI):** `unquote` escape açmıyordu,
gerçek-isimli eşleme reddediliyordu. Düzeltme: `strconv.Unquote` + tek-tırnak
fallback (`jsUnescape`); pin eklendi (M11 kırmızı).

**Breaker B-5 (duplicate-maskeleme, KAPATILDI):** aynı-isim ECMA
testlerinde last-wins sıralamaya göre bulgu gizliyordu (Python'da gölgeleme
doğru, JS'de hepsi koşar). Düzeltme: isim-başı aggregation (sum + union);
tek kopya silinmesi decrease okunuyor — doğru, test duruyor. Pin eklendi
(M12 kırmızı).

**Karar:** TASK-02 KAPANDI.

## TASK-03 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-03.1 | Karşılandı | `TestWeakeningScenarioEndToEnd`: §1 senaryosu tek değerlendirmede (3 bulgu, 1 blocking, 1 suppression) × 4 trigger. |
| AC-03.2 | Karşılandı | Grep kanıtı: text-scan yok; kod 44 (tek yeni); migration/komut yok (yalnız exit-class satırı); testguard'da SQL yok. |
| AC-03.3 | Karşılandı | M1…M12 defterde, tamamı kırmızı koşuldu. TASK-03 yeni guard eklemiyor — mutasyon borcu yok. |

`make verify` **yeşil** (exit 0: check + race + smoke), `make tidy-check`
**yeşil**. Test sayısı **1205**.

### TASK-03 kapı (commit sonrası bağımsız değerlendirme bekleniyor)
