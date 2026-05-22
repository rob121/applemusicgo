package music

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var iTunesAPIClient = &http.Client{Timeout: 15 * time.Second}

var (
	persistentIDRe = regexp.MustCompile(`(?i)^[0-9a-f]{16}$`)
	catalogIDRe    = regexp.MustCompile(`^\d{6,}$`)
)

// ITunesTrackMeta is catalog metadata from the iTunes Lookup API.
type ITunesTrackMeta struct {
	TrackID      int64
	TrackName    string
	ArtistName   string
	CollectionID int64
	TrackNumber  int
	TrackViewURL string
}

type itunesLookupResponse struct {
	ResultCount int               `json:"resultCount"`
	Results     []itunesSearchHit `json:"results"`
}

// IsPersistentTrackID reports whether id is a 16-character Music.app persistent ID.
func IsPersistentTrackID(id string) bool {
	return persistentIDRe.MatchString(strings.TrimSpace(id))
}

// IsCatalogTrackID reports whether id is a numeric iTunes / Apple Music catalog track id.
func IsCatalogTrackID(id string) bool {
	return catalogIDRe.MatchString(strings.TrimSpace(id))
}

// LookupITunesTrack fetches catalog metadata for a track id via the iTunes Lookup API.
func LookupITunesTrack(trackID string) (*ITunesTrackMeta, error) {
	trackID = strings.TrimSpace(trackID)
	if !catalogIDRe.MatchString(trackID) {
		return nil, fmt.Errorf("invalid catalog track id")
	}
	u := fmt.Sprintf("https://itunes.apple.com/lookup?id=%s", url.PathEscape(trackID))
	resp, err := iTunesAPIClient.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("itunes lookup: HTTP %d", resp.StatusCode)
	}
	var payload itunesLookupResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	for _, r := range payload.Results {
		if r.Kind != "" && r.Kind != "song" {
			continue
		}
		return &ITunesTrackMeta{
			TrackID:      r.TrackID,
			TrackName:    r.TrackName,
			ArtistName:   r.ArtistName,
			CollectionID: r.CollectionID,
			TrackNumber:  r.TrackNumber,
			TrackViewURL: r.TrackViewURL,
		}, nil
	}
	return nil, fmt.Errorf("catalog track not found")
}

// ErrCatalogPlayFailed is returned when a catalog id was resolved but Music.app did not start playback.
var ErrCatalogPlayFailed = fmt.Errorf("catalog track could not be played")

// PlayCatalogTrack plays an Apple Music catalog track (subscription), not a library item.
// Metadata (name, artist, album URL) is loaded from the iTunes Lookup API.
func PlayCatalogTrack(catalogID string) (bool, error) {
	catalogID = strings.TrimSpace(catalogID)
	if !IsCatalogTrackID(catalogID) {
		return false, fmt.Errorf("invalid catalog track id")
	}

	meta, err := LookupITunesTrack(catalogID)
	if err != nil {
		return false, err
	}

	// Already in library (e.g. added on a previous request).
	if id, ok := findCatalogInLibrary(meta.TrackName, meta.ArtistName); ok {
		if played, err := playLibraryTrackByPersistentID(id); played || err != nil {
			return played, err
		}
	}

	// Add to Library then play is the most reliable path for Apple Music subscription tracks.
	if ok, err := playCatalogViaAddToLibrary(meta); err != nil {
		return false, err
	} else if ok {
		return true, nil
	}

	if ok, err := playCatalogViaStoreSearch(meta); err != nil {
		return false, err
	} else if ok {
		return true, nil
	}

	if ok, err := playCatalogViaITMSPage(meta); err != nil {
		return false, err
	} else if ok {
		return true, nil
	}

	for _, q := range catalogSearchQueries(meta.TrackName, meta.ArtistName) {
		if ok, err := playCatalogViaMusicSearchQuery(q); err != nil {
			return false, err
		} else if ok {
			return true, nil
		}
	}

	return false, ErrCatalogPlayFailed
}

func findCatalogInLibrary(name, artist string) (string, bool) {
	tracks, err := GetLibraryTracks()
	if err != nil {
		return "", false
	}
	id := matchLibraryTrack(tracks, name, artist)
	return id, id != ""
}

func playCatalogViaMusicSearchQuery(query string) (bool, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return false, nil
	}
	script := fmt.Sprintf(`
var music;
try { music = Application("Music"); } catch(e) { music = Application("iTunes"); }
var lib = music.libraryPlaylists[0];
var q = %q;
var results = music.search(lib, { for: q });
if (results.length === 0) { "0"; }
else {
  try {
    music.play(results[0]);
    "1";
  } catch (e) {
    try {
      results[0].play();
      "1";
    } catch (e2) {
      "0";
    }
  }
}
`, query)
	out, err := RunJXA(script)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(out) != "1" {
		return false, nil
	}
	return waitForPlayback(8 * time.Second), nil
}

func catalogSearchQueries(trackName, artist string) []string {
	seen := map[string]struct{}{}
	add := func(q string) {
		q = strings.TrimSpace(q)
		if q == "" {
			return
		}
		if _, ok := seen[q]; ok {
			return
		}
		seen[q] = struct{}{}
	}
	add(strings.TrimSpace(trackName + " " + artist))
	add(strings.TrimSpace(trackName))
	add(strings.TrimSpace(normalizeTrackTitle(trackName) + " " + foldAccents(artist)))
	return mapsKeys(seen)
}

func mapsKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func waitForPlayback(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		state, err := GetCurrentState()
		if err == nil {
			ps, _ := state["player_state"].(string)
			if ps == "playing" || ps == "paused" {
				return true
			}
		}
		time.Sleep(400 * time.Millisecond)
	}
	return false
}
