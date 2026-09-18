# MR-005 — Bulgular ve kapı kaydı

Bu kayıt, dondurulmuş [gereksinimlerin](mr-005-requirements.md) TASK-01
kapısı içindir. TASK-02 yazılmadan önceki Reader/Breaker değerlendirmesini ve
bu turdaki kanıtları korur; gereksinim veya tasarımın yerine geçmez.

## Başlangıç ve çalışma ağacı

- Başlangıç HEAD'i `40c42d0` idi. Dondurulmuş gereksinimlerin §0 tablosunda
  kaydedilen `make verify` ve `make tidy-check` yeşildi.
- Bu turdaki ilk WIP migration/store hali kırmızıydı; kapı, bu kırmızıyı
  gizlemeden Reader ve Breaker geri bildirimleriyle tamamlandı.
- Çalışma ağacı hâlâ commitlenmemiştir. `.claude/**` ve `graphify-out/**`
  altındaki kirli dosyalar kullanıcıya ait/ilgisiz kabul edildi; TASK-01
  değerlendirmesine veya bu kayda taşınmadı.

## TASK-01 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-01.1 | Karşılandı | `migrations/000004_index.sql`: yalnız `project_units`, `file_index_state`, `symbols`, `symbol_imports`, `symbol_references`; state CHECK'i D-81 değerlerini sınırlar. |
| AC-01.2 | Karşılandı | `internal/index/index.go`: `TableSchemaVersion = 4`; `internal/index/index_test.go` ledger'ın en yüksek sürümüne bağlar. |
| AC-01.3 | Karşılandı | `internal/app/code.go` üç index kodunu ve ayrı remedy'lerini kaydeder; registry/allCodes ve envelope sınıflandırma testleri güncellendi. |
| AC-01.4 | Karşılandı | `migrations/shipped_test.go` 000004 byte checksum'ını pinler; migration ledger/shape testleri beş tablo ve beklenen sütunları kapsar. |
| AC-01.5 | Karşılandı | `internal/cli/upgrade_test.go` schema-3 fixture'ını 000004'e yükseltir ve koordinasyon satırlarının korunduğunu doğrular. |

Raporlanan doğrulama komutları: `make check`, `make verify`, `make tidy-check`
ve `go test -list '.*' ./... | grep -c '^Test'`. Son kabulde bağımsız delta
değerlendirmesi Reader **PASS**, Breaker **VERIFIED** sonucuna ulaştı.

## Reader / Breaker bulguları ve giderim

| Bulgu | Sonuç ve giderim |
|---|---|
| B1 — typed-nil | Store sınırlarında typed-nil hata/sonuç yüzeyleri denetlendi; ilgili guard ve test kanıtları eklendi. |
| B2 — schema gate | Store'un migration-4 öncesi şemayı boş veri gibi okumaması için schema gate doğrulandı. |
| B3 — pointer ambiguity | `resolved_symbol_id` nullable kaldı; belirsiz/çözümsüz referansın NULL olması hata veya sahte çözüm sayılmadı. |
| B4 — guard gaps | Mutasyon envanteri genişletildi; kaçan guard'lar kontrol/mutant kanıtıyla kapatıldı. |

### D-89 — referans hedefi silinince `ON DELETE SET NULL`

`symbol_references.resolved_symbol_id`, yapısal eşleşmenin opsiyonel bir
ipuçudur; reference gerçeğinin kendisi değildir. Hedef sembol silinince
reference satırını silmek veya işlemi FK hatasıyla durdurmak, D-87'nin
"çözümsüz/ambiguous referans da bir olgudur" kuralını bozar. Bu nedenle FK
`ON DELETE SET NULL` kullanır: hedef pointer'ı kalkar, reference satırı ve
STRUCTURAL bağlamı kalır. Kontrol ve mutant çalışmaları bu davranışın hem
korunduğunu hem de kaldırıldığında testin kırmızıya döndüğünü gösterdi.

## Mutasyon kapsamı

- Store envanteri: 70 adayın 65'i kırmızıya döndü. Üç Go istisnası gerekçeli:
  - `store.go:248` ve `store.go:414`: `json.Marshal([]string)` mevcut tip
    sözleşmesinde erişilemez hata yoludur.
  - `store.go:279`: unique-ID uyuşmazlığı PK ve aynı transaction invarianti
    altında erişilemezdir.
- Bu üç istisna, ilgili type/schema/transaction sınırı değişirse yeniden
  mutasyon gerektirir; özellikle slice tipi, ID üretimi, PK/UNIQUE kuralı veya
  transaction ayrımı değiştiğinde istisna geçersiz sayılır.
- State CHECK, migration düzeyinde kapsanmıştır. Error envanterinde 35 adayın
  30'u kırmızıya döndü; 268 numaralı yol tam suite ile kapsandı. SQL/schema
  envanterinde 31 mutant kırmızıya döndü. Son store turunda 449 mutant
  kırmızıya döndü.
- Kapsam sınırı: her boolean yaprak ve her `NOT NULL` sütunu tek tek mutant
  yapılmadı; yukarıdaki envanter ve migration kontrol/mutantları seçilmiş
  guard'ları kanıtlar, evrensel mutasyon iddiası değildir.

## Denetim artefaktları

Geçici artefaktlar bu çalışma ortamında saklanmıştır:

- `/tmp/mr005-breaker-CsHEvo/BREAKER_REPORT.md`
- `/tmp/mr005-breaker-CsHEvo/mutation-results.json`
- `/tmp/mr005-breaker-delta-9QNGkx/DELTA_BREAKER_REPORT.md`
- `/tmp/mr005-breaker-delta-9QNGkx/mutation-inventory.json`
- `/tmp/mr005-breaker-final-C4PhcQ/FINAL_BREAKER_REPORT.md`
- `/tmp/mr005-breaker-final-C4PhcQ/{mutation-inventory,error-mutation-results,sql-mutation-results}.json`
- `/tmp/mr005-error-evidence-TIiZX1/{empty-cancellation-mutation,error-mutation-results}.json`

Bu dosyalar `/tmp` altında olduğundan kalıcı proje kaydı değildir; bu özet
onların sonuçlarını, ölçüm uydurmadan, TASK-01 için kalıcılaştırır.
