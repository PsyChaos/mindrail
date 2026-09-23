# MR-013 — Bulgular ve kapı kaydı

Bu kayıt, dondurulmuş [gereksinimlerin](mr-013-requirements.md) TASK-01
kapısını kaydeder. Reader/Breaker değerlendirmelerini ve bu turdaki kanıtları
korur; gereksinim veya tasarımın yerine geçmez.

## Başlangıç ve çalışma ağacı

- Başlangıç HEAD'i `1ff3ee5` idi (dondurulmuş gereksinimler + tasarım).
  §0 tablosundaki `make verify` ve `make tidy-check` MR-012 kapanışında
  yeşildi.

## TASK-01 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-01.1 | Karşılandı | Aile-başı denial: kod + provenance + next_action (`TestGateDeniesPerFamily`, 5 kod sıralı). |
| AC-01.2 | Karşılandı | Temiz ALLOW + uyarılar-asla (`TestGateAllowsClean`). |
| AC-01.3 | Karşılandı | Çift-değerlendirme + sıra-bağımsızlık DeepEqual (`TestGateDecisionIdempotent`). |
| AC-01.4 | Karşılandı | Kod kayıtlı + exit-class; sayaç 44→45 (`TestEvidenceCodeRegistered`, nongoals). |
| AC-01.5 | Karşılandı | Severity kapısı: non-CRITICAL sessiz (`TestGateSeverityGatePinsWarnScope`). |

### TASK-01 guard mutasyon defteri (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M1 | Allow tersine çevrildi | family + allows testleri FAIL |
| M2 | severity kapısı kapatıldı | severity testi FAIL |
| M3 | kod kayıt dışı bırakıldı | registry + exit-class testleri FAIL |
| M4 | sıralama kaldırıldı | idempotency sıra-testi FAIL |
| M5 | attribution-blocking filtresi kapatıldı | allows testi FAIL |
| M6 | boş-bulgu reddi kapatıldı (kapı bulgusu) | refusal testi FAIL |
| M7 | dedupe kaldırıldı (kapı bulgusu) | dedupe testi FAIL |
| M8 | ambiguous-dalı kaldırıldı (kapı bulgusu) | ambiguous testi FAIL |

Her mutant sonrası dosyalar backup'tan restore edilip md5 ile doğrulandı.

`make check` **yeşil** (exit 0). Test sayısı **1210**.

### TASK-01 kapı — Reader: 5/5 CONFIRMED + gaps; Breaker: 3 REFUTED + 3 BROKEN→KAPANDI

**Reader gaps (kapatıldı):** binding-ambiguous yolu pinsizdi → test eklendi;
`"ambiguous"` literali → `index.BindingAmbiguous` sabiti; M-ledger
doğrulanamaz kaydı not edildi.

**Breaker B-1/B-2 (boş-bulgular, KAPATILDI):** boş blocking finding'ler
belirsiz denial üretiyordu. Düzeltme: `Evaluate` artık `(Decision, error)`
dönüyor — boş blocking bulgu ve bilinmeyen binding statüsü yüksek sesle
reddediliyor (fail-closed); pin eklendi (M6 kırmızı).

**Breaker B-3 (dup-denial, KAPATILDI):** aynı denial iki kez listeleniyordu.
Düzeltme: birebir-aynı denial'lar tekleniyor; pin eklendi (M7 kırmızı).

**Karar:** TASK-01 KAPANDI.
