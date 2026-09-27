# Mindrail 0.1 — merkezi doğrulama kaydı

Tarih: 2026-09-26. Kaynak: `2814acb22d8aa94d48ade1242b58e1fdb14239fc`.
Ortam: `go1.27.0 linux/amd64`, AMD Ryzen AI 9 HX 370. Bu kayıt ana denetçi tarafından çalıştırılan kontrolleri içerir; bağımsız Reader/Breaker sonuçlarının yerine geçmez.

> **Tarihsel snapshot notu (remediation sonrası eklendi):** Buradaki komutlar ve ölçümler yalnız `2814acb` binary'sine aittir; özellikle `mcp_compatibility:"none"`, schema 8 ve ham `READY_TO_COMPLETE → COMPLETED` sonucu güncel davranış değildir. Sonraki çalışma ağacı schema 9 guard baseline, stdio MCP launcher ve evaluator üzerinden `task complete` uygular. Tarihsel ölçümleri yeni release kanıtı diye okumayın; güncel arayüz için [kılavuza](../../usage-tr.md) bakın.

## Çalışma ağacı ve izolasyon

Başlangıçta ürün kaynaklarında değişiklik yoktu. Önceden kirli `.claude/`, `.gitignore`, `graphify-out/`, `.wrongstack/`, `.zcode/` bu denetime dahil edilmedi ve değiştirilmedi. Üretim kaynaklarına düzeltme uygulanmadı.

Release ve dependency-tidy denemeleri `git clone --quiet --no-hardlinks . /tmp/mindrail-release-audit-ZzpRaU/release-src` ile oluşturulan temiz yerel kopyada yapıldı. Örnek CLI işlemleri ve ajanların hata denemeleri ayrı geçici Git repolarında çalıştı; kullanıcının repository'sinde `mindrail init` veya gate mutation yapılmadı.

## Çalıştırılan kontroller

Go komutlarında yazılabilir cache olarak `GOCACHE=/tmp/mindrail-go-build` kullanıldı.

| Kontrol | Sonuç | Kanıt / sınır |
|---|---|---|
| `make verify` | PASS, exit 0 | Format + vet + normal testler + race + derlenmiş binary smoke. Normal ve race aşamalarında 33'er paket başarılı; her aşamada 18 paket cache sonucu. Smoke `-count=1`, 2.764 s. CLI normal 44.399 s, race 295.388 s. |
| `make tidy-check` | PASS, exit 0 | Temiz release kopyasında; `go.mod`/`go.sum` tutarlı. |
| `go test -list '.*' ./... \| grep -c '^Test'` | 1308 | Bu bir test fonksiyonu sayımıdır; 1308 bağımsız davranış veya eksiksiz requirement coverage iddiası değildir. |
| `make gate` | PASS, exit 0 | Dokuz kategori yeşil; her seçimde en az bir gerçek PASS kontrolü mevcut. |
| `make bench` | PASS, exit 0 | Aşağıdaki warm-path ölçümleri, her seri 100 örnek. |
| `make release` | PASS, exit 0 | Temiz kopyada stamp, platform matrisi, SHA256 ve native smoke. |
| `sha256sum -c SHA256SUMS` | PASS | `mindrail-linux-amd64: OK`. |
| `make vuln` | SKIPPED | `govulncheck` kurulu değil. Güvenlik açığı taraması yapılmış veya temiz çıkmış sayılmaz. Bu denetim harici CI çalıştırmasını doğrulamadı. |
| Ürün yolu diff kontrolü | Temiz | `git diff --name-only -- internal cmd migrations go.mod go.sum Makefile scripts` boş çıktı. |

Test sayımı ile yürütme sayısı arasındaki fark ayrıca incelendi: `go test -v ./...` çıktısında 1306 top-level PASS, iki top-level SKIP vardı:

- `TestResolveRejectsDirectoryOutsideAnyRepository`: ortamda `/tmp` de bir Git repository'si olduğundan fixture “bütün repoların dışında” koşulunu sağlayamadı. Bu ortamın var olan Git durumuna müdahale edilmedi.
- `TestTheOwnershipRuleHoldsOverTheWiderSpace`: `MR002_WIDE_ENUMERATION=1` isteyen, tür başına 110.592-store geniş tarama opt-in testi bu koşuda açılmadı.

Alt-test düzeyindeki her platform koşulunun çalıştığı iddia edilmez. Yeşil paket sonucu ve sayım, bağımsız karşı örneklerin çürütülmesi değildir.

## Kalite kategorileri

`scripts/gate.sh` tarafından bildirilen top-level PASS sayıları birbirleriyle örtüşür; toplanıp benzersiz test sayısı olarak sunulmamalıdır.

| Seçim | PASS |
|---|---:|
| unit | 1306 |
| domain | 406 |
| integration | 10 |
| race (concurrency seçimi) | 9 |
| knowledge-schema | 216 |
| mcp-contract | 26 |
| git-worktree | 77 |
| git-worktree-named | 7 |
| sqlite-concurrency | 17 |
| end-to-end | 10 |
| end-to-end-smoke | 3 |

## Warm-path ölçümleri

| Operasyon | p50 | p95 | Testteki p95 hedefi |
|---|---:|---:|---:|
| MCP status | 0.286 ms | 0.870 ms | 150 ms |
| MCP context | 0.309 ms | 0.882 ms | 250 ms |
| MCP before_change | 0.740 ms | 1.634 ms | 300 ms |
| MCP after_change | 3.081 ms | 4.461 ms | 1500 ms |
| MCP reconcile | 5.195 ms | 6.322 ms | 2000 ms |
| changes after_change servisi | 1.672 ms | 2.290 ms | 1500 ms |
| changes reconcile servisi | 13.102 ms | 16.532 ms | 2000 ms |
| impact traversal | 0.096 ms | 0.123 ms | Bu satırda ayrı hedef ilan edilmedi |

Bunlar repository'deki sentetik fixture'ların ölçümüdür. Gerçek büyük projeler, cold start ve erişilebilir bir MCP process launcher için uçtan uca performans garantisi vermez. Kaynak: `make bench`; her seri `samples=100`.

## Release çıktısı ve public yüzey

Temiz clone'daki stamp:

```json
{"command":"version","ok":true,"data":{"version":"0.1.0","commit":"2814acb","build_date":"2026-09-26T08:24:58Z","dirty":"clean","go":"go1.27.0","platform":"linux/amd64","write_schema_version":1,"readable_schema_versions":[1],"mcp_compatibility":"none"}}
```

Platform matrisi: `linux/amd64` üretildi; `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64` Tree-sitter Go/C binding build constraints nedeniyle `not-built` oldu. Makefile bu hedefleri best-effort olarak raporluyor. Bu çalışmada diğer platformların kullanılabilirliği doğrulanmadı.

Public CLI kontrolü:

```text
mindrail --help        => exit 0; MCP/serve alt-komutu listelenmiyor
mindrail version --json => mcp_compatibility: "none"
mindrail mcp --json    => exit 2; COMMAND_LINE_INVALID
                          unknown command "mcp" for "mindrail"
```

Buradaki MCP eksikliği tüm handler kodunun yokluğu anlamına gelmez: `internal/mcp` test edilebilir servis/transport kodu içerir. Eksik olan, yayımlanabilir binary üzerinden kullanıcı istemcisinin başlatabileceği production girişidir. Bu fark [Reader](reader.md), [Breaker](breaker.md) ve [dokümantasyon](documentation.md) raporlarında sözleşme katmanlarıyla birlikte değerlendirilir.

## Yorum

Ek public CLI deneyi: `task state` üzerinden `READY_TO_COMPLETE → COMPLETED`, reconcile/evidence bulunmayan temiz ve değiştirilmiş task'larda exit 0 verdi; revision 5 yazıldı ve lease bırakıldı. Bağımsız SQLite okumasında change/evidence sayıları sıfırdı. Değişikliği stage edip `verify --staged` çalıştırmak exit 1 / `UNREGISTERED_CHANGE` üretti; task terminal `COMPLETED` kaldı. Dolayısıyla task state, gate onayı değildir; bu deney verification bypass'ı göstermez. Ayrıntılar [Breaker B-04 ekinde](breaker.md).

Build, mevcut regresyon suite'i, release otomasyonu ve benchmark kontrolleri bu HEAD üzerinde çalışıyor. Bunların yeşil olması tamamlamanın doğru kanıtı zorunlu tuttuğunu ispatlamıyor: bağımsız audit, yeşil suite'in dışında karşı örnekler üretti. Nihai ürün kararı için [birleşik denetim raporu](report.md) esas alınmalıdır.
