# Changelog

## [Unreleased]

### Added

- `gobl init <domain>`: scaffolds a per-domain identity under
  `~/.config/gobl/<domain>/` (auto-generated keypair + a raw
  `party.json` template with a pre-filled `gobl:<domain>` endpoint).
- `gobl net who <address> --from <domain>`: performs an authenticated
  mutual party exchange — POSTs a signed request and returns the
  target's verified `org.Party` (full envelope, including any
  authority countersignatures present).
- `gobl net send <envelope> --to <domain> --from <domain>`:
  delivers a signed envelope to a remote `/inbox`.
- `gobl net serve`: HTTPS server with per-key `/.well-known/gobl/keys/<kid>`
  lookups, a bulk `/.well-known/jwks.json` endpoint for browser-based
  JOSE tooling (`jwt.io`-style verifiers), `/who` (authenticated mutual
  party exchange) and `/inbox` (signed envelope delivery). Open CORS
  (`Access-Control-Allow-Origin: *` plus OPTIONS preflight → 204) is
  enabled so JOSE tooling can fetch the JWKS from a browser context.
  Multi-tenant: auto-discovers every `<domain>/` directory under the
  config dir and routes by HTTP `Host`. ACME issues for every
  discovered domain. Optional per-domain `allow.json` gates `/who` and
  `/inbox` by signer address.
- `gobl sign --domain X [--to Y]`: signs with the key from
  `~/.config/gobl/<X>/` and stamps `iss=gobl:X` / `aud=gobl:Y` into
  the signed payload.
- `gobl verify`: gains `--address` / `--remote` flags for remote key
  discovery via the new GOBL Net per-key endpoint.
- Top-level `--json` flag: all operator-facing log output flows
  through `log/slog`. With the flag, structured JSON (one entry per
  line) replaces the default human-readable text. Logs go to
  **stderr**; result output (signed envelopes, `/who` party JSON,
  `version` JSON) stays on **stdout**.
- HTTP access logs on `gobl net serve`: structured `http_request`
  entries for every request plus handler-specific
  `keys.lookup`, `jwks.served`, `who.exchange` / `who.rejected`,
  `inbox.accepted` / `inbox.rejected`, `inbox.write_failed` events
  with high-signal fields (`caller`, `envelope`, `reason`, `status`,
  `duration_ms`). Startup messages (`generated keypair`,
  `initialised domain`, `GOBL Net listening`, `ACME enabled`,
  `Shutting down`) are also structured.
- CLI errors are emitted as a single `command failed` log entry with
  `key` / `message` / `faults` fields.
- On-disk layout for `gobl net serve`:
  `<config>/<domain>/{private.jwk, keys/<kid>.json, party.json,
  allow.json, inbox/}`. One file per `kid` (filename equals `kid`,
  validated at startup) — the model maps 1-to-1 to a future
  row-per-kid database. Rotation is filesystem ops.

### Changed

- `gobl net serve` `/inbox`: an envelope MUST now be signed with an
  `aud` equal to the inbox owner's address. Envelopes signed without
  an audience, or bound to a different audience, are rejected with
  `401 Unauthorized` (access log `inbox.rejected` carries
  `reason=aud_missing` or `reason=aud_mismatch`). This prevents a
  valid envelope from being replayed against multiple inboxes —
  signers must know the recipient at sign time. `gobl sign --domain
  X --to Y` already stamps `aud=gobl:Y` into the signed payload, so
  the operator workflow is unchanged; callers that previously sent
  audience-less envelopes to an inbox MUST start setting `--to`.
- `gobl keygen`: deprecated in favour of `gobl init <domain>`.
- `gobl net serve --keys` → `--keys-dir`. The on-disk layout for
  published keys is now `<domain>/keys/<kid>.json` (one file per
  `kid`) instead of a single `<domain>/keys.json` JWKS.
- The CLI now requires the post-GOBL-Net core
  (`github.com/invopop/gobl@net`): the signed payload is
  `{uuid, dig, iss, aud, iat}`, key IDs are UUIDv7, and the per-key
  endpoint replaces the old bulk `/keys` endpoint.

### Security

- `gobl net serve` `/inbox` handler re-parses the document UUID with
  `uuid.Parse` before writing the envelope to disk, as a
  defence-in-depth check against path traversal. UUIDs already pass
  `env.Validate()` + `uuid.HasTimestamp` + the strict 36-char
  `[0-9a-f-]` format check from `google/uuid`, but the re-parse keeps
  the filesystem write site self-contained.
