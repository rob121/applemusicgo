package music

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const defaultSearchLimit = 50
const maxSearchLimit = 100


// DefaultSearchLimit returns the default result cap for GET /music/search.
func DefaultSearchLimit() int { return defaultSearchLimit }

// MusicSearchTrack is a track from catalog search, Music.app, or the local library cache.
type MusicSearchTrack struct {
	ID             string `json:"id,omitempty"`
	DatabaseID     int64  `json:"database_id,omitempty"`
	CatalogTrackID int64  `json:"catalog_track_id,omitempty"`
	Name           string `json:"name"`
	Artist         string `json:"artist"`
	Album          string `json:"album"`
	Kind           string `json:"kind,omitempty"`
	Source         string `json:"source"`
	TrackViewURL   string `json:"track_view_url,omitempty"`
	ArtworkURL     string `json:"artwork_url,omitempty"`
}

// MusicSearchResponse is returned by GET /music/search.
type MusicSearchResponse struct {
	Query  string             `json:"query"`
	Tracks []MusicSearchTrack `json:"tracks"`
}

// SearchMusic searches the Apple Music / iTunes catalog (iTunes Search API), Music.app
// library search (osascript), and the cached local library index, then merges results.
// This is broader than GET /library/search, which only matches track titles in cache.
func SearchMusic(query string, limit int) (*MusicSearchResponse, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > maxSearchLimit {
		limit = maxSearchLimit
	}

	catalog, _ := searchITunesCatalog(query, limit)

	appResults, err := searchViaMusicApp(query, limit)
	if err != nil {
		appResults = nil
	}

	var local []MusicSearchTrack
	if libTracks, libErr := GetLibraryTracks(); libErr == nil {
		local = searchLibraryTracks(libTracks, query, limit)
	}

	merged := mergeMusicSearchResults(catalog, appResults, local, limit)
	return &MusicSearchResponse{Query: query, Tracks: merged}, nil
}

type itunesSearchResponse struct {
	ResultCount int              `json:"resultCount"`
	Results     []itunesSearchHit `json:"results"`
}

type itunesSearchHit struct {
	TrackID       int64  `json:"trackId"`
	TrackName     string `json:"trackName"`
	ArtistName    string `json:"artistName"`
	CollectionID  int64  `json:"collectionId"`
	Collection    string `json:"collectionName"`
	TrackNumber   int    `json:"trackNumber"`
	TrackViewURL  string `json:"trackViewUrl"`
	ArtworkURL100 string `json:"artworkUrl100"`
	Kind          string `json:"kind"`
}

func searchITunesCatalog(query string, limit int) ([]MusicSearchTrack, error) {
	u := fmt.Sprintf(
		"https://itunes.apple.com/search?term=%s&entity=song&limit=%d",
		url.QueryEscape(query),
		limit,
	)
	resp, err := iTunesAPIClient.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("itunes search: HTTP %d", resp.StatusCode)
	}
	var payload itunesSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	tracks := make([]MusicSearchTrack, 0, len(payload.Results))
	for _, r := range payload.Results {
		if r.Kind != "" && r.Kind != "song" {
			continue
		}
		tracks = append(tracks, MusicSearchTrack{
			CatalogTrackID: r.TrackID,
			ID:             strconv.FormatInt(r.TrackID, 10),
			Name:           r.TrackName,
			Artist:         r.ArtistName,
			Album:          r.Collection,
			Kind:           "song",
			Source:         "catalog",
			TrackViewURL:   r.TrackViewURL,
			ArtworkURL:     strings.Replace(r.ArtworkURL100, "100x100bb", "200x200bb", 1),
		})
	}
	return tracks, nil
}

func searchViaMusicApp(query string, limit int) ([]MusicSearchTrack, error) {
	queryJSON, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}
	script := fmt.Sprintf(`
var music;
try { music = Application("Music"); } catch(e) { music = Application("iTunes"); }
var lib = music.libraryPlaylists[0];
var q = %s;
var limit = %d;
var results = music.search(lib, { for: q });
var out = [];
for (var i = 0; i < results.length && out.length < limit; i++) {
  var t = results[i];
  try {
    var kind = "";
    try { kind = t.mediaKind(); } catch(e) {}
    if (kind !== "song" && kind !== "" && kind !== undefined) { continue; }
    var id = "";
    try { id = t.persistentID(); } catch(e) {}
    if (!id) { continue; }
    out.push({
      id: id,
      database_id: t.id(),
      name: t.name() || "",
      artist: t.artist() || "",
      album: t.album() || "",
      kind: kind || "song",
      source: "music"
    });
  } catch(e) {}
}
JSON.stringify(out);
`, string(queryJSON), limit)
	out, err := RunJXA(script)
	if err != nil {
		return nil, err
	}
	var tracks []MusicSearchTrack
	if out == "" {
		return tracks, nil
	}
	if err := json.Unmarshal([]byte(out), &tracks); err != nil {
		return nil, fmt.Errorf("parse music search: %w", err)
	}
	return tracks, nil
}

func searchLibraryTracks(tracks []LibraryTrack, query string, limit int) []MusicSearchTrack {
	q := strings.ToLower(query)
	var results []MusicSearchTrack
	for _, t := range tracks {
		if len(results) >= limit {
			break
		}
		name := strings.ToLower(t.Name)
		artist := strings.ToLower(t.Artist)
		album := strings.ToLower(t.Album)
		albumArtist := strings.ToLower(t.AlbumArtist)
		if !strings.Contains(name, q) &&
			!strings.Contains(artist, q) &&
			!strings.Contains(album, q) &&
			!strings.Contains(albumArtist, q) {
			continue
		}
		results = append(results, MusicSearchTrack{
			ID:     t.ID,
			Name:   t.Name,
			Artist: t.Artist,
			Album:  t.Album,
			Kind:   "song",
			Source: "library",
		})
	}
	return results
}

func searchMatchKey(name, artist string) string {
	return strings.ToLower(strings.TrimSpace(name)) + "|" + strings.ToLower(strings.TrimSpace(artist))
}

func mergeMusicSearchResults(catalog, app, local []MusicSearchTrack, limit int) []MusicSearchTrack {
	type entry struct {
		track MusicSearchTrack
		order int
	}
	byKey := map[string]*entry{}
	byPersistentID := map[string]*entry{}
	orderKeys := []string{}
	n := 0

	upsert := func(t MusicSearchTrack) {
		if t.ID != "" && len(t.ID) == 16 && isHexID(t.ID) {
			if ex, ok := byPersistentID[t.ID]; ok {
				enrichSearchTrack(&ex.track, t)
				return
			}
		}
		key := searchMatchKey(t.Name, t.Artist)
		if key == "|" {
			return
		}
		if ex, ok := byKey[key]; ok {
			enrichSearchTrack(&ex.track, t)
			if ex.track.ID != "" && len(ex.track.ID) == 16 {
				byPersistentID[ex.track.ID] = ex
			}
			return
		}
		e := &entry{track: t, order: n}
		n++
		byKey[key] = e
		orderKeys = append(orderKeys, key)
		if t.ID != "" && len(t.ID) == 16 && isHexID(t.ID) {
			byPersistentID[t.ID] = e
		}
	}

	for _, t := range catalog {
		upsert(t)
	}
	for _, t := range app {
		upsert(t)
	}
	for _, t := range local {
		upsert(t)
	}

	out := make([]MusicSearchTrack, 0, limit)
	for _, key := range orderKeys {
		if len(out) >= limit {
			break
		}
		out = append(out, byKey[key].track)
	}
	return out
}

func isHexID(s string) bool {
	for _, c := range s {
		if (c >= '0' && c <= '9') || (c >= 'A' && c <= 'F') || (c >= 'a' && c <= 'f') {
			continue
		}
		return false
	}
	return true
}

func enrichSearchTrack(dst *MusicSearchTrack, src MusicSearchTrack) {
	if src.Name != "" {
		dst.Name = src.Name
	}
	if src.Artist != "" {
		dst.Artist = src.Artist
	}
	if src.Album != "" {
		dst.Album = src.Album
	}
	if src.Kind != "" {
		dst.Kind = src.Kind
	}
	if src.TrackViewURL != "" {
		dst.TrackViewURL = src.TrackViewURL
	}
	if src.CatalogTrackID != 0 && dst.CatalogTrackID == 0 {
		dst.CatalogTrackID = src.CatalogTrackID
	}
	if src.DatabaseID != 0 && dst.DatabaseID == 0 {
		dst.DatabaseID = src.DatabaseID
	}
	// Prefer Music library persistent ID over iTunes catalog numeric id.
	if src.ID != "" && len(src.ID) == 16 && isHexID(src.ID) {
		dst.ID = src.ID
		dst.Source = src.Source
		return
	}
	if dst.ID == "" && src.ID != "" {
		dst.ID = src.ID
	}
	if dst.Source == "" || dst.Source == "catalog" {
		if src.Source != "" {
			dst.Source = src.Source
		}
	}
}
