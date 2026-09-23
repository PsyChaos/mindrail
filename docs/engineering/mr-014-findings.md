# MR-014 — Bulgular ve kapı kaydı

Bu kayıt, dondurulmuş [gereksinimlerin](mr-014-requirements.md) TASK-01
kapısını kaydeder. Reader/Breaker değerlendirmelerini ve bu turdaki kanıtları
korur; gereksinim veya tasarımın yerine geçmez.

## Başlangıç ve çalışma ağacı

- Başlangıç HEAD'i `1ad4932` idi (dondurulmuş gereksinimler + tasarım).
  §0 tablosundaki `make verify` ve `make tidy-check` MR-013 kapanışında
  yeşildi.

## TASK-01 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-01.1 | Karşılandı | SDK go.mod'da; 4 okuma-aracı kayıtlı + liste assertion'lı (`TestServerRegistersFourReadTools`); handler'lar tipli In/Out. |
| AC-01.2 | Karşılandı | Bootstrap/status aynı `bootstrap.App` + `status.Build` yolundan; readiness eşitliği pinli (`TestBootstrapAndStatusShareServices`). |
| AC-01.3 | Karşılandı | Kayıt + sembol kaynakları, 50-cap refuse'lu (`TestSearchBindsRecords`, sembol ucu dahil). |
| AC-01.4 | Karşılandı | Summary/focused şekilleri (`TestContextLevels`). |
| AC-01.5 | Karşılandı | Sunulan-ertelenmiş her parametre tek-tek refuse'lu; yokluk reddetmiyor (aynı test, izole vakalar). |

**Tasarım notları:** SDK şema-doğrulaması katı (nil slice/map reddediliyor)
→ refusal'lar boş kap taşıyor; `Results` hep-var (omitempty yok);
present-vs-absent ayrımı pointer alanlarla; DB yolu fixture'da gerçek
(`.git/mindrail/mindrail.db`).

### TASK-01 guard mutasyon defteri (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M1 | limit-kapağı kaldırıldı | search testi FAIL |
| M2 | full summary'e bağlandı | context testi FAIL |
| M3 | target_type reddi kapatıldı | ilk varyant maskeli (target_id yedeği) → izole vakalar, FAIL |
| M4 | search kaydı silindi | liste testi FAIL (pin eklendi: ListTools assertion) |
| M5 | boş-sorgu reddi kapatıldı | ilk varyant pinsizdi (yeşil) → raw-call testi eklendi, FAIL |

Her mutant sonrası dosyalar backup'tan restore edilip md5 ile doğrulandı.

`make check` **yeşil** (exit 0, ikinci koşu — ilki 3 yeni-kod artçısı
düşürdü: kod sayacı 45→46, exit-class satırı, MCP-yasağı kaldırma;
tamamı düzeltildi). Test sayısı **1219**.

### TASK-01 kapı (commit sonrası bağımsız değerlendirme bekleniyor)
