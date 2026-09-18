# MR-005 — Bulgular ve kapı kaydı

Bu kayıt, dondurulmuş [gereksinimlerin](mr-005-requirements.md) TASK-01 ve
TASK-02 kapılarını kaydeder. Reader/Breaker değerlendirmelerini ve bu turdaki
kanıtları korur; gereksinim veya tasarımın yerine geçmez.

## Başlangıç ve çalışma ağacı

- Başlangıç HEAD'i `40c42d0` idi. Dondurulmuş gereksinimlerin §0 tablosunda
  kaydedilen `make verify` ve `make tidy-check` yeşildi.
- Bu turdaki ilk WIP migration/store hali kırmızıydı; kapı, bu kırmızıyı
  gizlemeden Reader ve Breaker geri bildirimleriyle tamamlandı.
- TASK-01, `4548f38` commit'iyle kapatıldı. Son `make verify` ve
  `make tidy-check` yeşildi; `go test -list '.*' ./... | grep -c '^Test'`
  sonucu 906 idi. `.claude/**` ve `graphify-out/**` altındaki kirli dosyalar
  kullanıcıya ait/ilgisiz kabul edildi; TASK-01 değerlendirmesine veya bu
  kayda taşınmadı.

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

## TASK-02 kabul kanıtı

Bu bölüm yalnız TASK-02 uygulama/validation kanıtıdır; bağımsız Reader/Breaker
kapısının sonucunu iddia etmez.

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-02.1 | Karşılandı | `internal/index/inventory.Discover`, `pyproject.toml` ve `package.json` marker'larını, aynı dizindeki `tsconfig.json` ile TypeScript'e rafine eder; `TestDiscoverFindsDeterministicPythonTypeScriptAndJavaScriptUnits` üç birimi path sırasıyla doğrular. `Owner` sınır-güvenli `filepath.Rel` ile en uzun kökü seçer; nested-root testi bunu doğrular. |
| AC-02.2 | Karşılandı | Sıfır birim kalıcı inventory sonucu `inventory: OK, "0 project units discovered"`, `syntax.phase: "INVENTORY"` ve genel `PARTIAL_READY` üretir. Bu, boş envanter için READY iddiasını engeller; `TestBuildReportsInventoryPhaseWithoutRescanning` kanıtıdır. |
| AC-02.3 | Karşılandı | `Discover` canonical repository root altında `WalkDir` ile yalnızca okur; symlink dizinlerini izlemez ve `.git`, `.mindrail`, `node_modules` altını atlar. `TestDiscoverIsIdempotentAndDoesNotTouchTheRepository`, tekrarlı pass'te aynı ID/zamanı, tek unit satırını, sıfır `file_index_state` satırını ve değişmeyen marker mtime'ını doğrular; ayrı symlink-escape testi dış kökü reddeder. |
| AC-02.4 | Karşılandı | Bootstrap mevcut `load_index_state` (§87 step 8) içinde non-blocking discovery yapar; step listesine ek bloklayıcı adım eklenmemiştir. Read-only startup yalnız `Store.ListUnits` ile persisted sonuçları okur. `TestStartupDiscoversInventoryAtTheExistingIndexStateStep` ve marker silindikten sonraki `TestReadOnlyStartupReportsPersistedInventoryWithoutWalkingSource` status'un filesystem rescan yapmadığını doğrular. |

### TASK-02 karar kaydı — boş envanter lifecycle uyuşmazlığı

Dondurulmuş tasarım §9, sıfır unit için `UNINITIALIZED` der; dondurulmuş
gereksinim AC-02.2 ise açıkça `INVENTORY` ve "no READY" ister. TASK-02,
ayrıntı kabul kriterini izler: `syntax.phase = INVENTORY` ve genel
`PARTIAL_READY`. Tasarım dosyası dondurulmuş olduğu için değiştirilmedi; bu
kayıt uyuşmazlığı görünür kılar.

### TASK-02 mutasyon ve doğrulama

- Longest-root guard'ın gerçek olduğunu göstermek için `Owner` içindeki
  `len(unit.Path) > len(owner.Path)` karşılaştırması geçici olarak `<` yapıldı.
  `GOCACHE=/tmp/mindrail-go-build go test ./internal/index/inventory -run
  '^TestOwnerUsesTheLongestContainingUnitRoot$' -count=1` beklenen kırmızıyı
  verdi: nested path `UNT-child` yerine `UNT-parent` seçildi. Doğru guard geri
  yüklendi.
- Geri yükleme sonrası focused doğrulama:
  `GOCACHE=/tmp/mindrail-go-build go test ./internal/index/inventory
  ./internal/index ./internal/status ./internal/bootstrap` yeşildi.
- Tam doğrulama: `GOCACHE=/tmp/mindrail-go-build go test ./...` yeşildi.
- Kod grafiği değişiklikten sonra `graphify update .` ile yenilendi. Bu
  çalışma ağacında önceden var olan `graphify-out/**` ve
  `internal/cli/testdata/doctor_human.golden` değişiklikleri TASK-02 kanıtı
  değildir; sonuncusu TASK-01'in schema-v4 golden farkıdır ve bu turda
  değiştirilmedi.

## TASK-02 Breaker giderimi

Breaker'ın doğruladığı dört eksik aşağıdaki şekilde giderildi; bu bölüm önceki
TASK-02 kanıtını geçersiz kılmaz, remediation delta'sını kaydeder.

- **Worktree kapsamı:** `Store.ListUnits(ctx, root)` artık canonical ve temiz
  mutlak root ile sınırlandırılır. SQL predicate'i `LIKE` kullanmaz; `%`/`_`
  path byte'ları ile `/web`–`/website` prefix çakışmasını separator kontrolüyle
  kapatır. Bootstrap her iki read/write yolunda `inventory.CanonicalRoot`u
  geçirir; Store filesystem erişimi yapmaz. Gerçek sibling linked-worktree ve
  nested registered-worktree testleri, yabancı unit'in status'a sızmadığını
  doğrular.
- **Stale reconcile:** Tam `WalkDir` ve bütün upsert'ler başarılı olduktan
  sonra `ReconcileUnits` tek SQLite transaction'ında stale root'un
  `symbol_references → symbol_imports → symbols → file_index_state →
  project_units` olgularını siler. Cancel, walk veya upsert hatasında bu çağrı
  hiç yapılmaz. `workspaces.root_path` içindeki yabancı registered worktree
  altı korunur; migration-4 schema gate'i bu MR-001 tablosunun zaten mevcut
  olmasını garanti eder. Workspace satırları bu Store tarafından değiştirilmez.
- **Context ve symlink:** Discovery girişte ve walk/upsert/reconcile sınırında
  context'i denetler; pre-cancelled boş discovery `context.Canceled` döner.
  File-symlink marker'ı kalıcı testle dışlanır (yalnız directory-symlink
  davranışına güvenilmez).
- **Rollback:** `BEFORE DELETE project_units` injected failure'ı, stale prune
  transaction'ının hem unit hem file-state satırını koruduğunu doğrular.

Giderim guard mutasyonları ve beklenen kırmızıları:

1. Symlink predicate devre dışı bırakıldı →
   `TestDiscoverDoesNotTreatASymlinkedMarkerFileAsInventory` symlinked
   `pyproject.toml` için Python unit üreterek kırmızı oldu.
2. Pre-cancel check devre dışı bırakıldı →
   `TestDiscoverPrioritizesCanceledContextOverAConfigurationError` nil-store
   hatası yerine `context.Canceled` beklerken kırmızı oldu.
3. SQL separator predicate'i kaldırıldı →
   `TestDiscoverReconcilesRemovedRootWithoutTouchingSiblingRoot` `/web`
   reconcile'ının `/website` unit'ini de sildiğini göstererek kırmızı oldu.
4. `file_index_state` delete adımı çıkarıldı → aynı reconcile testi FK
   constraint hatasıyla kırmızı oldu.
5. Registered-foreign-worktree filtresi devre dışı bırakıldı →
   `TestNestedRegisteredWorktreeIsExcludedFromParentInventoryAndPruning`
   nested unit'in prune edildiğini göstererek kırmızı oldu.

Her mutant geri alındı. Giderim sonrası focused komut
`GOCACHE=/tmp/mindrail-go-build go test ./internal/index/inventory
./internal/index ./internal/bootstrap ./internal/status` yeşildi. Host'un
30-saniyelik command penceresi `go test ./...` çıktısını CLI sonrası kesmeye
başladığından tam package kümesi iki bounded çalışmada doğrulandı:
`go test ./internal/cli` ve CLI dışındaki `go list ./...` package'larının
eksiksiz explicit listesi; ikisi de yeşildi. `git diff --check` de yeşildi.

### TASK-02 final delta — filesystem root kapsamı

Delta Reader/Breaker'ın bulduğu root `"/"` hatası kapatıldı. Normal SQL
predicate'i child separator'ını ayrıca aradığından `/` için `//` prefix'i
üretiyor, bu da descendant unit'leri hem `ListUnits` hem `ReconcileUnits`
tarafından görünmez kılıyordu. `pathInRootSQL`, filesystem/volume root için
root'un zaten taşıdığı separator'ı boundary kabul eden ayrı predicate kullanır;
normal `/web`–`/website` koruması değişmeden kalır.

`TestStoreScopesAndReconcilesAtFilesystemRoot`, temp altında persist edilmiş
unit'in `ListUnits(ctx, "/")` ile göründüğünü ve
`ReconcileUnits(ctx, "/", nil)` ile silindiğini doğrular. Root branch'i
geçici olarak devre dışı bırakıldığında aynı test beklenen kırmızıyı verdi
(0 unit); guard geri yüklendi. Geri yükleme sonrası focused
`go test ./internal/index/inventory ./internal/index ./internal/bootstrap
./internal/status` ve `git diff --check` yeşildi.

### TASK-02 final kapı

Son bağımsız değerlendirme Reader **PASS**, Breaker **VERIFIED** sonucunu
verdi (root kapsamı gideriminden sonra). `GOCACHE=/tmp/mindrail-go-build make
verify`; `go vet`, normal test, race test ve smoke aşamalarında yeşildi.
`make tidy-check` de yeşildi. Test sayısı
`go test -list '.*' ./... | grep -c '^Test'` ile 921 olarak kaydedildi.
