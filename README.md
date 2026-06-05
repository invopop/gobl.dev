# gobl.dev

The home for [GOBL](https://github.com/invopop/gobl) (Go Business Language)
tooling: a command-line interface and a web/API server, built on the core GOBL
library with the full addon set bundled in.

Released under the Apache 2.0 [LICENSE](https://github.com/invopop/gobl.dev/blob/main/LICENSE), Copyright 2026 [Invopop S.L.](https://invopop.com).

[![Lint](https://github.com/invopop/gobl.dev/actions/workflows/lint.yaml/badge.svg)](https://github.com/invopop/gobl.dev/actions/workflows/lint.yaml)
[![Test Go](https://github.com/invopop/gobl.dev/actions/workflows/test.yaml/badge.svg)](https://github.com/invopop/gobl.dev/actions/workflows/test.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/invopop/gobl.dev)](https://goreportcard.com/report/github.com/invopop/gobl.dev)
[![codecov](https://codecov.io/gh/invopop/gobl.dev/graph/badge.svg)](https://codecov.io/gh/invopop/gobl.dev)
[![GoDoc](https://godoc.org/github.com/invopop/gobl.dev?status.svg)](https://godoc.org/github.com/invopop/gobl.dev)
![Latest Tag](https://img.shields.io/github/v/tag/invopop/gobl.dev)
[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/invopop/gobl.dev)

Core GOBL is a pure document library. This project composes it with every
GOBL addon (see [`bundle/`](./bundle)) and ships two binaries:

| Binary     | Package          | What it is |
|------------|------------------|------------|
| **`gobl`** | `cmd/gobl`       | Command-line tool for building, validating, signing and correcting GOBL documents, plus an HTTP API server and an MCP server. |
| **`gobl.dev`** | `cmd/gobl.dev` | Web server: the GOBL HTTP API plus a browser-based document editor. Powers the public instance at [gobl.dev](https://gobl.dev). |

Both binaries register the same complete addon set, so they support every
document type GOBL knows about.

## `gobl` — the CLI

Install:

```bash
go install github.com/invopop/gobl.dev/cmd/gobl@latest
```

Commands:

| Command | Description |
|---------|-------------|
| `gobl build` | Parse, calculate, and validate a document (YAML or JSON), wrapping it in an envelope. Supports `--set` / `--set-file` overrides and `-i` indentation. |
| `gobl validate` | Validate an existing document or envelope. |
| `gobl correct` | Generate a corrective document (credit/debit note) for an invoice. |
| `gobl sign` | Sign an envelope with a JWK private key. |
| `gobl verify` | Verify an envelope's signatures. |
| `gobl replicate` | Clone a document with a fresh UUID. |
| `gobl keygen` | Generate an ES256 key pair. *(Deprecated: prefer `gobl init`.)* |
| `gobl init` | Scaffold a GOBL Net domain identity under `~/.config/gobl/<domain>/` (keypair + party template). See [GOBL Net](#gobl-net). |
| `gobl net who` | Authenticated mutual party exchange with a remote GOBL Net address. |
| `gobl net send` | POST a signed envelope to a remote `/inbox`. |
| `gobl net serve` | Run the GOBL Net HTTPS server (keys + `/who` + `/inbox` + bulk JWKS). |
| `gobl serve` | Launch the HTTP API server (see [API](#http-api)). |
| `gobl mcp` | Launch a [Model Context Protocol](https://modelcontextprotocol.io) server over stdio for AI tools and editors. |
| `gobl version` | Print the version. |

Examples:

```bash
# Calculate and validate a YAML invoice, indented
gobl build -i ./invoice.yaml

# Build, overriding values from another file and the command line
gobl build -i ./invoice.yaml --set-file customer=./party.yaml --set series=TEST

# Correct an invoice with a credit note
gobl correct -i ./invoice.json --credit

# Serve the HTTP API on :8080
gobl serve
```

## `gobl.dev` — the web server

Serves the GOBL HTTP API alongside a browser-based JSON editor (built with
[PopUI](https://github.com/invopop/popui.go) and [Templ](https://templ.guide)).

```bash
go run ./cmd/gobl.dev          # listens on :8080
PORT=3000 go run ./cmd/gobl.dev
```

Open [http://localhost:8080](http://localhost:8080) for the editor; the API is
served under `/v0` (see below).

### Docker

```bash
docker build -t gobl.dev .
docker run -p 8080:8080 gobl.dev
```

### Deployment

Deployed to [Fly.io](https://fly.io) via the `deploy.yaml` workflow on push to
`main`; the app name is set in `fly.toml`.

## HTTP API

The same API is exposed by both `gobl serve` and the `gobl.dev` server, under
the `/v0` prefix:

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v0/build` | Parse, calculate, and validate a GOBL document |
| `POST` | `/v0/validate` | Validate an existing document or envelope |
| `POST` | `/v0/sign` | Sign an envelope with a JWK private key |
| `POST` | `/v0/verify` | Verify an envelope's signatures |
| `POST` | `/v0/correct` | Generate a corrective document (credit/debit note) |
| `POST` | `/v0/replicate` | Clone a document with a new UUID |
| `POST` | `/v0/keygen` | Generate an ES256 key pair |
| `GET` | `/v0/schemas` | List registered JSON schemas |
| `GET` | `/v0/schemas/{path}` | Fetch a specific schema (add `?bundle` for bundled) |
| `GET` | `/v0/regimes` | List available tax regimes |
| `GET` | `/v0/regimes/{code}` | Fetch a specific tax regime definition |
| `GET` | `/v0/addons` | List available addons |
| `GET` | `/v0/addons/{key}` | Fetch a specific addon definition |
| `POST` | `/v0/mcp` | Model Context Protocol (MCP) endpoint |
| `GET` | `/v0/openapi.json` | OpenAPI specification |

To embed the handler in your own server, import [`gobl.dev/api`](./api)
(`api.NewHandler(...)`) and blank-import [`gobl.dev/bundle`](./bundle) to register
the addons.

## WebAssembly

[`wasm/`](./wasm) compiles GOBL to WebAssembly so it can run in the browser, and
publishes the `gobl-worker` npm package (a web-worker wrapper). The release
workflow attaches the wasm build to each GitHub Release, uploads it to
`cdn.gobl.org`, and publishes the npm package.

## Addons

[`bundle/bundle.go`](./bundle/bundle.go) is the single place that declares which
addons the binaries ship with — one blank import per addon module. Add an
approved addon there and both `gobl` and `gobl.dev` pick it up.

## GOBL Net

> ⚠️ **EXPERIMENTAL** — GOBL Net is under active development. The CLI
> commands, on-disk layout, and the wire protocol may change without notice.

GOBL Net is a decentralised identity-and-discovery protocol for signed GOBL
documents: a signer's identity is an FQDN (e.g. `billing.invopop.com`), and
verifying keys, an endorsed identity, and a delivery inbox are all served
from well-known HTTPS endpoints at that domain. The protocol itself lives in
the core library at
[`github.com/invopop/gobl/net`](https://github.com/invopop/gobl/blob/net/net/README.md) —
that file is the authoritative spec for addresses, the signed `iss`/`aud`/`iat`
payload, the per-key and JWKS endpoints, `/who`, and `/inbox`. This section
covers only the CLI / server side.

### `gobl init <domain>`

Scaffolds a per-domain identity under `~/.config/gobl/<domain>/`:

```
~/.config/gobl/billing.invopop.com/
├── private.jwk                              ← active signing key (0600)
├── keys/<kid>.json                          ← published JWK (stamped valid_from=now)
├── party.json                               ← party template with a pre-filled gobl: endpoint
├── allow.json                               ← optional: gates /who and /inbox by signer
└── inbox/                                   ← envelopes received over /inbox land here
```

Flags: `--config-dir`, `--force`, `--name`. Rotation is just filesystem ops:
drop a new `<kid>.json` to publish a key, set `valid_until` on a file to
retire it, `rm` to remove it (future requests for that `kid` return `404`).

### `gobl sign --domain X [--to Y]`

Signs with the key from `~/.config/gobl/<X>/` and stamps `iss=gobl:X` /
`aud=gobl:Y` into the signed payload (alongside `uuid`, `dig`, and `iat`).

### `gobl verify`

Two flags activate remote verification:

- `-a, --address <fqdn>` — require the verified `iss` to equal this address.
- `-r, --remote` — fetch the verifying key from the issuer published in the
  signed `iss`, via `<iss>/.well-known/gobl/keys/<kid>`.

### `gobl net who <address> --from <domain>`

Authenticated mutual party exchange: POSTs a signed request (`iss=gobl:from`,
`aud=gobl:address`) and prints the target's verified `org.Party` envelope —
including any authority countersignatures the target serves alongside its
self-signature.

### `gobl net send <envelope> --to <fqdn>`

Reads a signed envelope from a file (or stdin), POSTs it to the destination's
`/inbox`. Exits 0 on `202 Accepted`; otherwise `ErrInboxRejected`.

The envelope's signed `aud` MUST equal `--to`: receiving inboxes reject
envelopes signed without an audience or bound to a different one (replay
protection). `gobl sign --domain X --to Y` stamps `aud=gobl:Y` for you.

- `--insecure` — use `http://` and permit `host:port` form in `--to`
  (development only).

### `gobl net serve`

Runs the HTTPS server. Always listens on an HTTP port (default 80); when a
TLS source is configured it also listens on the HTTPS port (default 443),
serving identical content — no redirect, senders choose the scheme.

**Multi-tenant.** Auto-discovers every `<config-dir>/<domain>/` directory and
routes by HTTP `Host`. `--domain` restricts to one; `--party` + `--keys-dir`
selects a single manual identity. ACME issues for every discovered domain.

**Startup checks** (each is a hard error with a clear message):

- If neither `keys/` nor `private.jwk` exists, the server generates an ECDSA
  P-256 keypair, writes `private.jwk` (0600) and `keys/<kid>.json` (with
  `valid_from = now`), and logs the new kid + paths.
- Every file in `keys/` MUST be named `<kid>.json` where `kid` equals the
  JWK's `kid` field. Non-`.json` entries and subdirectories are ignored.
- The active `private.jwk`'s `kid` MUST be one of the published kids.
- The party envelope MUST contain at least one signature whose `kid` is
  published and which verifies against that key. Endorser signatures are
  allowed alongside.

**Ports:**

- `--http-port <int>` (default 80)
- `--https-port <int>` (default 443; only used with a TLS source)

**TLS sources (mutually exclusive):**

- `--acme-live` — Let's Encrypt production. Recommended: `--acme-email`.
- `--acme-test` — Let's Encrypt staging (untrusted certs; use during
  iteration to dodge production rate limits).
- `--tls-cert <path>` + `--tls-key <path>` — operator-supplied PEM cert/key.

ACME options:

- `--domain <fqdn>` — hostname the ACME client is allowed to issue for; MUST
  match the participant's GOBL Net address. Optional: when omitted, derived
  from the party's `gobl:` endpoint (`org.Party.endpoints[?(@.uri ~ /^gobl:/)]`).
- `--acme-email <email>` — ACME account email (recommended by LE). Optional:
  derived from the party's first `org.Party.emails` entry when omitted.
- `--cert-dir <path>` — directory used to cache ACME-issued certs (default
  `<config-dir>/certs/`).

Explicit flags always override party-derived values.

**Operational stances:**

| Stance                                          | Listens on   | Use when                                          |
|-------------------------------------------------|--------------|---------------------------------------------------|
| default (no TLS flags)                          | HTTP only    | Behind a reverse proxy that terminates TLS.       |
| `--acme-live` / `--acme-test` + `--domain`      | HTTP + HTTPS | Direct internet exposure; LE manages the cert.    |
| `--tls-cert` + `--tls-key`                      | HTTP + HTTPS | Cert is sourced elsewhere (corporate CA, …).      |

**Docker:**

```bash
docker run \
    -p 80:80 -p 443:443 \
    -v gobl-config:/root/.config/gobl \
    gobl net serve
```

For unprivileged containers, pick high ports inside and remap:

```bash
docker run \
    -p 80:8080 -p 443:8443 \
    -v gobl-config:/home/gobl/.config/gobl \
    gobl net serve --http-port 8080 --https-port 8443
```

**ACME operational sequence:** start → challenge (HTTP-01 on the HTTP port,
TLS-ALPN-01 fallback on the HTTPS port) → cert issued + cached → ready. If
the public internet can't reach the configured `--domain`, the challenge
fails and the server logs a clear error. Successful issuance doubles as a
reachability check.

### Logging

All operator-facing log output goes through `log/slog` and is written to
**stderr**. Result output (signed envelopes, the `/who` party JSON,
`gobl version`'s JSON) stays on **stdout**, so a pipeline like
`gobl sign … | gobl net send …` is unaffected.

The top-level `--json` flag toggles the format:

| flag      | stderr format        | example                                                        |
|-----------|----------------------|----------------------------------------------------------------|
| (default) | slog text            | `time=… level=INFO msg=listening scheme=http addr=:8080`       |
| `--json`  | slog JSON-per-line   | `{"time":"…","level":"INFO","msg":"listening","scheme":"http","addr":":8080"}` |

**Startup messages (`gobl net serve`):** `generated keypair`
(fields: `kid`, `private`, `key_file`); `initialised domain` (`domain`,
`party`, `inbox`); `GOBL Net listening` (`scheme`, `addr`); `ACME enabled`
(`domains`); `Shutting down`.

**HTTP access logs** — one baseline entry plus handler-specific ones:

| msg                  | level | fields                                                                            |
|----------------------|-------|-----------------------------------------------------------------------------------|
| `http_request`       | INFO  | `method`, `path`, `host`, `remote`, `status`, `duration_ms`                       |
| `keys.lookup`        | INFO  | `kid`, `found`                                                                    |
| `jwks.served`        | INFO  | `count`                                                                           |
| `who.exchange`       | INFO  | `caller` (verified `iss` as FQDN)                                                 |
| `who.rejected`       | WARN  | `reason` (`bad_body`/`verify_failed`/`not_allowed`), `remote`/`caller`/`error`    |
| `inbox.accepted`     | INFO  | `caller`, `envelope` (UUID)                                                       |
| `inbox.rejected`     | WARN  | `reason` (`bad_body`/`validation`/`verify_failed`/`aud_missing`/`aud_mismatch`/`not_allowed`)   |
| `inbox.write_failed` | ERROR | `caller`, `envelope`, `error`                                                     |

**Error reporting.** A CLI command that fails emits a single `command failed`
entry on stderr with `key=<gobl-error-key>` and (when present) `message=…`
and `faults=…`. With `--json` the same fields appear as a JSON object.
Successful commands write no log output and their result still lands on
stdout.

## Project structure

```
cmd/
  gobl/            CLI entry point          (binary: gobl)
  gobl.dev/        web server entry point   (binary: gobl.dev)
api/               HTTP API handler (package api)
bundle/            addon registration (package bundle)
editor/            browser editor (PopUI + Templ)
internal/
  ops/             document operations engine (build, validate, sign, …)
  mcp/             Model Context Protocol server
wasm/              WebAssembly build + gobl-worker npm package
.goreleaser.yml    CLI + wasm release configuration
Dockerfile         builds the gobl.dev web server
fly.toml           Fly.io configuration
```

## Releases

`release-cli.yaml` runs on every push to `main`: it bumps the semver tag
automatically (patch by default; `#minor` / `#major` in a commit message bump
further), releases the `gobl` CLI and wasm via GoReleaser, and publishes the
`gobl-worker` npm package.

## License

See [LICENSE](LICENSE).
