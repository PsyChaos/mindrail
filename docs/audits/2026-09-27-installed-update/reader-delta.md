# Installed update and JEV distribution — Reader delta re-audit

Date: 2026-09-27
Base finding report: `reader.md`
Scope: yalnız RDR-001, RDR-002 ve düzeltmelerin dokunduğu REQ-008/REQ-009
contract yolları. Breaker sonuçları bu kararı oluşturmak için kullanılmadı.

## Karar

**APPROVE**

RDR-001 ve RDR-002 kapanmıştır. Affected REQ-008 ve REQ-009 artık **PASS**.
Yeni veya açık Reader finding üretilmedi.

## Finding kapanışları

| Finding | Delta kararı | Üretilmiş kanıt |
|---|---|---|
| RDR-001 — active Claude skill deleted adapter path'ini çağırıyor | CLOSED | `.claude/skills/engineering-orchestrator/SKILL.md:74-110` artık `mindrail agent route < request.json>` kullanıyor; exact `id`/`description` schema'sını, minimal non-sensitive payload'ı, env-only key injection'ı, atomic acceptance'ı ve bütün disabled/fallback/error branch'lerinde normal akışa dönüşü tarif ediyor. Active instruction taramasında eski invocation için 0 eşleşme bulundu. |
| RDR-002 — Graphify silinmiş adapter path'ini aktif gösteriyor | CLOSED | `graphify query "mindrail agent route embedded JEV adapter"` aktif `internal/agent/jev_route.py`, `internal/agent/test_jev_route.py` ve CLI agent testlerini döndürdü. `graphify explain "runAgentRoute"` node'u `internal/cli/agent.go:L57` ve gerçek çağrı edge'lerini gösterdi. Raw graph'ta eski `.claude/.../jev_route.py` source node sayısı 0; changeset'te 6 `graphify-out/**` artifact'ı güncellendi. |

## Discriminating probes

### RDR-001 — eski ve yeni invocation

Aktif instruction taraması:

```bash
rg -n "python \.claude/skills/engineering-orchestrator/scripts/jev_route\.py|\.claude/skills/engineering-orchestrator/scripts/jev_route\.py < request\.json" \
  .claude/skills AGENTS.md README.md docs/usage-tr.md internal/setup
```

Sonuç: **0 eşleşme**.

Yeni skill contract taraması `mindrail agent route`, password manager/keyring,
no-key stdin/Python/network engeli ve fail-open metinlerinin tamamını buldu.

Gerçek bridge kontrolleri:

| Girdi | Exit | Typed sonuç |
|---|---:|---|
| `TYPESAFE_API_KEY` unset, empty stdin | 0 | `enabled:false`, `status:"disabled"`, `reason:"api_key_missing"` |
| Dummy environment key, `{}` local input | 0 | `enabled:true`, `status:"fallback"`, `reason:"invalid_goal"` |

Bu iki kol skill'in güncel disabled/fail-open sözleşmesiyle aynıdır. Focused Go
bridge suite'i de PASS verdi:

```text
go test ./internal/agent ./internal/cli -run 'Test(Agent|FindTrustedPython)' -count=1
```

### RDR-002 — aktif ve stale graph düğümleri

Kontrol sorguları:

```text
graphify query "mindrail agent route embedded JEV adapter" --budget 5000
graphify explain "runAgentRoute" --budget 5000
```

Gözlenen aktif kaynaklar:

- `internal/agent/jev_route.py:L1` ve `route():L410`
- `internal/agent/test_jev_route.py`
- `internal/cli/agent_test.go`
- `internal/cli/agent.go:L57` içindeki `runAgentRoute`

Stale-node probe:

```text
jq '[.nodes[] | select((.source_file // .src // "") ==
  ".claude/skills/engineering-orchestrator/scripts/jev_route.py")] | length'
```

Sonuç: `0`.

## REQ-008 / REQ-009 delta matrisi

| Requirement | Karar | Delta kanıtı |
|---|---|---|
| REQ-008 — fresh/existing guidance tutarlılığı ve aktif dokümantasyon | PASS | Initial pass'teki managed AGENTS byte-equality ve setup tests korunuyor. Eksik kalan active Claude skill artık installed bridge, schema, data-minimization, env-only key ve fail-open contract'la aynı dili kullanıyor. Eski executable path yalnız historical audit kayıtlarında kalabilir; active instruction surface'te yok. |
| REQ-009 — test/gate completion contract | PASS | Focused agent tests PASS; `make install-test` PASS; clean rerun `make gate` on kategorinin tamamında PASS ve final satır `gate: all ten categories green`. Install boundary artık gate'in 10. kategorisi. |

## Install docs ve gate sayısı

- Makefile `install-test` target'ını sunuyor ve `gate` açıklaması “Run ten test
  categories” diyor.
- `scripts/gate.sh` kategorileri 1–10 olarak yürütüyor; onuncu kategori
  `install-boundary` → `scripts/test-install.sh`.
- Gerçek gate çıktısı şu sayıları verdi: unit 1443, domain 421, integration 12,
  race 9, knowledge-schema 216, MCP-contract 58, Git worktree 77, named
  worktree 8, SQLite concurrency 17, E2E 12, smoke 7; ardından install-boundary
  PASS ve `all ten categories green`.
- `make help`, `install`, `install-test` ve ten-category `gate` target'larını aynı
  adlarla gösterdi.
- README ve Türkçe kullanım kılavuzundaki verified-Linux install sınırları
  Makefile/test ile uyuşuyor: destination symlink/directory refusal, resolved
  `DESTDIR` containment ve regular-file same-directory atomic replacement.

`make install-test` ayrıca iki normal install, directory target refusal, final
symlink refusal, parent-symlink escape refusal, lexical traversal refusal,
`DESTDIR=/` boundary ve temp-residue yokluğunu gerçek filesystem objeleriyle
PASS etti.

## Test notu

İlk delta `make gate` denemesi ürün assertion'ından değil `/tmp` tmpfs'in dolu
olmasından (`no space left on device`) unit-link aşamasında durdu. Yalnız bu
Reader'ın önceki ve mevcut Go cache dizinleri temizlendikten sonra aynı komut
baştan çalıştırıldı ve on kategorinin tamamı geçti. Bu çevresel ilk koşu ürün
finding'i değildir; PASS iddiası clean rerun'a dayanır.

## Sonuç

İki Reader finding'i için de yokluk pozitif olarak üretildi: active instruction
artık çalışır bridge'i tarif ediyor ve generated graph yalnız aktif internal
adapter/bridge kaynaklarını gösteriyor. Install documentation, Makefile hedefleri
ve gerçek ten-category gate çıktısı tutarlı. Delta Reader sonucu: **APPROVE**.
