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
| AC-01.3 | Kısmi-karşılandı (şerhli) | İleri-şema fail-closed + knowledge-first yan-etkisizliği; *malformed-JSON* alt-davranışı karşılanmadı — loader'ın yerleşik non-fatal anlami kazanır (aşağıda). Test adı dürüstleşti (`...NewerSchemaFailsClosed`). |
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
| M9 | Diagnosis-hüküm kontrolü kaldırıldı (kapı bulgusu) | store-root testi FAIL |

Her mutant sonrası dosyalar backup'tan restore edilip md5 ile doğrulandı.

`make check` **yeşil** (exit 0, ikinci koşu — ilki yüzey pinini düşürdü,
D-220 müzakeresiyle güncellendi). Test sayısı **1251**.

`make check` **yeşil** (exit 0). Test sayısı **1254**.

### TASK-01 kapı — Reader: 3 CONFIRMED + 1 REFUTED-kısmi; Breaker: 4 REFUTED + 2 BULGU→KAPANDI

**Reader:** AC-01.1/01.2/01.4 CONFIRMED. AC-01.3 corrupt-yarısı DoD-1
şerhiyle kayıtlı (aşağıda).

**Eklenen-dosya şerhi (DoD-1 kaydı):** AC-01.3'ün "corrupt" alt-davranışı
karşılanmadı — malformed-JSON loader tasarımıyla non-fatal listeleniyor
(MR-001/002 yerleşikği, değiştirmek kapsam-dışı ve riskli). Fail-closed
yalnız fatal (ileri-şema) kayıtlara uygulanır. Task-list ebeveyni
("corruption") fatal-yolla karşılanıyor.

**Breaker B-3 (GERÇEK, KAPATILDI):** store-root dosya olunca empty-success
dönüyordu. Düzeltme: Diagnosis-hükmü startup-hatasında komutu reddediyor;
pin eklendi (M9 kırmızı).

**Breaker B-4 gözlemleri (kayıtlı, kapı-dışı):** rename-guard eski-taraf
görmüyor (addition okunuyor); stale-satırlar koşular-arası kalıyor (D-113
bilinen kısıt); symlink içerik-dışı hash'leniyor (zararsız).

**M7 düzeltmesi:** ölü "bad revision" eşleşmesi gerçekten silindi (önceki
kayıt yanlıştı).

**Karar:** TASK-01 KAPANDI (AC-01.3 şerhli).
