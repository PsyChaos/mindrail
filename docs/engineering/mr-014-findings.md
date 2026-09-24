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

### TASK-01 kapı — Reader: 3 CONFIRMED + 2 REFUTED(harf); Breaker: 6 REFUTED + 2 not

**Reader bulguları (kapatıldı):** AC-01.1 "altı tool" dondurulmuş harfe
aykırı 4 kayıttı → decide/invariant refuse-stub olarak kaydedildi (ad +
şema sabit, davranış TASK-02'de); AC-01.2 eşdeğerlik prose Seviyesindeydi →
`status.Build` bayt-eşitlik pini eklendi (duration_ms hariç).

**Breaker:** 6/6 REFUTED (limit-sıfır refuse, 65-hit cap doktrini, boş
detail refuse, concurrent roots, kötü-root, bozuk-knowledge hepsi tutarlı).
Notlar (kapatıldı): limit mesajı düzeltildi; cap-truncation totalsızlığı
0.1 kısıtı olarak kayda geçti.

**Karar:** TASK-01 KAPANDI.

## TASK-02 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-02.1 | Karşılandı | Validate + persist + loader-readback (filename==id) + max+1 (`TestDecideRoundTrips`). |
| AC-02.2 | Karşılandı | Active round-trip + candidate/empty/missing refuse'ları (`TestInvariantActiveRoundTrips`). |
| AC-02.3 | Karşılandı | Full deferred matrisi + over-limit, hepsi kodlu + next_action'lı (read + write testleri). |
| AC-02.4 | Karşılandı | İki farklı kimlik, aynı şema + aynı sonuçlar, 6 tool (`TestTwoClientsAgree`). |
| AC-02.5 | Karşılandı | Hata/red cevaplarında değer yankısı yok (`TestDiagnosticsEchoNamesOnly`). |

**Tasarım notları:** severity/scope ön-kontrol kaldırıldı (record
validasyonu tek kaynak); O_EXCL yarış-zırhı deterministik pinlenemiyor
(tahsis hep max+1 seçer — best-effort, kayıtlı); stub assertion'ları
TASK-02 ile emekli.

### TASK-02 guard mutasyon defteri (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M6 | tahsis max+1 → max | round-trip testi FAIL (çakışma) |
| M7 | mode-kontrolü kapatıldı | invariant testi FAIL |
| M8 | uzantısız dosya adı | round-trip testi FAIL (loader okumuyor) |
| M9 | next_action boşaltıldı | ilk pin yeşil kaldı → len assertion'ı eklendi, FAIL |
| M10 | id-dolgu kaldırıldı | round-trip testi FAIL (ID deseni) |

Her mutant sonrası dosyalar backup'tan restore edilip md5 ile doğrulandı.

`make check` **yeşil** (exit 0). Test sayısı **1223**.

### TASK-02 kapı — Reader: 5/5 CONFIRMED + notlar; Breaker: 5 REFUTED + 1 gözlem-kayıtlı

**Reader:** AC-02.1…AC-02.5 CONFIRMED (mutant koşuları reader-remit dışı —
defterdeki kayıtlar geçerli).

**Breaker:** 5/6 REFUTED (traversal, yarış-bütünlüğü, bogus-red, readonly
hepsi temiz). (4) sınırsız metin: tool doğrudan-dosya-yazma yetisini
yansıtıyor — yeni tehdit yüzeyi yok, ajan zaten dosyayı yazabilir; kayıtlı
kısıt, takip-işi, kapı-dışı.

**Karar:** TASK-02 KAPANDI.

## TASK-03 kabul kanıtı (PENDING)

## TASK-03 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-03.1 | Karşılandı (kapıda) | `make verify` + `make tidy-check` + sayı aşağıda. |
| AC-03.2 | Karşılandı | Grep kanıtı: tek yeni kod (46); 6 tool kaydı; CLI komutu yok; mcp'de tablo yok; diagnostiklerde değer yankısı yok (alanlar yalnızca kayda akar). `mcpCompatibility="none"` bilinçli korunuyor: serving yok (D-191), aggregate cevap hâlâ dürüst. |
| AC-03.3 | Karşılandı (kapıda) | M1…M10 defterde, tamamı kırmızı koşuldu. Durum bloğu kapıda. |

`make verify` **yeşil** (exit 0: check + race + smoke), `make tidy-check`
**yeşil**. Test sayısı **1223**.

### TASK-03 kapı (commit sonrası bağımsız değerlendirme bekleniyor)
