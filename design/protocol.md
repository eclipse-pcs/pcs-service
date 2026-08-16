# pcs-service wire protocol

pcs-service exposes a TCP control channel and seven ephemeral data ports per session.
Particle semantics match [footer v1](https://github.com/eclipse-pcs/pcs) (shared
`github.com/eclipse-pcs/pcs` module) and [pcs-demo](https://github.com/dido/pcs-demo)
on-disk layout. Each particle stream carries a **complete particle file**
(payload + 64-byte footer). Clients write streams to disk verbatim — no offline
file assembly from trailer fields.

## Control channel

Default bind: `0.0.0.0:4567` (`--listen` / `PCS_LISTEN`).

### Handshake

1. Client connects and sends a line: `split` or `merge` (UTF-8, optional trailing newline).
2. For **merge** only, the client sends a second required line:

```text
profile <missing-mask-hex>\n
```

Missing-core mask bits (hex): `EC=1`, `OC=2`, `EN=4`, `ON=8`. Value `0` means all four core
streams will be sent. Any non-zero bit means the client closes that stream empty immediately;
the server reconstructs the missing core from the partner stream and parity (`CP` or `NP`) during
streaming decode. Logical object length and odd/even parity come from the footer `length` field
(parsed at each stream's trailing 64-byte footer), not from filename suffixes.

3. Server allocates seven ephemeral TCP listeners (`:0`) and replies with one line:

```text
<token> <mode> <docPort> <ec> <oc> <en> <on> <cp> <np>\n
```

Ports are decimal. The client must open seven TCP connections and prefix each with the session token.

### Token authentication

- **Particle ports** (`ec`, `oc`, `en`, `on`, `cp`, `np`): server reads bytes until the token is
  matched exactly, then treats further bytes as complete particle file data (merge mode).
- **Document port**: token is stripped by the reader; payload after the token is the plaintext
  document (split) or reconstructed output (merge).

## Split mode

| Stream | Direction | Content |
|--------|-----------|---------|
| docPort | client → server | token + plaintext |
| ec/oc/en/on/cp/np | server → client | payload + 64-byte footer v1 per particle |

The server generates one WriteID per session and stamps identical WriteID values into all six
footers. Clients save each stream verbatim to the unified paths:

| Kind | Suffix | Storage |
|------|--------|---------|
| even cypher | `.ec` | storageA |
| odd cypher | `.oc` | storageB |
| even noise | `.en` | storageB |
| odd noise | `.on` | storageA |
| cypher parity | `.cp` | storageC |
| noise parity | `.np` | storageC |

## Merge mode

Directions are reversed: client sends complete particle files; server emits reconstructed
document on docPort.

The client must send the merge profile line (see Handshake). File-based clients (e.g.
`pcs-merge`) map missing on-disk particles to the mask locally. The server separates each
inbound stream's trailing 64-byte footer (look-behind at EOF), verifies own-CRC, cross-CRC,
WriteID agreement, and fingerprint vs SHA-256 of the reconstructed document.

### Default send set (full merge)

When all four core particles are present (profile mask `0`), the client sends only **ec, oc, en, on**
and closes **cp** and **np** empty, even if parity files exist on disk. Parity streams are sent
only when at least one core in that pair is declared missing (recovery merge).

Invalid send sets (rejected before upload):

- Cypher parity without any cypher core (e.g. cp + en + on + np)
- Noise parity without any noise core (e.g. cp + ec + oc + np)
- Both cores missing in the same pair

## Session trailer

After all data streams finish successfully, the server sends on the **control channel**:

```text
<trailerLength>\n<trailerJSON>
```

### Session error

If split or merge fails, the server sends an error frame on the **control channel** instead of a trailer:

```text
error <message>\n
```

`trailerLength` is a decimal byte count. `trailerJSON` is UTF-8 JSON:

| Field | Type | Description |
|-------|------|-------------|
| `secret_sha256` | string | hex SHA-256 of original secret (split) or reconstructed secret (merge) |
| `fingerprint_shard` | object | per-kind hex (16 bytes): `ec`, `oc`, `en`, `on`, `cp`, `np` |
| `cross_crc` | object | per-kind uint32 LE CRC of partner **payload** (footer v1) |
| `length` | number | logical secret length in bytes (from footer) |
| `write_id` | string | hex u64 WriteID shared by all six particles |
| `write_id_valid` | bool | merge only: all parsed footers agreed on WriteID |
| `footer` | object | split only: base64-encoded 64-byte footer per kind (informational) |
| `bytes_processed` | number | plaintext bytes processed |
| `mode` | string | `split` or `merge` |
| `hash_valid` | bool | merge only: fingerprint verification result |
| `recoveries` | string[] | optional recovery notes |

See `~/go/src/pcs/design/footer-v1.md` for the canonical footer layout.

## Config

| Flag / env | Default | Purpose |
|------------|---------|---------|
| `--listen` / `PCS_LISTEN` | `0.0.0.0:4567` | control channel |
| `--token` / `PCS_TOKEN` | `SECRET_TOKEN` | shared secret |
| `--session-timeout` | `5m` | idle timeout during handshake only |
| `--max-object-size` | `0` (unlimited) | max document bytes |
| `--chunk-size` | 32KiB | streaming read size |
