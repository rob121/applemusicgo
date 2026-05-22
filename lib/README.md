# lib/*.applescript

These files are **reference copies** from the original Node.js app. They are **not used at runtime**.

Standalone `.applescript` files fail to compile under `osascript` on current Music.app because of parser conflicts with properties like `id of` and certain `tell` forms (error `-2741`). The Go code runs equivalent logic via **inline `osascript -e`** (see `internal/music/artwork.go`) or **JXA** (see `internal/music/playlists.go`, `player.go`).

| File | Status | Replacement |
|------|--------|-------------|
| `get-playlists.applescript` | Broken as file | `listPlaylistsJXA` in `playlists.go` |
| `play-playlist.applescript` | Broken as file | JXA in `playPlaylistByID()` |
| `art.applescript` | Broken as file | `nowPlayingArtScript` in `artwork.go` |
| `album-art.applescript` | Broken as file | inline script in `FetchAndSaveArtwork()` |

Safe to delete these files if you do not need them for reference.
