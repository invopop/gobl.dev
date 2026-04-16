# gobl.dev

Web editor and HTTP API server for [GOBL](https://github.com/invopop/gobl) (Go Business Language).

This project composes the GOBL HTTP API (provided by [`gobl/pkg/api`](https://github.com/invopop/gobl/tree/main/pkg/api)) with a browser-based document editor built using [PopUI](https://github.com/invopop/popui.go) and [Templ](https://templ.guide). It powers the public instance at [gobl.dev](https://gobl.dev).

## Running locally

```bash
go run .
```

The server listens on port 8080 by default. Set the `PORT` environment variable to override:

```bash
PORT=3000 go run .
```

Open [http://localhost:8080](http://localhost:8080) to access the editor.

## API

All GOBL API endpoints are available under the `/v0` prefix:

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

## Docker

```bash
docker build -t gobl-dev .
docker run -p 8080:8080 gobl-dev
```

## Deployment

The app is deployed to [Fly.io](https://fly.io) via the `deploy.yaml` workflow on push to `main`. The Fly app name is configured in `fly.toml`.

GOBL dependency updates are automated: when a new GOBL version is tagged, the `update-gobl.yaml` workflow creates a pull request with the updated dependency.

## Project structure

```
main.go              Server entry point
editor/
  editor.go          HTTP handler and asset registration
  editor.templ       Templ template (PopUI components)
  assets/            JavaScript, CSS, and static files
Dockerfile           Multi-stage Alpine build
fly.toml             Fly.io configuration
```

## License

See [LICENSE](LICENSE).
