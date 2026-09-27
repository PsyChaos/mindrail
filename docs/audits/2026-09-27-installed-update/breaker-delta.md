# Installed update and JEV distribution — Breaker delta audit

## Sonuç

**REJECT**

Önceki iki ürün bulgusu kapanmıştır:

- `BRK-001`: **CLOSED** — normal ve tekrar kurulum çalışıyor; hedef dizin, hedef
  symlink, parent-symlink escape ve traversal reddediliyor; `DESTDIR=/` doğru
  nedenle reddediliyor; dış sentinel değişmiyor ve temp kalmıyor.
- `BRK-002`: **CLOSED** — daha önce yaşayan sekiz Go bridge mutasyonunun tamamı
  artık kalıcı ve ayırt edici bir testi kırıyor (`8 killed / 0 survived`).

Ancak delta kapsamındaki “category claims truthful” kontrolü yeni bir düşük önem
dereceli çelişki üretti:

- `BRK-003`: **OPEN / LOW** — `scripts/gate.sh` “Needs go and git only” diyor,
  fakat onuncu kategori `scripts/test-install.sh` üzerinden doğrudan `make`
  çalıştırıyor. `make` kullanılamaz yapıldığında aynı kategori exit `127` veriyor.

Bu nedenle kapanış doğrulaması başarılı olsa da mevcut snapshot için nihai Breaker
kararı `REJECT`tir.

## Kimlik, kapsam ve güvenlik sınırı

- Base: `64e8f8a533dc6bbf34b60edd8455f5d5120ddace`
- Donmuş gereksinimler:
  `docs/engineering/installed-update-and-jev-distribution-2026-09-27.md`
- Önceki Breaker kanıtı:
  `docs/audits/2026-09-27-installed-update/breaker.md`
- Delta scratch root: `/tmp/mindrail-breaker-delta-zg443c`
- Mod: önceki bağımsız Breaker'ın aynı rol ile delta yeniden denetimi; Reader
  sonucuna dayanılmadı.
- Üretim dosyalarının mutasyonları yalnız scratch kopyasında yapıldı. Sekiz mutasyon
  sonrası `internal/cli/agent.go` byte karşılaştırması exit `0` ile geri yüklendi.
- Gerçek key, session log veya gerçek repository state okunmadı/değiştirilmedi;
  yalnız sentineller ve geçici filesystem nesneleri kullanıldı.
- Bu tur, talimat gereği değişmeyen Python adapter mutasyonu ve old-repository
  upgrade deneylerini yeniden çalıştırmadı; aşağıda önceki üretilmiş rapora açıkça
  atıf yapılır.

## BRK-001 kapanış kanıtı — hostile install matrix

Gerçek recipe iki yoldan çalıştırıldı:

```sh
env GOCACHE=/tmp/mindrail-breaker-delta-zg443c/go-cache \
  GOTMPDIR=/tmp/mindrail-breaker-delta-zg443c/go-tmp \
  make install-test
```

çıktısı `install-test: PASS` ve exit `0` oldu. Ayrıca her kol ayrı scratch nesneleri
ile ölçüldü.

| Kol | Önceki bulgu exit | Güncel exit | Güncel hedef/dış etki | Temp kalıntısı |
| --- | ---: | ---: | ---: | ---: |
| Kontrol: absent destination | 0 | 0 | regular binary `1` | 0 |
| Kontrol: existing regular file | — | 0 | old/new SHA farklı `1` | 0 |
| Senaryo: destination directory | 0 | 2 | hedef içi child `0` | 0 |
| Senaryo: final destination symlink | denenmedi | 2 | dış sentinel SHA aynı `1` | 0 |
| Senaryo: parent symlink escape | 0 | 2 | dış yazı `0` | 0 |
| Senaryo: lexical traversal | denenmedi | 2 | dış yazı `0` | 0 |
| Round-2: `DESTDIR=/`, hedef bir dizin | denenmedi | 2 | shape-message hit `1`; escape-message hit `0` | 0 |

Matris sonundaki global `.mindrail.install.*` kalıntı sayısı `0` idi. Round-2
`DESTDIR=/` kolu özellikle ilk containment koşulunun `/` altında her absolute path'i
kabul ettiğini, reddin daha sonraki target-shape korumasından geldiğini ayırt eder.
Bu, yanlış nedenle yeşil kalan bir boundary testi değildir.

### En zayıf predicate ve over-refusal kontrolü

Yeni korumaların literal en zayıf girdileri çalıştırıldı:

- `[ -e "$dest" ] && [ ! -f "$dest" ]`: boş directory;
- `[ -L "$dest" ]`: tek final symlink;
- `"$root"/*`: parent symlink ile canonical root dışı resolved path;
- `realpath -m`: iki `..` segmentli traversal;
- `case "$root" in /)`: `/` kökü altında scratch absolute target.

Normal absent destination ve existing regular replacement ayrı ayrı exit `0`
verdi. Dolayısıyla düzeltme desteklenen iki çalışan konfigürasyonu reddetmedi;
unsafe şekillerin beşi exit `2` verdi.

### Class sweep

`mv -fT` ve `.mindrail.install.*` production recipe'si yalnız `Makefile` içindedir;
diğer eşleşmeler onun hostile-target testleridir. Aynı sınıfta ikinci bir production
install recipe üretilmedi.

## BRK-002 kapanış kanıtı — hedefli Go bridge mutasyonları

Kontrol:

```text
go test ./internal/cli -run TestAgent -count=1
exit 0; PASS
```

Her satırda yalnız belirtilen production guard etkisizleştirildi, aynı komut
çalıştırıldı ve temiz kaynak bir sonraki kola geri kondu.

| Guard mutasyonu | Kontrol exit | Mutant exit | Ayırt edici kalıcı test | Sonuç |
| --- | ---: | ---: | --- | --- |
| exact key output taramasını kaldır | 0 | 1 | `TestAgentRouteRejectsSecretInOtherwiseValidOutput` | killed |
| `status`/`enabled` consistency'yi kaldır | 0 | 1 | `TestAgentRouteRejectsInconsistentFallbackStatus` | killed |
| stdin `LimitReader`ı kaldır | 0 | 1 | `TestAgentRouteBoundsStdinBeforeLaunchingAdapter` | killed |
| raw output üst sınırını kaldır | 0 | 1 | `TestAgentRouteRejectsOversizedAdapterOutput` | killed |
| `boundedBuffer` remaining sınırını kaldır | 0 | 1 | `TestAgentRouteBoundedBufferRejectsOverflow` | killed |
| read-error erken fallback'ını kaldır | 0 | 1 | `TestAgentRouteReadErrorFailsOpenWithoutLaunching` | killed |
| record newline eklemesini kaldır | 0 | 1 | `TestAgentRouteAddsRecordTerminatingNewline` | killed |
| interpreter-race sınıflandırmasını kaldır | 0 | 1 | `TestAgentRouteClassifiesInterpreterRaceAsUnavailable` | killed |

Toplam değişim: önceki raporda `2 killed / 8 survived`; güncel delta'da önceki sekiz
survivor için **`8 killed / 0 survived`**. Her mutasyonda kaynak gerçekten değişti
(`cmp` exit `1`), tur sonunda temiz kopyaya gerçekten döndü (`cmp` exit `0`).

### Skill invocation ve deleted-file kontrolü

Üç aktif engineering-orchestrator skill konumu tarandı. Yalnız Claude skill'i JEV
komutu içeriyor ve güncel invocation şudur:

```text
mindrail agent route < request.json
```

Eski `.claude/skills/engineering-orchestrator/scripts/jev_route.py` için `test -e`
exit `1`; `.claude/skills`, `.agents/skills`, `.codex/skills`, `AGENTS.md` ve
`CLAUDE.md` altında bu silinmiş dosyayı çağıran eşleşme sayısı `0` idi. Graphify'nin
embedded `internal/agent/jev_route.py` kaynak kayıtları executable skill invocation
değildir.

### Class sweep

JEV'e ait secret/input/output guard'ları tek production reader olan
`internal/cli/agent.go` içindedir. `internal/validation/runner.go` içinde ayrı ve
önceden var olan aynı adlı bir `boundedBuffer` vardır; JEV bridge'i okumaz ve delta
mutasyon sınıfının kopyası değildir.

## BRK-003 — gate dependency iddiası yanlış

**Severity:** LOW
**Confidence:** CONFIRMED

`scripts/gate.sh:8` şu executable kullanım sözleşmesini taşıyor:

```text
Usage: ./scripts/gate.sh (or `make gate`). Needs go and git only.
```

Fakat kategori 10 şu zincirdir:

```text
run_step install-boundary ./scripts/test-install.sh
scripts/test-install.sh -> make -s -C "$repo" install ...
```

Kontrol/senaryo aynı snapshot ve aynı scratch data üzerinde, yalnız `make`
availability değiştirilerek ölçüldü:

| Kol | Exit | `install-test: PASS` hit | `make unavailable` hit |
| --- | ---: | ---: | ---: |
| Kontrol: gerçek toolchain | 0 | 1 | 0 |
| Senaryo: `make` unavailable | 127 | 0 | 1 |

Bu runtime kusuru değil, doğrudan çalıştırılabilir gate'in dependency contract
kusurudur. En küçük düzeltme yorumu gerçek bağımlılıkla uyumlu hale getirmektir;
`scripts/test-install.sh` ayrıca kendi Linux/coreutils niyetini zaten açıkça belirtir.

### Class sweep

“Needs go and git only” eşleşmesi audit dokümanları ve graph çıktısı hariç repository'de
`1` adettir. Aynı yanlış claim başka aktif scriptte bulunmadı.

## Gate ve mekanik kural kanıtı

Gerçek delta scratch'ında tam gate çalıştırıldı:

```text
unit 1443
domain 421
integration 12
race 9
knowledge-schema 216
mcp-contract 58
git-worktree 77
git-worktree-named 8
sqlite-concurrency 17
end-to-end 12
end-to-end-smoke 7
install-boundary passed
gate: all ten categories green
```

Toplam Go PASS sayısı `2,280`; numbered category comment sayısı `10`;
`run_step install-boundary ./scripts/test-install.sh` occurrence sayısı `1`.
`Makefile` ayrıca `check: fmt-check vet test install-test` içerir. Scratch'ta gerçek
`make check` exit `0` verdi ve sonunda `install-test: PASS` görüldü. Dolayısıyla
BRK-001 ve BRK-002'nin mekanik regresyon kuralları hem yerel check hem full gate
tarafında erişilebilirdir. Yanlış olan tek kategori iddiası BRK-003'teki dependency
cümlesidir; on kategori ve yeşil sayım iddiaları gerçektir.

## Altı Breaker hareketinin delta özeti

1. **B1 Mutation:** Önceki sekiz survivor'ın tümü tekrar mutasyona uğratıldı;
   `8/8` killed. Değişmeyen Python adapter mutation sonucu önceki rapordan
   `82 killed / 0 survived` olarak referanslandı, yeniden çalıştırılmadı.
2. **B2 Deleted behavior A/B:** Delta bu kodu değiştirmedi. Önceki rapordaki base
   adapter ↔ embedded adapter SHA/cmp ve `45/45` offline A/B kanıtı referanslandı.
3. **B3 Threat model:** destination directory/symlink, parent escape, traversal,
   root boundary, outside hash ve temp residue canlı oynandı. Deleted skill path
   invocation tarandı. Key/log/argv/body ve hostile provider output deneyleri delta
   dışı değişmediği için önceki rapordaki sentinel sonuçları referanslandı.
4. **B4 Upgrade path:** Update/state kodunda bu delta ile yeni değişiklik yok.
   Talimat gereği tekrar edilmedi; önceki rapordaki base-init → new-update → first
   run/repeat run, task/data/hook/managed-block koruma matrisi referanslandı.
5. **B5 Weakest satisfying input:** boş final directory, tek symlink, iki-segment
   traversal ve root `/` discriminating shape kolu çalıştırıldı; sonuçlar yukarıdaki
   sayısal matriste.
6. **B6 Dual readers:** `make install-test`, `make check` ve category-10 gate aynı
   hostile matrix için PASS verdi. Üç skill kopyasının deleted-path cevabı birlikte
   tarandı; eski dosyayı çağıran `0` reader bulundu.

## Cost

- Her install-boundary koşusu iki başarılı `go build` yapar: absent control ve
  regular-file repeat. Beş unsafe kol build'den önce reddedilir.
- Tam gate `2,280` Go test PASS'i ve bir install-boundary category çalıştırdı.
- JEV route sınırları delta ile değişmedi: tek provider attempt, `10 s` deadline,
  `65,536` byte kabul edilen request, `262,144` byte provider response ve
  `1,048,576` byte Go stdout ceiling. Ayrıntılı çarpım önceki Breaker raporundadır.

Yeni bir kaynak tüketimi bulgusu üretilmedi.

## Backward compatibility

Desteklenen normal kurulum şekilleri ölçüldü: absent destination ve existing regular
binary ikisi de exit `0`; ikinci kurulum eski SHA'yı değiştirdi ve temp bırakmadı.
Unsafe final symlink/directory artık bilinçli olarak reddedilir. Existing repository
state, task/data/config/knowledge/hook ve managed-block uyumluluğu delta ile
değişmedi; önceki rapordaki gerçek old-initialized repo matrisi bu turda talimat
gereği yeniden çalıştırılmadı.

## Açık madde ve nihai karar

BRK-001 ve BRK-002 için açık madde kalmadı. Önceki rapordaki gerçek TypeSafe endpoint
deneyi bu delta ile ilgili değildir ve gerçek credential olmadan tekrar denenmedi.

`scripts/gate.sh` dependency cümlesi kategori 10'un gerçek çalıştırma zinciriyle
uyumlu hale gelene ve aynı iki-kollu probe yeniden geçene kadar **BRK-003 açık** ve
nihai Breaker kararı **REJECT** kalır.
