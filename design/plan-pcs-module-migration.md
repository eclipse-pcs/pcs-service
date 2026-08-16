# Plan: migrate pcs-service to the shared `github.com/eclipse-pcs/pcs` module

Read first (canonical, do not paraphrase into this repo):

- `~/go/src/pcs/design/footer-v1.md` — the particle footer v1 format spec
- `~/go/src/pcs/design/plan-module.md` — the module's API surface

Prerequisites: the module in `~/go/src/pcs` passes its tests, and
**pcs-demo has already migrated** (order: module → demo → service →
files-gateway → s3-gateway). pcs-demo's new on-disk files are the
compatibility target: files written by `pcs-split` must decode with
pcs-demo's `pcs-decode` and vice versa.

## Scope

- Replace `internal/pcs` and `internal/stream` with the module.
- Move the wire protocol and on-disk layout to footer v1 (no legacy reads;
  pre-production clean slate).
- Keep the service architecture (control channel + 7 ephemeral data ports)
  unchanged.

## Protocol change (v1 footers on the wire)

Key insight: on a TCP stream there is no stat/list metadata, so payload
lengths are *not* known upfront (unlike file/object storage). The footer
trails each stream, and the server/client separate payload from footer with a
**64-byte look-behind buffer per stream** at EOF. This is compatible with the
existing streaming decode: the only length-parity-dependent decision (last
byte of parity reconstruction) occurs at EOF, exactly when the footer has
arrived. Today's protocol already infers odd-length at EOF; the footer makes
that inference explicit and verified.

Concretely:

1. **Split mode**: data streams now carry **complete particle files**
   (payload + 64-byte footer). The server builds footers itself (one
   WriteID per session via `footer.NewWriteID()`, SHA-256 fingerprint
   shards, own/cross CRCs, length; mtime 0). Clients write streams to disk
   verbatim — no offline file assembly from trailer fields anymore.
2. **Merge mode**: clients send **complete particle files** as stored
   (payload + footer); no more footer-stripping client side. The server
   look-behinds 64 bytes per stream, parses footers, and verifies:
   own-CRCs, cross-CRCs, WriteID agreement, fingerprint vs SHA-256 of the
   reconstructed document. The merge profile mask (`EC=1 OC=2 EN=4 ON=8`)
   and the send-set rules are unchanged.
3. **Trailer JSON** (update `design/protocol.md` accordingly):
   - `secret_md5` → `secret_sha256` (hex)
   - `hash_suffix` (8-byte hex per kind) → `fingerprint_shard`
     (16-byte hex per kind)
   - `cross_crc` stays (values now cover payload only, per spec)
   - new: `length` (number), `write_id` (hex u64),
     `write_id_valid` (bool, merge), `footer` (base64 64-byte per kind,
     split — informational; streams already carry them)
   - `hash_valid` (merge) now means fingerprint verification result
4. The server no longer infers odd-length from relative stream lengths; the
   footer `length` field is authoritative (mismatch with received payload
   byte counts ⇒ error frame).

## Work items

1. **Wire the module**: add `use ./pcs-service` to `~/go/src/go.work`; add
   `github.com/eclipse-pcs/pcs` to `go.mod`.
2. **Delete superseded packages**:
   - `internal/pcs`: `pcs.go`, `encode.go`, `decode.go`, `hash.go`,
     `crc.go`, `decode_crc.go`, `particles.go`, `inventory.go`,
     `storage.go` and their tests → module `pcs` + `footer`.
   - `internal/stream` (whole package) → module `stream`.
3. **Keep and adapt service-specific pieces**:
   - `internal/pcs/merge_plan.go` (+ test): move to a service package (e.g.
     `internal/mergeplan`), adapt to module types. Send-set validation rules
     are unchanged. Inventory scanning loses the `.cp_`/`.np_` probing —
     presence only, unified suffixes.
   - `internal/pcs/particle_io.go`: adapt to `payload+64` layout via
     `footer.PayloadLen`.
4. **Server** (`internal/server`): swap to module `stream.Encoder` /
   `stream.Decoder`. Split: pass `EncodeOptions` (fresh WriteID), stream
   payloads+footers out. Merge: wrap each inbound stream in the 64-byte
   look-behind splitter (add a small helper here or in the module if generic
   enough: `SplitTrailingFooter(r io.Reader) (payload io.Reader, footer
   func() (*footer.Footer, error))`), feed payload readers into the decoder
   with `PayloadLen = -1` (unknown), verify at EOF.
5. **Clients** (`internal/client`, `cmd/pcs-split`, `cmd/pcs-merge`):
   - `pcs-split`: write streams verbatim into
     `storageA/<base>.ec|.on`, `storageB/<base>.oc|.en`,
     `storageC/<base>.cp|.np` (note `.e`→`.ec`, `.o`→`.oc`, no underscore
     variants).
   - `pcs-merge`: scan by presence, upload full file bytes, map missing
     particles to the profile mask as today.
6. **Update `design/protocol.md`** with the sections above (streams carry
   full particle files; trailer schema change; footer reference).
7. **Tests**:
   - Unit tests for merge-plan and look-behind splitter.
   - In-process TCP integration tests (`test/merge_send_test.go` etc.):
     regenerate expectations for the new layout.
   - Shell tests (`make test-shell`): regenerate all fixture data under
     `test/_data/` (golden, recovery, recovery_matrix, object, smoke,
     stdin) with the new format — the recovery matrix must keep covering
     every loss pattern × odd/even length. Fixtures can be produced with the
     migrated `pcs-split`.
   - **Cross-project round-trip** (acceptance): encode a file with pcs-demo's
     `pcs-encode`, merge it with `pcs-merge` (and the reverse:
     `pcs-split` output decoded by pcs-demo's `pcs-decode`). Bytes must
     match; WriteID/fingerprint verification must pass.

## Acceptance

- `go build ./...`, `go test ./...`, `make test-shell` green
- Cross-project round-trip with pcs-demo passes both directions
- Trailer JSON matches the updated `design/protocol.md`

## Out of scope

- Multi-session WriteID policies, healing — unchanged service semantics
- Reading legacy (12-byte tail) particle files
