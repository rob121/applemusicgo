# applemusicgo

Go reimplementation of the Apple Music HTTP/CLI server for macOS. It drives **Music.app** (or iTunes) through **osascript** (JXA and AppleScript) and implements the same REST API as [apple-music-custom](https://github.com/Hackashaq666/apple-music-custom), with added catalog search and Apple Music subscription playback.

## Requirements

- macOS with Apple Music (or iTunes)
- Go 1.22+
- **Apple Music subscription** for playing catalog tracks that are not in your library
- **Accessibility permission** for the process that runs `applemusicgo` (Terminal, Cursor, iTerm, etc.) — required for catalog play via Music.app UI automation

## Build

```bash
go build -o applemusicgo ./cmd/applemusicgo
```

Run from the repo root so `./lib/` resolves, or set `APPLEMUSICGO_LIB`.

## HTTP server

```bash
./applemusicgo serve
# or
PORT=8181 ./applemusicgo serve --dir .
```

Default port is **8181** (`PORT` env or `--port`).

**Web player:** [http://localhost:8181/](http://localhost:8181/) — search, play, and now-playing controls with album art.

**API docs:** [http://localhost:8181/swagger/](http://localhost:8181/swagger/) (alias `/docs`). OpenAPI spec at `/openapi.yaml`. Use the running server URL — do not open `swagger-ui.html` as a `file://` page.

### Endpoints

| Area | Examples |
|------|------------|
| Health | `GET /_ping` |
| Player | `PUT /play`, `/pause`, `/stop`, `/power`, `/next`, `/volume`, `/seek`, … · `GET /now_playing` |
| Library | `GET /library/artists`, `/library/albums`, `/library/search` · `PUT /library/tracks/{id}/play` |
| Catalog search | `GET /music/search?q=…&limit=50` |
| Playlists | `GET /playlists` · `PUT /playlists/{id}/play` |
| Artwork | `GET /artwork`, `/artwork/{artist}/{album}`, … |
| AirPlay | `GET /airplay_devices` · `PUT /airplay_devices/{id}/on` |
| Events | `GET /events` (SSE) · `POST /notify` |

Full details are in Swagger or `api/openapi.yaml`.

### Search

- **`GET /library/search?q=…`** — fast filter over the cached local library (track title only).
- **`GET /music/search?q=…&limit=50`** — iTunes Search API (full catalog) + Music.app search + local library (name/artist/album). Each hit has a `source`: `catalog`, `music`, or `library`. Library tracks use a 16-char hex `id` (persistent ID); catalog-only hits use a numeric `id` / `catalog_track_id`.

### Play by id

**`PUT /library/tracks/{id}/play`** accepts a single id and resolves the type automatically:

| Id shape | Meaning |
|----------|---------|
| 16-char hex | Library persistent ID |
| Numeric (6+ digits) | Apple Music catalog track id (from `/music/search`) |

Catalog metadata is fetched from the iTunes Lookup API. Music.app adds/plays the track via subscription streaming — it does not need to be in your library first.

```bash
# Catalog track
curl -X PUT http://localhost:8181/library/tracks/1738363893/play

# Library track
curl -X PUT http://localhost:8181/library/tracks/8359A9EB1A4FC305/play
```

First catalog play can take **~30 seconds** while Music.app loads the album page and adds the track. Subsequent plays are fast if the track is already in your library. On failure, the API returns **404** with a JSON body explaining likely causes (Accessibility, Apple Music sign-in).

### Now-playing artwork

While a track is **playing or paused**, fetch the current cover as JPEG:

```bash
curl -o cover.jpg http://localhost:8181/artwork/now.jpg
# or
curl -o cover.jpg http://localhost:8181/artwork
```

Any `/artwork/{name}.jpg` path returns now-playing art (`/artwork/cover.jpg`, etc.) — useful for clients that require a `.jpg` URL. Album art by library metadata remains at `/artwork/{artist}/{album}` (two path segments).

Each request pulls art from Music.app and updates `{data-dir}/artwork-cache/now-playing.jpg`. `GET /now_playing` includes `"artwork_url": "/artwork/now.jpg"` when a track is active.

## CLI

```bash
./applemusicgo play
./applemusicgo now-playing
./applemusicgo volume 50
./applemusicgo play-track 1738363893    # library hex or catalog numeric id
./applemusicgo serve --port 8181
```

Other commands: `pause`, `playpause`, `stop`, `previous`, `next`, `mute`, `shuffle`, `repeat`, `seek`.

## Install as a service (launchd)

To run at login on a Mac (local or remote via SSH), see **[install/README.md](install/README.md)**.

```bash
chmod +x install/*.sh
./install/install-remote.sh user@your-mac
```

This cross-compiles, copies the binary and plist, and registers a `LaunchAgent` at `~/Library/LaunchAgents/com.rob121.applemusicgo.plist`.

## Apple Music error 3048

If Music shows **“There was a problem downloading … (3048)”** when playing a track, that usually means a **purchased/cloud track** could not be downloaded (authorization or network). Single-track library play uses direct playback (no temp playlist) to avoid forcing a download. If it persists: **Music → Account → Authorizations → Authorize This Computer**, sign in with the Apple ID used for the purchase, then retry.

## Layout

- `cmd/applemusicgo` — CLI and server entrypoint
- `internal/music` — osascript / JXA integration (player, library, search, catalog play, artwork)
- `internal/server` — HTTP API and Swagger UI
- `api/` — embedded OpenAPI spec
- `install/` — launchd plist template and install/deploy scripts
- `lib/` — reference AppleScript only (not used at runtime; see `lib/README.md`)

## Credits

This project is a Go port of the macOS REST API used by **[apple-music-custom](https://github.com/Hackashaq666/apple-music-custom)** — Apple Music for Home Assistant (MIT license). That server’s API traces back to **[itunes-api](https://github.com/jonmaddox/itunes-api)** by Jon Maddox, with further updates by chasut for the modern Music.app.

Use **apple-music-custom** if you want the Home Assistant integration, launchd service install, and macOS push notifications (`notify.py` + SSE). Use **applemusicgo** for a single Go binary, Swagger docs, and catalog search/play without Node.js.

## Environment

| Variable | Default | Purpose |
|----------|---------|---------|
| `PORT` | `8181` | HTTP listen port |
| `APPLEMUSICGO_LIB` | `<dir>/lib` | Path to `lib/` (reference scripts) |
