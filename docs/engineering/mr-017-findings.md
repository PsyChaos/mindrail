# MR-017 — Bulgular ve kapı kaydı

Bu kayıt, dondurulmuş [gereksinimlerin](mr-017-requirements.md) TASK-01
kapısını kaydeder. Reader/Breaker değerlendirmelerini ve bu turdaki kanıtları
korur; gereksinim veya tasarımın yerine geçmez.

## Başlangıç ve çalışma ağacı

- Başlangıç HEAD'i `8551a1b` idi (dondurulmuş gereksinimler + tasarım).
  §0 tablosundaki `make verify` ve `make tidy-check` MR-016 kapanışında
  yeşildi.

## TASK-01 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-01.1 | Karşılandı | Staged ambiguity/unregistered DENY'leri paylaşılan değerlendirmeden (`TestVerifyStagedAmbiguityDenies`, CLI unregistered). |
| AC-01.2 | Karşılandı | Staged guard blocking, HEAD/index deltasıyla (`TestVerifyStagedGuardBlocks`, iki paslı seed). |
| AC-01.3 | Karşılandı | İleri-şema fail-closed + knowledge-first yan-etkisizliği (`TestKnowledgeValidateCorruptFailsClosed`, `TestVerifyStagedKnowledgeFirst`). |
| AC-01.4 | Karşılandı | Temiz yeşil + kodlu remedy'li DENY + envelope exit'leri (`TestVerifyStagedCleanGreen`, unregistered). |

**Tasarım notları:** `-z` sekmesizliği gerçek git'e soruldu; op-yakınsama
(NULL-change mint yerine replay); kırık-desk index-katmanı kısıtı (satırlar
commit'i, index masayı izler — kayıtlı); D-216 diferansiyel pinli.

**Süreç notu (dosya karışması):** mutant-backup isim çakışması
`internal/cli/verify.go` + `internal/git/staged.go`'yu çapraz-bulaştırdı;
tam-suite + paket-beyanı denetimiyle yakalandı, dosyalar içerik-bilgisinden
baştan yazıldı, yeşil öncesi paket listesi doğrulandı. Kural: backup'lar
benzersiz isimli olur; restore sonrası paket-beyanı + tam-suite koşulur.

### TASK-01 guard mutasyon defteri (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M1 | mod-kontrolü kapatıldı | bare testi FAIL |
| M2 | fatal-dalı kapatıldı | corrupt + knowledge-first testleri FAIL |
| M3 | knowledge fatal-hata dalı kapatıldı | corrupt testi FAIL |
| M4 | FatalKnown erken-dönüşü kapatıldı | yan-etki pini FAIL |
| M5 | staged-sync diskten okundu | diferansiyel test FAIL |
| M6 | staged-drift düşürüldü | drift testi FAIL |
| M7 | unborn-HEAD eşleşmesi kaldırıldı | unborn testi FAIL (ölü "bad revision" eşleşmesi de temizlendi) |
| M8 | verify Args-kısıtı kaldırıldı | extra-args testi FAIL |

Her mutant sonrası dosyalar backup'tan restore edilip md5 ile doğrulandı.

`make check` **yeşil** (exit 0, ikinci koşu — ilki yüzey pinini düşürdü,
D-220 müzakeresiyle güncellendi). Test sayısı **1251**.

`make check` **yeşil** (exit 0). Test sayısı **1254**.

### TASK-01 kapı (commit sonrası bağımsız değerlendirme bekleniyor)
