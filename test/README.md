# pcs-service shell integration tests

Black-box tests for the TCP split/merge server and CLI clients, following the layout used in [pcs2/test](https://github.com/dido/pcs2/tree/main/test).

## Prerequisites

- Go 1.26.3+ (see repo `go.mod`)
- `nc` (netcat) for port readiness checks
- Optional: `lsof` for stale process cleanup

## Quick start

```bash
cd test
chmod +x *.sh
./setup.sh
./compare.sh test smoke
```

Full gate (all scenarios):

```bash
./compare_all.sh
```

From the repo root:

```bash
make test-shell
```

## Layout

| Script | Purpose |
|--------|---------|
| `setup.sh` | Create `_data/`, `_logs/`; ensure repo `bin/` exists |
| `compare_env.sh` | Default ports, token, paths (override via `compare_env.local.sh`) |
| `compare_common.sh` | Build binaries into repo `bin/`, start/stop `pcs-service`, helpers |
| `compare.sh` | Run one test: `smoke`, `object`, `golden`, `recovery`, `stdin` |
| `compare_recovery.sh` | Parity recovery matrix: 16 cases (small/large × even/odd × missing ec/oc/en/on) |
| `compare_all.sh` | Run all scenarios sequentially (includes recovery matrix) |

Default control port: **14567** (`PCS_SERVICE_PORT`). Override in `compare_env.local.sh` (gitignored).

## Shell test scenarios

| Test | What it checks |
|------|----------------|
| `smoke` | `pcs-split` → six particle files on disk → `pcs-merge` round-trip (merge sends four core streams) |
| `object` | Odd-length secret (17 bytes) round-trip |
| `golden` | Split + `go test` golden decode via `pcs.DecodeFromStorage` |
| `recovery` | Delete `storageA/*.e`, merge still reconstructs original (streaming + profile handshake) |
| `stdin` | `cat` pipe into `pcs-split -f -` (512 KiB streaming round-trip) |

## Recovery matrix (`compare_recovery.sh`)

Parametrized parity recovery tests: split → delete one core particle → merge → `cmp` original vs reconstructed.

```bash
./compare_recovery.sh list
./compare_recovery.sh test small_odd_ec
./compare_recovery.sh test all    # 16 cases; large cases may take 1–2 minutes
```

| Dimension | Values |
|-----------|--------|
| Size | small (32/33 B), large (512 KiB / 512 KiB + 1 B) |
| Length parity | even / odd |
| Missing core | `ec`, `oc`, `en`, `on` (one per case) |

## Go unit / integration tests

```bash
go test ./...
go test ./internal/stream/...   # streaming vs buffer PCS golden (in-memory reference)
go test ./test/...              # in-process TCP integration
go test ./test/... -run 'MergeSend|MergeFromFiles' -v   # merge send-set (4 cores vs recovery)
```

| Go test file | Focus |
|--------------|-------|
| `integration_test.go` | Split/merge round-trip, size limits |
| `recovery_test.go` | TCP parity recovery (in-memory particles) |
| `merge_send_test.go` | File-based merge with varied particle subsets |
| `golden_test.go` | On-disk particle layout vs `pcs.DecodeFromStorage` |

## Environment variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `PCS_SERVICE_PORT` | `14567` | Control channel port |
| `PCS_SERVICE_TOKEN` | `TEST_TOKEN` | Session token |
| `PCS_SERVICE_HOST` | `127.0.0.1` | Server host |
| `BIN_DIR` | `<repo>/bin` | Override compiled CLI binaries location |
