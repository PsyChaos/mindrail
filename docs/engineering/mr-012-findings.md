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
| AC-01.4 | Karşılandı | Kod kayıtlı + exit-class; bulgu 8 alanlı (`TestGuardCodeRegistered`, decrease testi). |
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

### TASK-01 kapı (commit sonrası bağımsız değerlendirme bekleniyor)
