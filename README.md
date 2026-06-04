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
| `gobl keygen` | Generate an ES256 key pair. |
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

### Editor-private routes

The browser editor uses a few additional routes that are not part of the GOBL
API surface:

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/_editor/examples` | List curated starter invoices (ID, label, country, addon) |
| `GET` | `/_editor/examples/{id}` | Raw JSON of a curated example |
| `GET` | `/_editor/formats` | List output formats the viewer can render |
| `POST` | `/_editor/convert?format={id}` | Convert a GOBL envelope to the requested output (UBL, CII, FatturaPA, HTML) |

## WebAssembly

[`wasm/`](./wasm) compiles GOBL to WebAssembly so it can run in the browser, and
publishes the `gobl-worker` npm package (a web-worker wrapper). The release
workflow attaches the wasm build to each GitHub Release, uploads it to
`cdn.gobl.org`, and publishes the npm package.

## Addons

[`bundle/bundle.go`](./bundle/bundle.go) is the single place that declares which
addons the binaries ship with — one blank import per addon module. Add an
approved addon there and both `gobl` and `gobl.dev` pick it up.

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
