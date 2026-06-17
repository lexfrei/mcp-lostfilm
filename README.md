# mcp-lostfilm

[![CI](https://github.com/lexfrei/mcp-lostfilm/actions/workflows/ci.yml/badge.svg)](https://github.com/lexfrei/mcp-lostfilm/actions/workflows/ci.yml) [![Release](https://img.shields.io/github/v/release/lexfrei/mcp-lostfilm?sort=semver)](https://github.com/lexfrei/mcp-lostfilm/releases) [![Go Report Card](https://goreportcard.com/badge/github.com/lexfrei/mcp-lostfilm)](https://goreportcard.com/report/github.com/lexfrei/mcp-lostfilm) [![Go](https://img.shields.io/github/go-mod/go-version/lexfrei/mcp-lostfilm)](go.mod) [![License](https://img.shields.io/github/license/lexfrei/mcp-lostfilm)](LICENSE)

MCP server for [LostFilm.TV](https://www.lostfilm.tv). Browse the release feed (the RSS equivalent), search current and older series, inspect a series' episodes, resolve an episode's per-quality torrents, and download `.torrent` files — all from any MCP-compatible client.

## Highlights

- **Discovery without credentials.** The release feed and series search are public; the server starts and serves them with no login configured.
- **Full resolve chain to the `.torrent`.** An episode is resolved through `v_search.php` → the intermediate redirect page → the per-quality download links, returning SD/720p/1080p/… variants with exact sizes.
- Mirror round-robin across the known LostFilm domains, with automatic failover on network and `5xx` errors.
- Canonical BitTorrent info-hash computed from the downloaded torrent's own bencode.
- Distroless multi-arch container image, signed with cosign.

## Features

- **lostfilm_feed** — list the latest releases from the public RSS feed (show, original title, episode, season/episode numbers, series id, poster, publish date). No authentication required.
- **lostfilm_search** — search series and movies by title, including the full back catalogue of older shows. Returns the series id and link used by the other tools. No authentication required.
- **lostfilm_series** — a series' metadata and full episode list (season/episode numbers, localized and original titles, episode page URLs). No authentication required.
- **lostfilm_torrents** — resolve the available quality variants and `.torrent` download links for an episode (use `999` as the episode for a whole-season pack). Requires authentication.
- **lostfilm_download** — download an episode's `.torrent` as base64 (ready to hand to a BitTorrent client), choosing a quality variant and enriched with the info-hash and file list; optionally saved to disk. Requires authentication.
- **lostfilm_server_version** — report the server version, revision, and Go runtime.

## Authentication

Discovery (`lostfilm_feed`, `lostfilm_search`, `lostfilm_series`) is public. Only `lostfilm_torrents` and `lostfilm_download` need a session, so the server starts even with no credentials and logs a warning instead of failing.

There are two ways to authenticate:

- **E-mail/password** (`LOSTFILM_EMAIL` + `LOSTFILM_PASSWORD`) — the server logs in via `ajaxik.php` and persists the resulting `lf_session` cookie. After several login attempts LostFilm starts demanding a captcha; when that happens the server returns a structured error pointing you at the cookie method.
- **Session cookie** (`LOSTFILM_COOKIE`) — paste an `lf_session` cookie from a logged-in browser. This bypasses both the captcha and any Cloudflare challenge, and is the most robust option. When a mirror is behind a Cloudflare challenge, include `cf_clearance=...` in the cookie and set `LOSTFILM_USER_AGENT` to the exact User-Agent that minted it.

## Mirrors and resilience

LostFilm rotates domains and is blocked in some regions. With no `LOSTFILM_BASE_URL` set, the client round-robins across known mirrors (`www.lostfilm.tv`, `www.lostfilmtv5.site`, `www.lostfilm.today`, `www.lostfilm.download`, `www.lostfilm.run`, `www.lostfilm.life`), failing over automatically on network and `5xx` errors. Pin a single reachable mirror by setting `LOSTFILM_BASE_URL` (e.g. `https://www.lostfilm.today/`).

The `.torrent` itself is served from a fixed download host (`n.tracktor.site`) via a signed, self-authenticating link, so the download step works without re-sending the session.

## Configuration

Configuration is read from environment variables. No variable is required to start; credentials are needed only for torrent tools.

| Variable | Description | Default |
| --- | --- | --- |
| `LOSTFILM_EMAIL` | Account e-mail for login | — |
| `LOSTFILM_PASSWORD` | Account password for login | — |
| `LOSTFILM_COOKIE` | Raw cookie (`lf_session=...`, optionally `; cf_clearance=...`), used instead of a password login | — |
| `LOSTFILM_COOKIE_FILE` | Path to persist the session between runs | `~/.mcp-lostfilm/cookies.json` (bare process); `/home/nobody/.mcp-lostfilm/cookies.json` (container, set in the image) |
| `LOSTFILM_BASE_URL` | Pin a single mirror (e.g. `https://www.lostfilm.today/`) | round-robin across mirrors |
| `LOSTFILM_USER_AGENT` | Override the browser User-Agent (match the one used for `cf_clearance`) | recent Chrome |
| `LOSTFILM_PROXY` | HTTP/SOCKS5 proxy URL | — |
| `LOSTFILM_DOWNLOAD_DIR` | Directory for `saveToDisk` downloads | — |
| `MCP_HTTP_PORT` | Enable the HTTP transport on this port | stdio only |
| `MCP_HTTP_HOST` | HTTP bind host | `127.0.0.1` |

## Usage

With Claude Code, via the bundled `.mcp.json` (Docker):

```json
{
  "mcpServers": {
    "mcp-lostfilm": {
      "command": "docker",
      "args": [
        "run", "--rm", "-i",
        "-e", "LOSTFILM_EMAIL",
        "-e", "LOSTFILM_PASSWORD",
        "-e", "LOSTFILM_COOKIE",
        "-e", "LOSTFILM_BASE_URL",
        "-v", "mcp-lostfilm-session:/home/nobody/.mcp-lostfilm",
        "ghcr.io/lexfrei/mcp-lostfilm:latest"
      ],
      "env": {
        "LOSTFILM_EMAIL": "your-email",
        "LOSTFILM_PASSWORD": "your-password"
      }
    }
  }
}
```

The bundled `.mcp.json` ships with empty `LOSTFILM_EMAIL`/`LOSTFILM_PASSWORD` values. You can leave them empty to use only the public discovery tools, fill them in, or pass a `LOSTFILM_COOKIE` instead. The named volume persists the session cookie across `--rm` container runs; drop it if you do not want persistence.

The base64 returned by `lostfilm_download` is directly compatible with the `metainfo` parameter of a sibling Transmission MCP server, so a feed entry can be resolved, downloaded, and added to a torrent client in one chain.

## Development

```bash
go build ./cmd/mcp-lostfilm
go test -race ./...
golangci-lint run
```

An opt-in live integration test exercises the full flow against the real site. Prefer the cookie variant — repeated logins trigger a captcha:

```bash
LOSTFILM_LIVE=1 LOSTFILM_COOKIE='lf_session=...' \
  go test -run TestLive -count=1 ./internal/lostfilm/
```

## Support

If this project is useful to you, you can support its development via [GitHub Sponsors](https://github.com/sponsors/lexfrei).

## See also

Other LostFilm tooling worth knowing about: [Jackett's LostFilm indexer](https://github.com/Jackett/Jackett), the [FlexGet lostfilm plugin](https://flexget.com/Plugins/lostfilm), and [lAnubisl/LostFilmTorrentsFeed](https://github.com/lAnubisl/LostFilmTorrentsFeed) (a configured RSS-feed service).

## License

BSD 3-Clause. See [LICENSE](LICENSE).
