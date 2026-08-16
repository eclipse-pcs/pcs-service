# pcs-service

Go implementation of the PCS (particle cipher split) network service. Clients stream documents to the server for encoding into six particle streams, or send particles back for reconstruction. On-disk layout matches [pcs-demo](https://github.com/dido/pcs-demo).

## Build

Binaries are written to `bin/` (gitignored). From the repo root:

```bash
make build
```

Or manually:

```bash
mkdir -p bin
go build -o bin/pcs-service ./cmd/pcs-service
go build -o bin/pcs-split   ./cmd/pcs-split
go build -o bin/pcs-merge   ./cmd/pcs-merge
```

## Quick start

**Terminal 1 — start the server**

```bash
./bin/pcs-service --listen=127.0.0.1:4567
```

Startup log reports `max-object-size=unlimited` by default.

**Terminal 2 — split a file**

```bash
./bin/pcs-split \
  -host=127.0.0.1 \
  -port=4567 \
  -f /path/to/document.bin \
  -o ./output
```

Particle files are written under `./output/storageA`, `storageB`, and `storageC`.

**Merge back**

```bash
./bin/pcs-merge \
  -host=127.0.0.1 \
  -port=4567 \
  -f document.bin \
  -dir ./output \
  -o ./output/document_reconstructed.bin
```

**Verify**

```bash
cmp /path/to/document.bin ./output/document_reconstructed.bin
```

## Operational notes

### Large files

The default path streams data in 32 KiB chunks; memory use stays O(chunk size), not O(file size).

- Default `--max-object-size` is **0 (unlimited)**. To enforce a cap:

  ```bash
  ./bin/pcs-service --max-object-size=67108864   # 64 MiB
  ```

- **Stdin / pipe** (same streaming path):

  ```bash
  cat /path/to/large.iso | ./bin/pcs-split -f - -name large.iso -o ./output
  ```

  `-name` is required when `-f -` because there is no filename to derive a particle basename from.

### Token

Server and clients must use the same session token (default `SECRET_TOKEN`).

```bash
./bin/pcs-service --token=my-secret
./bin/pcs-split -token=my-secret ...
./bin/pcs-merge -token=my-secret ...
```

Environment variables for the server: `PCS_LISTEN`, `PCS_TOKEN`.

### Ports

| Port | Role |
|------|------|
| **4567** (default) | Control channel — handshake, trailer, or error |
| **Ephemeral (7 per session)** | One document port + six particle ports; assigned per session in the invitation line |

Clients only dial the control port explicitly. The server reply lists the seven data port numbers for that session.

Bind address default is `0.0.0.0:4567` (all interfaces). For local use only:

```bash
./bin/pcs-service --listen=127.0.0.1:4567
```

### Errors

If a session fails (size limit, encode/decode error, etc.), the server sends an error frame on the control channel:

```text
error <message>
```

Clients surface this via `ReadTrailer` as a `SessionError` instead of a vague socket/`sendfile` failure. See [design/protocol.md](design/protocol.md).

### Other server flags

| Flag | Default | Purpose |
|------|---------|---------|
| `--chunk-size` | 32 KiB | Streaming read/write chunk size |
| `--session-timeout` | 5m | Handshake idle timeout (cleared before data transfer) |

### Merge

`pcs-merge` sends **four core particle streams** (ec, oc, en, on) by default. Parity (cp/np) is uploaded only when a core in that pair is missing (recovery). See [design/protocol.md](design/protocol.md#default-send-set-full-merge).

## Tests

```bash
go test ./...          # unit + in-process TCP integration
make test-shell        # black-box shell scenarios (split/merge round-trips)
```

See [test/README.md](test/README.md) for shell test layout and ports (default **14567** in tests, not 4567).

## Protocol

Wire format, trailer JSON, and config reference: [design/protocol.md](design/protocol.md).
