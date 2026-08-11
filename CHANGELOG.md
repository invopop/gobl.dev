# Changelog

## [Unreleased]

### Added

- `gobl init <domain>`: scaffolds a per-domain identity under
  `~/.config/gobl/<domain>/` (auto-generated keypair + a raw
  `party.json` template with a pre-filled `gobl:<domain>` endpoint).
- `gobl net who <address> --from <domain>`: performs an authenticated
  identity lookup — GETs the target's `/who` with a bearer request
  token minted from the `--from` identity and returns the target's
  verified `org.Party` (full envelope, including any authority
  countersignatures present). A `202` (deferred disclosure) is
  recorded under `who-pending/` so the inbox accepts the party the
  target may deliver later.
- `gobl net send <envelope> --to <domain> --from <domain>`:
  delivers a signed envelope to a remote `/inbox` with a request
  token minted from the `--from` identity, which may differ from the
  envelope's signer (trusted-intermediary transmission).
- `gobl net requests --domain <domain>` / `gobl net approve
  <requester> --domain <domain>`: list and approve deferred `/who`
  requests — approval signs the domain's party for the requester
  (`aud=requester`) and delivers it to the requester's inbox.
- `gobl net serve`: HTTPS server with per-key `/.well-known/gobl/keys/<kid>`
  lookups, a bulk `/.well-known/jwks.json` endpoint for browser-based
  JOSE tooling (`jwt.io`-style verifiers), `/who` (authenticated
  identity lookup) and `/inbox` (signed envelope delivery). `/who`
  and `/inbox` require a bearer request token (spec §5.5) and reject
  requests without one with `401`; key endpoints stay open. The
  static `/who` response is self-signed once at startup and served
  with `Cache-Control: private`; a missing `party.json` makes the
  domain receive-only (`204`). Deferred disclosure via the
  `who-deferred` marker answers `202` and records requests for
  approval. Sender endorsement is always enforced on the inbox
  (`403` `not_endorsed`): senders must be endorsed by a trusted
  authority — `lookup.gobl.org` by default, `--authority` adds more —
  with a confirmed verifier, unless `--allow-unverified` relaxes the
  verifier requirement for sandboxes; party envelopes answering a
  pending `/who` request are exempt. The manual single-identity mode
  (`--party`/`--keys-dir`/`--private-key`/`--inbox`/`--who-deferred`)
  and the `--insecure` client flags are removed: domains come from the
  config dir, requests are always authenticated, and clients always
  dial `https://<address>`. Open CORS
  (`Access-Control-Allow-Origin: *`, including the `Authorization`
  header, plus OPTIONS preflight → 204) is enabled so JOSE tooling
  can fetch the JWKS from a browser context. Multi-tenant:
  auto-discovers every `<domain>/` directory under the config dir and
  routes by HTTP `Host`. ACME issues for every discovered domain.
- `gobl sign --domain X [--to Y]`: signs with the key from
  `~/.config/gobl/<X>/` and stamps `iss=X` / `aud=Y` into the signed
  payload — signed claims carry bare GOBL Net addresses (FQDNs); the
  `gobl:` scheme remains only on endpoint URIs and the unsigned
  header `from`/`to`.
- `gobl verify`: gains `--address` / `--remote` flags for remote key
  discovery via the new GOBL Net per-key endpoint.
- Top-level `--json` flag: all operator-facing log output flows
  through `log/slog`. With the flag, structured JSON (one entry per
  line) replaces the default human-readable text. Logs go to
  **stderr**; result output (signed envelopes, `/who` party JSON,
  `version` JSON) stays on **stdout**.
- HTTP access logs on `gobl net serve`: structured `http_request`
  entries for every request plus handler-specific
  `keys.lookup`, `jwks.served`, `auth.rejected` (`token_missing` /
  `token_invalid` / `token_expired` / `token_unavailable`),
  `who.served` / `who.deferred` /
  `who.approved` / `who.fulfilled`, `inbox.accepted` /
  `inbox.rejected` (incl. `not_endorsed` and `verify_unavailable`),
  `inbox.write_failed`
  events with high-signal fields (`requester`, `caller`, `envelope`,
  `reason`, `status`, `duration_ms`). The authenticated entries
  double as a request audit log. Startup messages (`generated
  keypair`, `initialised domain`, `GOBL Net listening`, `ACME
  enabled`, `Shutting down`) are also structured.
- CLI errors are emitted as a single `command failed` log entry with
  `key` / `message` / `faults` fields.
- On-disk layout for `gobl net serve`:
  `<config>/<domain>/{private.jwk, keys/<kid>.json, party.json,
  who-deferred, who-requests/, who-pending/, inbox/}`. One file per
  `kid` (filename equals `kid`, validated at startup) — the model
  maps 1-to-1 to a future row-per-kid database. Rotation is
  filesystem ops.

### Changed

- `gobl net serve`: transient verification failures — the requester's
  or sender's key/who endpoint unreachable — now answer
  `503 Service Unavailable` (log reasons `token_unavailable` /
  `verify_unavailable`) instead of `401`/`403`, so clients retry
  rather than treating the rejection as final.

- `gobl net serve` `/inbox`: an envelope MUST now be signed with an
  `aud` equal to the inbox owner's address. Envelopes signed without
  an audience, or bound to a different audience, are rejected with
  `401 Unauthorized` (access log `inbox.rejected` carries
  `reason=aud_missing` or `reason=aud_mismatch`). This prevents a
  valid envelope from being replayed against multiple inboxes —
  signers must know the recipient at sign time. `gobl sign --domain
  X --to Y` already stamps `aud=Y` into the signed payload, so
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
