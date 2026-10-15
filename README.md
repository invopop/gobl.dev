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
GOBL addon and converter (see [`bundle/`](./bundle)) and ships two binaries:

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
| `gobl convert` | Convert documents between GOBL and external formats such as UBL and CII (see [Converters](#converters)). |
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

# Export an invoice as UBL, and import it back into GOBL
gobl convert ./invoice.json ./invoice.xml --to ubl+en16931-v2017
gobl convert -i ./invoice.xml

# Embed a PDF version of the invoice, and save embedded files when importing
gobl convert ./invoice.json ./invoice.xml --to ubl+peppol+bis-v3 --attach ./invoice.pdf
gobl convert ./invoice.xml ./invoice.json --attachments ./files

# List the available conversion formats
gobl convert --list -i

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

The editor has two modes:

- **Build** calculates and validates the GOBL document, replacing it with the
  result.
- **Convert** shows the GOBL document beside a document in another format,
  both editable. **Export →**, above the GOBL editor, builds the GOBL document
  without changing it and writes it into the selected format; only formats
  that accept the document are offered, as checked through `POST /v0/formats`
  while it is edited. **← Import**, above the other editor, detects that
  document's format and replaces the GOBL document with the imported one.

Bare-minimum example documents for each region live in
[`editor/examples`](./editor/examples), picked above the GOBL editor, and
offered above the other editor as samples exported into their most specific
format. They are labelled from their regime and addons, so adding one only
requires adding its file.

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
| `POST` | `/v0/import` | Detect the format of the raw body (e.g. UBL XML) and import it into a GOBL envelope; `?format=` limits detection, and `?attachments=true` also returns the embedded files (see [Attachments](#attachments)) |
| `POST` | `/v0/export?format={key}` | Export a GOBL document or envelope into the first accepting format; the key used is returned in the `GOBL-Format` header. A `multipart/form-data` body also embeds files (see [Attachments](#attachments)) |
| `POST` | `/v0/keygen` | Generate an ES256 key pair |
| `GET` | `/v0/schemas` | List registered JSON schemas |
| `GET` | `/v0/schemas/{path}` | Fetch a specific schema (add `?bundle` for bundled) |
| `GET` | `/v0/regimes` | List available tax regimes |
| `GET` | `/v0/regimes/{code}` | Fetch a specific tax regime definition |
| `GET` | `/v0/addons` | List available addons |
| `GET` | `/v0/addons/{key}` | Fetch a specific addon definition |
| `GET` | `/v0/formats` | List the conversion formats registered by the bundled converters |
| `POST` | `/v0/formats` | Check which formats accept a GOBL document, with the reasons for refusals |
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

## Converters

Conversions between GOBL and external formats go through GOBL's
[`convert`](https://github.com/invopop/gobl/tree/main/convert) register:
converter modules register their formats when imported, and gobl.dev offers
exactly the formats registered, through `gobl convert`, the `/v0/import`,
`/v0/export` and `/v0/formats` endpoints, and the editor. The converters are
blank-imported in [`bundle/bundle.go`](./bundle/bundle.go) alongside the
addons:

| Module | Formats |
|--------|---------|
| [gobl.ubl](https://github.com/invopop/gobl.ubl) | `ubl+en16931-v2017`, `ubl+peppol+bis-v3` |
| [gobl.cii](https://github.com/invopop/gobl.cii) | `cii+en16931-v2017`, `cii+peppol+bis-v3` |
| [gobl.de.xinvoice](https://github.com/invopop/gobl.de.xinvoice) | `ubl+de-xrechnung-v3`, `cii+de-xrechnung-v3`, `cii+de-zugferd-v2` (`+basic`, `+extended`) |
| [gobl.es.facturae](https://github.com/invopop/gobl.es.facturae) | `es-facturae-v3` |
| [gobl.fr.ctc](https://github.com/invopop/gobl.fr.ctc) | `ubl+fr-ctc-cius-v1`, `ubl+fr-ctc-extended-v1`, `cii+fr-ctc-cius-v1`, `cii+fr-ctc-extended-v1`, `cii+fr-ctc-facturx-v1`, `cii+fr-facturx-v1` (`+basic`, `+extended`), `cii+fr-choruspro-v1`, `cdar+fr-ctc-cdv-v1` (`+ppf`) |
| [gobl.sa.zatca](https://github.com/invopop/gobl.sa.zatca) | `ubl+sa-zatca-v1` |

Format keys follow GOBL's naming rules (`<syntax>[+<layer>...]`). Each
format's converter decides whether it accepts a document, typically requiring
the addons of its specification, and explains a refusal with rules faults.
`POST /v0/formats` reports, for each format that exports a document's schema,
whether it accepts the document and why not.

### Attachments

Formats with `attachments: true` in `/v0/formats`, such as UBL and most CII
formats, can embed files in exported documents, such as a PDF version of the
invoice. Formats that cannot embed them are skipped when attachments are given.

- `gobl convert --to ... --attach file.pdf` embeds a file, and may be repeated.
  The MIME type is taken from the file's extension.
- `POST /v0/export` takes a `multipart/form-data` body with the GOBL document
  in a `document` field or file, and each file to embed in an `attachment`
  file part:

  ```bash
  curl -F document=@invoice.json -F attachment=@invoice.pdf \
    "https://gobl.dev/v0/export?format=ubl+peppol+bis-v3"
  ```

Imports provide the files embedded in UBL and CII documents separately from
the envelope:

- `gobl convert --attachments DIR` saves them to the directory, replacing
  existing files. Only the base of each file name is used, and names repeated
  in the document are numbered.
- `POST /v0/import` returns the envelope alone, as before, with the number of
  embedded files in the `GOBL-Attachments` header. With `?attachments=true`,
  it returns `{"format", "envelope", "attachments"}` instead, each attachment
  with its `id`, `name`, `description`, `mime` and base64 `data`.

## Project structure

```
cmd/
  gobl/            CLI entry point          (binary: gobl)
  gobl.dev/        web server entry point   (binary: gobl.dev)
api/               HTTP API handler (package api)
bundle/            addon and converter registration (package bundle)
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

`release-cli.yaml` runs on every push to `main`: it derives and pushes the next
semver tag, releases the `gobl` CLI and wasm via GoReleaser, and publishes the
`gobl-worker` npm package.

### Versioning

gobl.dev pins its **major and minor** to the core
[GOBL](https://github.com/invopop/gobl) release it is built against, and uses
the **patch** component for its own releases:

| gobl.dev   | Core GOBL | Meaning |
|------------|-----------|---------|
| `v0.505.0` | 0.505.0   | first gobl.dev release on the GOBL 0.505 line |
| `v0.505.3` | 0.505.x   | fourth gobl.dev release on that line |
| `v0.506.0` | 0.506.0   | GOBL moved to 0.506, so the patch resets |

A core *patch* release is absorbed into the next gobl.dev patch: if gobl.dev is
at `v0.505.4` when GOBL 0.505.1 lands, the release carrying it is `v0.505.5`.
The tag name therefore identifies the core *line*, not the exact core version —
that is reported by `gobl version` (from the build info), recorded in the
annotated tag message, and published in the release notes.

The version is a function of `go.mod` plus the existing tags, computed by
[`.github/scripts/next-version.sh`](.github/scripts/next-version.sh). Bumping
the GOBL requirement is what moves the minor; there is no manual version step.
Because releases must only ever move forward, *downgrading* the GOBL
requirement is refused by that script — roll forward instead.

To track a whole core line rather than an exact release, use the two-component
form. It is not a valid version, but both Go and npm accept it as a query:

```bash
# newest gobl.dev on the GOBL 0.505 line
go get github.com/invopop/gobl.dev@v0.505
```

```jsonc
// package.json. Note this floats to new patches on install,
// where Go resolves the query once.
{ "dependencies": { "@invopop/gobl-worker": "0.505" } }
```

## License

See [LICENSE](LICENSE).
