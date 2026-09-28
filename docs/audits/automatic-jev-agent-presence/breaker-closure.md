# RDR-F01 Breaker closure

Tarih: 2026-09-28  
Rol: bağımsız Breaker closure  
Kapsam: yalnız RDR-F01 legacy ClientInfo read-boundary sızıntısı  
Sonuç: **APPROVE — RDR-F01 yeniden üretilemedi; açık bulgu yok**

Bu rapor Reader closure sonucu görülmeden yazıldı. Implementation dosyaları
değiştirilmedi. Bütün attack/mutation deneyleri
`/tmp/mindrail-breaker-closure/repo` scratch kopyasında, disposable SQLite
veritabanları ve ayrı Go cache/build dizinleriyle çalıştırıldı.

## Yönetici özeti

RDR-F01'in eski saldırısı aynı legacy DB row şekliyle ve gerçek dashboard
transport'ları üzerinden tekrarlandı. Legacy row içinde üç ayrı marker kalmaya
devam ederken hiçbir marker Store `Get`, Store `ListProject`, dashboard SSE veya
REST snapshot çıktısına ulaşmadı. Public family `unknown-client` oldu; raw title
ve version JSON key'leri tamamen kaldırılmış durumda.

İki bağımsız read boundary canonicalization'ı mutation ile kapatıldı:

- Store `scanRuntime` guard'ı kaldırılınca `Get` ve `ListProject` **3/3 marker**
  sızdırdı ve testler kırmızı oldu.
- Dashboard collector guard'ı kaldırılınca public family üzerinden **1/3 marker**
  sızdı ve üç regression testi kırmızı oldu.
- Eski public model + raw collector davranışı birlikte geri getirildiğinde SSE ve
  REST yeniden **3/3 marker** sızdırdı.

## Control/scenario ve before/after sayıları

Test credential sentinel'i `TYPESAFE_API_KEY` olarak name marker'a bağlandı.
Title/version için ayrı opaque secret marker'ları kullanıldı. Control ve scenario
aynı project/workspace/task/session/runtime/timestamp verisini kullandı; yalnız
ClientInfo kolonları değişti.

| Okuyucu | Control canonical row | Legacy scenario DB | Eski/raw davranış | Final davranış |
| --- | ---: | ---: | ---: | ---: |
| DB'nin kendisi | marker `0/3` | marker `3/3` | `3/3` | `3/3` — row rewrite yok |
| PresenceStore `Get` | marker `0/3`, family `codex` | marker `3/3` | `3/3` | **`0/3`, family `unknown-client`** |
| PresenceStore `ListProject` | marker `0/3`, family `codex` | marker `3/3` | `3/3` | **`0/3`, family `unknown-client`** |
| SSE first snapshot | marker `0/3`, 2.141 byte | marker `3/3` | `3/3` | **`0/3`, 2.150 byte, `unknown-client`** |
| REST `/api/snapshot` | marker `0/3`, 2.117 byte | marker `3/3` | `3/3` | **`0/3`, 2.126 byte, `unknown-client`** |
| Public title/version keys | 0 | legacy DB'de 2 | 2 | **0** |

Final legacy ölçümü:

```text
Store Get:       marker_hits=0, family=unknown-client
Store List:      marker_hits=0, family=unknown-client
SSE:             marker_hits=0, bytes=2150
REST snapshot:   marker_hits=0, bytes=2126
Persisted DB:    marker_hits=3
```

Legacy DB'nin değişmeden kalması beklenen davranıştır: remediation destructive
data rewrite yapmadan her read boundary'de güvenli projection uygular.

## B1 — Guard mutation

Mutasyonlar yalnız scratch source'a uygulandı. Compile-only kırılma kanıt
sayılmadı; dashboard ilk kaba mutation'da unused import ürettiği için davranışsal
mutation'a çevrilip yeniden çalıştırıldı.

| # | Neutralize edilen koruma | Control | Mutation scenario | Sonuç |
| --- | --- | ---: | ---: | --- |
| CM1 | `scanRuntime` sonundaki `CanonicalClientInfo` kaldırıldı | Get/List `0/3` | Get/List `3/3` | RED, guard testli |
| CM2 | collector canonical family yerine raw `client_name` kullandı | SSE/REST `0/3` | SSE/REST `1/3` | RED, guard testli |
| CM3 | retired title/version public modele ve raw collector'a geri eklendi | SSE/REST `0/3` | SSE/REST `3/3`; control'de retired key 2 | RED, deleted behavior testli |

Mutation sonrasında source hash'leri orijinal snapshot ile birebir eşitlendi.
Green surviving guard yok.

## B2 — Deleted behavior A/B

RDR-F01'in ürettiği eski davranış scratch'te yeniden kuruldu:

1. collector family'yi raw `client_name`'den aldı;
2. public `AgentRuntime` modeline `client_title` ve `client_version` geri eklendi;
3. raw title/version collector'dan modele kopyalandı.

Aynı legacy row eski davranışta SSE ve REST'e **3/3 marker** taşıdı. Final
snapshot'ta aynı row **0/3** taşıyor ve title/version key sayısı `2→0`. Bu fark
security amendment'ın istediği farktır; canonical control row'ın family bilgisi
`codex` olarak korunmuştur.

## B3 — Threat model

| Tehdit | Concrete attempt | Sonuç |
| --- | --- | --- |
| Keyring/environment credential eski name alanında | name marker aynı zamanda `TYPESAFE_API_KEY` | DB 1 marker taşır; bütün read/public yollar 0 |
| Opaque secret eski title alanında | distinct printable marker | DB taşır; Get/List/SSE/REST 0 |
| Opaque secret eski version alanında | distinct printable marker | DB taşır; Get/List/SSE/REST 0 |
| Browser render etmese bile wire leak | gerçek SSE first flush ve REST handler body | iki transport da 0/3 |
| Internal store caller raw row alır | doğrudan `Get` ve `ListProject` JSON | ikisi de 0/3 |
| Remediation veriyi sessizce siler | probe sonrası raw DB tekrar okundu | DB hâlâ 3/3; destructive rewrite yok |

Default regex redactor'a güvenilmedi. Marker'lar collector'a raw DB'den verildi;
güvenlik `CanonicalClientInfo` projection'ından geldi.

## B4 — Upgrade ve backward compatibility

İki existing-installation şekli çalıştırıldı:

1. **Schema 10 → 11:** migration/upgrade/shipped testleri PASS; eski row'lar ve
   idempotence önceki contract'a uygun.
2. **Pre-remediation schema-11 legacy runtime row:** raw name/title/version doğrudan
   DB'ye seed edildi. Final binary ilk read'de row'u rewrite etmeden güvenli family
   döndürdü.

Komut:

`go test ./internal/agent ./internal/dashboard ./internal/migration ./migrations -count=1`

Dört paket PASS. Existing dashboard consumer'ın public raw title/version alanlarını
kaybetmesi explicit post-audit security amendment'ın amaçlanan compatibility
değişimidir. Runtime ID, timestamps, state, duration ve canonical family korunur.

## B5 — Weakest satisfying inputs

| Predicate/boundary | En zayıf ayırıcı input | Sonuç |
| --- | --- | --- |
| Known family lookup | `codex` + arbitrary title/version | family `codex`; raw optional alanlar yok |
| Unknown family lookup | bounded printable credential marker | `unknown-client`; marker 0 |
| Store single-row read | bir legacy row + `Get` | marker 0 |
| Store collection read | limit 10, bir legacy row | marker 0 |
| Public SSE | ilk flush'taki tek snapshot | marker 0 |
| Public REST | tek authenticated loopback GET | marker 0 |
| Persistence non-rewrite | read'den sonra aynı DB row | marker 3 |

Amaç dışı accept veya over-refusal üretilmedi: known family kalırken arbitrary
caller text atıldı.

## B6 — Dual readers ve class sweep

Aynı legacy row bütün okuyuculara verildi:

| Reader | Family | Raw marker |
| --- | --- | ---: |
| `PresenceStore.Get` | `unknown-client` | 0/3 |
| `PresenceStore.ListProject` | `unknown-client` | 0/3 |
| Dashboard collector → SSE | `unknown-client` | 0/3 |
| Dashboard collector → REST | `unknown-client` | 0/3 |

Class sweep sonucu:

- `agent_runtimes` production reader'ları yalnız Store `scanRuntime` ve dashboard
  collector'dır; ikisi ayrı canonicalization guard'ına sahiptir.
- `Heartbeat`, `Activity` ve `End` dönüşleri `Get` üzerinden geçtiğinden Store
  projection'ını paylaşır.
- SSE ve REST aynı bounded `Server.payload` yolunu paylaşır.
- Public dashboard modelinde yalnız `ClientFamily` bulunur; raw title/version için
  sibling JSON field kalmamıştır.
- Browser yalnız projected `client_name` family değerini okur.

Reader'lar arasında ayrışma üretilmedi.

## Mechanical rule = automated test

| Mekanik kural | Otomasyon |
| --- | --- |
| Legacy raw row Store Get/List'ten çıkamaz | `TestPresenceStoreProjectsLegacyClientMetadataOnEveryReadWithoutRewriting`; CM1 killed |
| Legacy raw row public dashboard JSON'a çıkamaz | collector regression testi; CM2/CM3 killed |
| REST snapshot raw marker taşımaz | server regression testi; CM2/CM3 killed |
| Raw title/version public schema'da yok | collector/server/client contract assertions |
| Raw DB remediation sırasında rewrite edilmez | her iki package testinde post-read DB assertion |
| Known canonical family yararlı kalır | control `codex→codex` |

SSE için production kod testinde mevcut exact first-flush attack bu Breaker
scratch probunda çalıştırıldı. REST ve collector kalıcı regression testleri,
shared payload ise SSE sibling'ini mekanik olarak kapsar.

## Maliyet

- Store `ListProject` üst sınırı **200 row**; row başına tek map lookup/projection.
- Dashboard üst sınırı **120 runtime/snapshot**; her runtime bir projection.
- Legacy writer'ın önceki bounded alanları üzerinden maksimum yaklaşık
  `200 × (128+256+64) = 89.600` karakter Store read input'u ve
  `120 × 448 = 53.760` karakter dashboard projection input'u işlenir.
- SSE snapshot hard limit'i **256 KiB**, stream **8**, interval **1.5s**:
  `8 × 40 × 256 KiB = 80 MiB/dakika` teorik loopback üst sınırı değişmedi.
- Projection DB rewrite yapmadığından migration sırasında O(N) update/lock maliyeti
  eklenmedi.

Yeni unacceptable cost veya amplification üretilmedi.

## Validation

- Focused attack tests: PASS.
- Agent/dashboard/migration/shipped package testleri: PASS.
- Race:
  `go test -race ./internal/agent ./internal/dashboard -run 'Presence|Runtime|Legacy|Client|Snapshot' -count=1`
  PASS; agent `3.411s`, dashboard `1.438s`.
- İki read-boundary ve deleted-public-model mutation'ı davranışsal olarak RED.

## Final karar

**APPROVE.** RDR-F01'in exact legacy-row saldırısı Store `Get`, Store
`ListProject`, SSE ve REST yollarında `3/3→0/3` kapandı. Canonical control family
korundu; raw DB destructive biçimde değiştirilmedi; tests mutation'ları öldürdü;
upgrade ve race yolları geçti. Critical, HIGH, MEDIUM veya LOW açık bulgu yok.

## İzolasyon

- Orijinal çalışma ağacında yalnız bu `breaker-closure.md` oluşturuldu.
- Implementation ve Reader closure dosyaları değiştirilmedi; Reader closure sonucu
  bu rapor yazılırken okunmadı.
- Scratch DB, mutation source ve cache kullanıcı/shared runtime'dan ayrıdır.
