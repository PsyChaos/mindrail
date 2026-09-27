# Installed update and JEV distribution — Breaker delta audit, round 3

## Sonuç

**APPROVE**

Bu tur yalnız `breaker-delta.md` içindeki açık `BRK-003` bulgusunu yeniden sınadı.
Prerequisite claim artık gerçek gate zincirini doğru adlandırıyor; stale kardeş claim
üretilmedi ve shell syntax geçerli. Önceki `BRK-001` ve `BRK-002` kapanışları
değişmedi. Açık Breaker bulgusu kalmadı.

## Kapsam ve güvenlik sınırı

- Base: `64e8f8a533dc6bbf34b60edd8455f5d5120ddace`
- Önceki kapanış kanıtı:
  `docs/audits/2026-09-27-installed-update/breaker-delta.md`
- Bu tur production davranışını değiştirmedi ve source mutasyonu yapmadı.
- Talimat gereği hostile install matrisi ile sekiz Go bridge mutasyonu yeniden
  çalıştırılmadı; önceki rapordaki `BRK-001: CLOSED` ve `BRK-002: CLOSED` sonuçları
  referanslandı.
- Gerçek key, session log veya runtime state okunmadı/değiştirilmedi.

## BRK-003 kapanış kanıtı

Güncel claim:

```text
Usage: ./scripts/gate.sh (or `make gate`). Needs Go, Git, make, and
Linux/coreutils (including realpath, sha256sum, and mv -T).
```

Gerçek çağrı zinciri:

```text
scripts/gate.sh: run_step install-boundary ./scripts/test-install.sh
scripts/test-install.sh: make -s -C "$repo" install ...
Makefile install: realpath ...; sha256sum testlerde; mv -fT ...
```

### Kontrol/senaryo sayıları

| Ölçüm | Kontrol | Senaryo/önceki hata | Güncel sonuç |
| --- | ---: | ---: | ---: |
| Claim'de beklenen token | 7 | — | 7/7 present |
| Somut executable availability (`go`, `git`, `make`, `realpath`, `sha256sum`, `mv`) | 6/6 | — | 6/6 |
| Normal `make --version` exit | 0 | — | 0 |
| `make` unavailable exit | — | 127 | 127 |
| Unavailable senaryosunda claim'in `make` hit'i | — | eski claim: 0 | 1 |
| Stale “Go and Git only” aktif claim | — | önceki: 1 | 0 |
| Güncel prerequisite claim occurrence | — | — | 1 |

Yedi claim token'ı `Go`, `Git`, `make`, `Linux/coreutils`, `realpath`,
`sha256sum` ve `mv -T` idi; her birinde fixed-string probe exit `0` verdi.
`make` PATH başında unavailable wrapper ile değiştirildiğinde aynı executable probe
exit `127` ve `make unavailable` hit `1` verdi. Claim bu eksik dependency'yi hit `1`
ile artık önceden bildiriyor; önceki rapordaki kontrat çelişkisi üretilemedi.

### Actual-command ve sibling sweep

| Probe | Sayı |
| --- | ---: |
| `run_step install-boundary ./scripts/test-install.sh` | 1 |
| `scripts/test-install.sh` içindeki `make -s -C` | 1 |
| Gate içindeki `go test` metin/call noktaları | 3 |
| Makefile install içindeki `realpath -m/-e` | 4 |
| Install testindeki `sha256sum` | 6 |
| Makefile install içindeki `mv -fT` | 1 |
| Repository Go kodundaki Git execution/lookup site'ları | 64 |
| Audit ve graph çıktısı hariç stale “Needs go and git only” | 0 |

Linux/coreutils gereksinimi ayrıca `scripts/test-install.sh` başlığında aynı niyetle
belirtilir. Aktif source, script, Makefile, README ve docs taramasında eski dar claim'in
ikinci bir kopyası bulunmadı.

## Syntax ve mekanik sonuç

```text
sh -n scripts/gate.sh              exit 0
sh -n scripts/test-install.sh      exit 0
git diff --check -- scripts/gate.sh exit 0
```

Değişiklik yalnız prerequisite açıklamasını gerçek bağımlılıklarla hizaladı;
executable davranış değişmedi. Bu nedenle önceki turda zaten gerçek `make gate` ile
ölçülen `install-boundary: passed` ve `all ten categories green` davranışı yeniden
çalıştırılmadı.

## Önceki bulguların durumu

| Bulgu | Round-2 durumu | Round-3 durumu |
| --- | --- | --- |
| `BRK-001` hostile install boundary | CLOSED | CLOSED; davranış değişmedi |
| `BRK-002` sekiz Go bridge survivor | CLOSED (`8 killed / 0 survived`) | CLOSED; davranış değişmedi |
| `BRK-003` yanlış gate dependency claim | OPEN / LOW | **CLOSED** |

## Nihai Breaker kararı

`BRK-003` kontrol/senaryo farkı artık dokümante edilen prerequisite ile uyumludur,
stale sibling claim yoktur ve script syntax geçerlidir. Önceki iki bulgu da kapalıdır.
Nihai Breaker kararı: **APPROVE**.
