# MR-018 — Bulgular ve kapı kaydı

Bu kayıt, dondurulmuş [gereksinimlerin](mr-018-requirements.md) TASK-01
kapısını kaydeder. Reader/Breaker değerlendirmelerini ve bu turdaki kanıtları
korur; gereksinim veya tasarımın yerine geçmez.

## Başlangıç ve çalışma ağacı

- Başlangıç HEAD'i `a2ae21c` idi (dondurulmuş gereksinimler + tasarım).
  §0 tablosundaki `make verify` ve `make tidy-check` MR-017 kapanışında
  yeşildi.

## TASK-01 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-01.1 | Karşılandı | Temiz aralık yeşili (`TestVerifyCICleanGreen`), unregistered DENY (`TestVerifyCIUnregisteredDenies`), guard-denial (`TestVerifyCIGuardDenies`), hepsi paylaşılan kapıdan. |
| AC-01.2 | Karşılandı | Bozuk base (`TestVerifyCIBadBaseRefuses`, COMMAND_LINE_INVALID + 0 satır), unborn HEAD (`TestVerifyCIUnbornHeadRefuses`), default-zincir olumlu/olumsuz (`TestVerifyCIDefaultBase`, `...NamesCandidates`). |
| AC-01.3 | Karşılandı | Çift mod ve rev-flags kapısı (`TestVerifyModesAreExclusive`); yalın `verify` reddi MR-017'den devralındı (`TestVerifyBareRefusesMode`). |
| AC-01.4 | Karşılandı | Fatal knowledge + bozuk base yan yana: knowledge hatası kazanır, 0 satır (`TestVerifyCIKnowledgeFirst`). |
| AC-01.5 | Karşılandı | Aynı içerik staged ve CI'da birebir aynı code+key kümesini deny ediyor (`TestVerifyCIParityWithStaged`). |

**Tasarım notları:** `compose`/`guardMappings` çıkarımları staged
davranışını satır-satır korur (`make check` yeşil, staged testleri
dokunulmadan geçiyor). Guard trigger'ı CI yolunda `TriggerCI` — staged
bulgusu "staged", CI bulgusu "ci" der; parite karşılaştırması bu yüzden
code+key üzerinden yapılır (D-225 okuması, aşağıda). `M5` adayı
davranış-koruyucu çıktı (isim dönmek de çalışıyor — git downstream'da
kendisi çözümlüyor); pin `M5'` ile yazıldı (zincir hatası → olumlu test
kırmızı). `M1`/`M6` ilk yazımda kırmızı vermedi (test kurgusu reddi başka
yoldan üretiyordu); testler güçlendirildi, sonra kırmızı koşuldu.

**D-225 okuması (uygulama notu):** paritedeki "aynı provenance" guard
trigger kelimesini kapsamaz — trigger yolu adlandırır, bulgunun
kaynağını değil. CI'da "staged" yazmak provenance yalanı olurdu.

### TASK-01 guard mutasyon defteri (tamamı geri alındı, md5-doğrulamalı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M1 | mod-dışlayıcılık kapatıldı (`staged && ci`) | `TestVerifyModesAreExclusive` FAIL |
| M2 | knowledge-fatal erken-dönüşü kapatıldı | `TestVerifyCIKnowledgeFirst` FAIL |
| M3 | guardRange before=head (zayıflama görünmez) | `TestVerifyCIGuardDenies` FAIL |
| M4 | RangeEntries head..head (boş aralık) | `TestVerifyCIUnregisteredDenies` FAIL (yeşile döndü) |
| M5' | DefaultBase hep-hata | `TestVerifyCIDefaultBase` FAIL |
| M6 | rev-flags kapısı kapatıldı | `TestVerifyModesAreExclusive` FAIL |
| M7 | yalın-verify reddi kapatıldı | `TestVerifyBareRefusesMode` FAIL |

Her mutant sonrası dosyalar benzersiz-isimli backup'tan restore edilip
md5 ile doğrulandı (`mr018-mutant-m{1,2,3}-*.bak`). M5 (çözülmemiş isim
dönmek) kırmızı vermedi — davranış-koruyucu mutant olarak kayıtlı,
yerine M5' koşuldu.

`make check` **yeşil** (exit 0). Test sayısı **1276** (TASK-01 başında 1260).

## TASK-01 kapı remediasyonu (Breaker 5 bulgu → 5 düzeltme)

Reader 5/5 CONFIRMED verdi (F1/F2 INFO: boş-olmayan temiz aralık ve
merge-base-ci testi TASK-02'ye bırakıldı). Breaker 5 GERÇEK buldu;
tamamı aynı turda kapatıldı:

| # | Bulgu | Düzeltme | Pin |
|---|---|---|---|
| B1 | Ters aralık (`base` head'in torunu) boş diff'le yeşil onaylıyordu | `mergeBase == head && base != head` usage-reddi (SHA'lı mesaj) | `TestVerifyCIInvertedRangeRefuses` (M8) |
| B2 | Default-base sessizce boş aralığa çözümleniyordu (HEAD base'in gerisinde) | B1 reddi + çözümlenen SHA'nın mesajda adlandırılması; sessiz-yeşil kapandı | B1 pini + `TestVerifyCIWorktreeMismatchRefuses` mesaj denetimi |
| B3 | CI yolu worktree baytlarına dokunuyordu (`IndexFile` diskten okur); head-gerisi checkout sınıflanmamış çöküyordu (boş kod) | Önkoşul: worktree HEAD == judged head, aksi hâlde usage-reddi | `TestVerifyCIWorktreeMismatchRefuses` (M9) |
| B4 | `compose` tüm NULL-task satırlarını union'lıyordu: CI commitlenmemiş içeriği, staged commitli içeriği yargılıyordu | `AttributeChanges(ctx, [changeID])`: her mod yalnız kendi satırlarını attribute eder | `TestVerifyCIRangeScoping`, `TestVerifyStagedIgnoresCIRows` (M11) |
| B5 | Knowledge worktree'den okunuyordu; commitli-bozuk + düzeltilmiş-desk yeşil geçiyordu | Önkoşul: `.mindrail/`/`.git/` dışı kirli desk usage-reddi (dosyalar adlandırılır) | `TestVerifyCIDirtyDeskRefuses` (M10), dışlama pini (M12) |

**Önkoşul kaydı (D-222 uygulaması):** `VerifyCI` temiz checkout ister —
worktree HEAD judged head'e eşit, desk `.mindrail/`/`.git/` dışında temiz.
Fresh clone bunu yapısal olarak sağlar (TASK-02); kirli desk yalan
söylemek yerine reddedilir.

**Bilinen kısıt (kapsam-dışı, dürüst kayıt):** sembol satırları index
durumuna göre delta'dır; aynı veritabanında staged-yargıdan sonra
koşulan yerel `--ci` satır göremeyip eksik-deny verebilir (union
kalkmadan önce tam tersi kirlenme vardı — B4). Otoriter kapı fresh-clone
CI'dır (ayrı DB); parite testi bu yüzden faz-arasında DB sıfırlar.
Staged yolunun modlar-arası aynı delta-görünürlüğü MR-017'den devralındı,
değiştirilmedi.

**Gözlem (kapsam-dışı):** init'siz dizinde `verify` (staged dahil, önceden
var) boş kodla düşüyor — bootstrap davranışı, burada değiştirilmedi.

### Remediasyon mutasyon defteri (tamamı geri alındı, md5-doğrulamalı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M8 | inversiyon reddi kapatıldı | `TestVerifyCIInvertedRangeRefuses` FAIL |
| M9 | worktree-eşleşme reddi kapatıldı | `TestVerifyCIWorktreeMismatchRefuses` FAIL |
| M10 | kir-desk reddi kapatıldı | `TestVerifyCIDirtyDeskRefuses` FAIL |
| M11 | attribution union'a döndürüldü | `TestVerifyCIRangeScoping` + `TestVerifyStagedIgnoresCIRows` FAIL |
| M12 | kir-dışlama (`.mindrail/`) düşürüldü | `TestVerifyCICleanGreen` FAIL |

`M1`/`M6` ilk yazımda kırmızı vermedi (test kurgusu reddi başka yoldan
üretiyordu); testler güçlendirildi (`--staged --base` reddi, commitli
çift-mod), sonra kırmızı koşuldu. `M5` davranış-koruyucu çıktı, `M5'`
koşuldu.

Remediasyon sonrası `make check` **yeşil** (exit 0). Test sayısı **1281**.
Re-gate: Reader + Breaker ikinci tur (aşağıda).
