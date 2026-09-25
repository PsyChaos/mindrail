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
