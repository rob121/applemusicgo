package music

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"
)

type LibraryTrack struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Artist      string  `json:"artist"`
	AlbumArtist string  `json:"albumArtist"`
	Album       string  `json:"album"`
	TrackNumber int     `json:"track_number"`
	DiscNumber  int     `json:"disc_number"`
	Duration    float64 `json:"duration"`
}

const fetchTracksJXA = `var app; try { app = Application("Music"); } catch(e) { app = Application("iTunes"); } var tracks = []; try { var lib = app.libraryPlaylists[0]; var allIDs = lib.tracks.persistentID(); var allKinds = lib.tracks.mediaKind(); var allNames = lib.tracks.name(); var allArtists = lib.tracks.artist(); var allAlbumArtists = lib.tracks.albumArtist(); var allAlbums = lib.tracks.album(); var allTrackNums = lib.tracks.trackNumber(); var allDiscNums = lib.tracks.discNumber(); var allDurations = lib.tracks.duration(); for (var i = 0; i < allIDs.length; i++) { var kind = allKinds[i] || ""; if (kind !== "song" && kind !== "" && kind !== undefined) { continue; } var id = allIDs[i] || ""; if (!id) { continue; } tracks.push({ id: id, name: allNames[i] || "", artist: allArtists[i] || "", albumArtist: allAlbumArtists[i] || "", album: allAlbums[i] || "", track_number: allTrackNums[i] || 0, disc_number: allDiscNums[i] || 1, duration: allDurations[i] || 0 }); } } catch(bulkErr) { try { var raw = app.tracks(); for (var i = 0; i < raw.length; i++) { try { var t = raw[i]; var kind = ""; try { kind = t.mediaKind(); } catch(e) {} if (kind !== "song" && kind !== "" && kind !== undefined) { continue; } var id = ""; try { id = t.persistentID(); } catch(e) {} if (!id) { continue; } tracks.push({ id: id, name: (function(){ try { return t.name() || ""; } catch(e) { return ""; } })(), artist: (function(){ try { return t.artist() || ""; } catch(e) { return ""; } })(), albumArtist: (function(){ try { return t.albumArtist() || ""; } catch(e) { return ""; } })(), album: (function(){ try { return t.album() || ""; } catch(e) { return ""; } })(), track_number: (function(){ try { return t.trackNumber(); } catch(e) { return 0; } })(), disc_number: (function(){ try { return t.discNumber(); } catch(e) { return 1; } })(), duration: (function(){ try { return t.duration(); } catch(e) { return 0; } })() }); } catch(e) {} } } catch(e) {} } JSON.stringify(tracks);`

type libraryCache struct {
	mu        sync.Mutex
	tracks    []LibraryTrack
	fetchedAt time.Time
	ttl       time.Duration
	pending   []chan libraryResult
}

type libraryResult struct {
	tracks []LibraryTrack
	err    error
}

var libCache = &libraryCache{ttl: time.Hour}

type AlbumEntry struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Artist string `json:"artist"`
}

type ArtistEntry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type AlbumsResponse struct {
	Total  int          `json:"total"`
	Offset int          `json:"offset"`
	Limit  int          `json:"limit"`
	Albums []AlbumEntry `json:"albums"`
}

type ArtistsResponse struct {
	Total   int           `json:"total"`
	Offset  int           `json:"offset"`
	Limit   int           `json:"limit"`
	Artists []ArtistEntry `json:"artists"`
}

var (
	albumsCacheFull  *AlbumsResponse
	artistsCacheFull *ArtistsResponse
	cacheMu          sync.RWMutex
)

func InvalidateLibraryCache() {
	libCache.mu.Lock()
	libCache.fetchedAt = time.Time{}
	libCache.mu.Unlock()
	cacheMu.Lock()
	albumsCacheFull = nil
	artistsCacheFull = nil
	cacheMu.Unlock()
}

func GetLibraryTracks() ([]LibraryTrack, error) {
	libCache.mu.Lock()
	now := time.Now()
	if libCache.tracks != nil && now.Sub(libCache.fetchedAt) < libCache.ttl {
		tracks := libCache.tracks
		libCache.mu.Unlock()
		return tracks, nil
	}

	ch := make(chan libraryResult, 1)
	libCache.pending = append(libCache.pending, ch)
	isLeader := len(libCache.pending) == 1
	libCache.mu.Unlock()

	if !isLeader {
		res := <-ch
		return res.tracks, res.err
	}

	out, err := RunJXAWithBuffer(fetchTracksJXA, maxBuffer)
	var tracks []LibraryTrack
	if err == nil {
		err = json.Unmarshal([]byte(out), &tracks)
	}

	libCache.mu.Lock()
	callbacks := libCache.pending
	libCache.pending = nil
	if err == nil {
		libCache.tracks = tracks
		libCache.fetchedAt = time.Now()
		cacheMu.Lock()
		albumsCacheFull = nil
		artistsCacheFull = nil
		cacheMu.Unlock()
	}
	libCache.mu.Unlock()

	res := libraryResult{tracks: tracks, err: err}
	for _, c := range callbacks {
		c <- res
	}
	return tracks, err
}

func BuildAlbums(tracks []LibraryTrack, offset, limit int) AlbumsResponse {
	seen := map[string]bool{}
	var albums []AlbumEntry
	for _, t := range tracks {
		name := t.Album
		if name == "" {
			continue
		}
		artist := t.AlbumArtist
		if artist == "" {
			artist = t.Artist
		}
		key := artist + "||" + name
		if seen[key] {
			continue
		}
		seen[key] = true
		albums = append(albums, AlbumEntry{ID: Slugify(key), Name: name, Artist: artist})
	}
	sort.Slice(albums, func(i, j int) bool {
		return strings.ToLower(albums[i].Name) < strings.ToLower(albums[j].Name)
	})
	end := offset + limit
	if end > len(albums) {
		end = len(albums)
	}
	if offset > len(albums) {
		offset = len(albums)
	}
	return AlbumsResponse{
		Total:  len(albums),
		Offset: offset,
		Limit:  limit,
		Albums: albums[offset:end],
	}
}

func BuildArtists(tracks []LibraryTrack, offset, limit int) ArtistsResponse {
	seen := map[string]bool{}
	var artists []ArtistEntry
	for _, t := range tracks {
		name := t.AlbumArtist
		if name == "" {
			name = t.Artist
		}
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		artists = append(artists, ArtistEntry{ID: Slugify(name), Name: name})
	}
	sort.Slice(artists, func(i, j int) bool {
		return strings.ToLower(artists[i].Name) < strings.ToLower(artists[j].Name)
	})
	end := offset + limit
	if end > len(artists) {
		end = len(artists)
	}
	if offset > len(artists) {
		offset = len(artists)
	}
	return ArtistsResponse{
		Total:   len(artists),
		Offset:  offset,
		Limit:   limit,
		Artists: artists[offset:end],
	}
}

func BuildAlbumsByArtist(tracks []LibraryTrack, artistName string) map[string]any {
	seen := map[string]bool{}
	var albums []map[string]string
	for _, t := range tracks {
		albumArtist := t.AlbumArtist
		if albumArtist == "" {
			albumArtist = t.Artist
		}
		artist := t.Artist
		if albumArtist != artistName && artist != artistName {
			continue
		}
		name := t.Album
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		albums = append(albums, map[string]string{
			"id":   Slugify(artistName + "||" + name),
			"name": name,
		})
	}
	sort.Slice(albums, func(i, j int) bool {
		return strings.ToLower(albums[i]["name"]) < strings.ToLower(albums[j]["name"])
	})
	return map[string]any{"artist": artistName, "albums": albums}
}

func GetCachedAlbums() *AlbumsResponse {
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	return albumsCacheFull
}

func SetCachedAlbums(a *AlbumsResponse) {
	cacheMu.Lock()
	albumsCacheFull = a
	cacheMu.Unlock()
}

func GetCachedArtists() *ArtistsResponse {
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	return artistsCacheFull
}

func SetCachedArtists(a *ArtistsResponse) {
	cacheMu.Lock()
	artistsCacheFull = a
	cacheMu.Unlock()
}

func WarmLibraryCache() {
	tracks, err := GetLibraryTracks()
	if err != nil {
		return
	}
	a := BuildAlbums(tracks, 0, 999999)
	ar := BuildArtists(tracks, 0, 999999)
	SetCachedAlbums(&a)
	SetCachedArtists(&ar)
}

func RefreshLibraryCacheLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		InvalidateLibraryCache()
		tracks, err := GetLibraryTracks()
		if err != nil {
			continue
		}
		a := BuildAlbums(tracks, 0, 999999)
		ar := BuildArtists(tracks, 0, 999999)
		SetCachedAlbums(&a)
		SetCachedArtists(&ar)
	}
}

func TrackSortKey(t LibraryTrack) int {
	disc := t.DiscNumber
	if disc == 0 {
		disc = 1
	}
	return disc*10000 + t.TrackNumber
}

func FilterArtistTracks(tracks []LibraryTrack, artistName string) []map[string]any {
	var results []map[string]any
	for _, t := range tracks {
		albumArtist := t.AlbumArtist
		if albumArtist == "" {
			albumArtist = t.Artist
		}
		artist := t.Artist
		if albumArtist != artistName && artist != artistName {
			continue
		}
		results = append(results, map[string]any{
			"id":           t.ID,
			"name":         t.Name,
			"artist":       t.Artist,
			"album":        t.Album,
			"track_number": t.TrackNumber,
			"disc_number":  t.DiscNumber,
			"duration":     t.Duration,
		})
	}
	sort.Slice(results, func(i, j int) bool {
		aAlbum, _ := results[i]["album"].(string)
		bAlbum, _ := results[j]["album"].(string)
		if aAlbum != bAlbum {
			return aAlbum < bAlbum
		}
		ad, _ := results[i]["disc_number"].(int)
		bd, _ := results[j]["disc_number"].(int)
		if ad != bd {
			return ad < bd
		}
		at, _ := results[i]["track_number"].(int)
		bt, _ := results[j]["track_number"].(int)
		return at < bt
	})
	return results
}

func FilterAlbumTracks(tracks []LibraryTrack, artistName, albumName string) []map[string]any {
	var results []map[string]any
	for _, t := range tracks {
		if t.Album != albumName {
			continue
		}
		albumArtist := t.AlbumArtist
		if albumArtist == "" {
			albumArtist = t.Artist
		}
		artist := t.Artist
		if albumArtist != artistName && artist != artistName {
			continue
		}
		results = append(results, map[string]any{
			"id":           t.ID,
			"name":         t.Name,
			"artist":       t.Artist,
			"album":        t.Album,
			"track_number": t.TrackNumber,
			"duration":     t.Duration,
			"disc_number":  t.DiscNumber,
		})
	}
	sort.Slice(results, func(i, j int) bool {
		ad, _ := results[i]["disc_number"].(int)
		bd, _ := results[j]["disc_number"].(int)
		if ad != bd {
			return ad < bd
		}
		at, _ := results[i]["track_number"].(int)
		bt, _ := results[j]["track_number"].(int)
		return at < bt
	})
	return results
}

func SearchTracks(tracks []LibraryTrack, query string) []map[string]any {
	q := strings.ToLower(query)
	var results []map[string]any
	for _, t := range tracks {
		if len(results) >= 50 {
			break
		}
		if !strings.Contains(strings.ToLower(t.Name), q) {
			continue
		}
		results = append(results, map[string]any{
			"id":     t.ID,
			"name":   t.Name,
			"artist": t.Artist,
			"album":  t.Album,
		})
	}
	return results
}

func CollectAlbumIDs(tracks []LibraryTrack, artist, album string) []string {
	type item struct {
		id  string
		key int
	}
	var items []item
	for _, t := range tracks {
		if t.Album != album {
			continue
		}
		aa := t.AlbumArtist
		if aa == "" {
			aa = t.Artist
		}
		if aa != artist {
			continue
		}
		items = append(items, item{t.ID, TrackSortKey(t)})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].key < items[j].key })
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.id
	}
	return ids
}

func CollectArtistIDs(tracks []LibraryTrack, artist string) []string {
	type item struct {
		id    string
		album string
		key   int
	}
	var items []item
	for _, t := range tracks {
		aa := t.AlbumArtist
		if aa == "" {
			aa = t.Artist
		}
		if aa != artist {
			continue
		}
		items = append(items, item{t.ID, t.Album, TrackSortKey(t)})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].album < items[j].album {
			return true
		}
		if items[i].album > items[j].album {
			return false
		}
		return items[i].key < items[j].key
	})
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.id
	}
	return ids
}
