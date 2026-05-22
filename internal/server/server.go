package server

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rob121/applemusicgo/internal/music"
)

type Server struct {
	BaseDir string
	Addr    string
	sse     sseHub
}

func New(baseDir, addr string) *Server {
	music.EnsureArtworkDirs(baseDir)
	if os.Getenv("APPLEMUSICGO_LIB") == "" {
		_ = os.Setenv("APPLEMUSICGO_LIB", filepath.Join(baseDir, "lib"))
	}
	return &Server{BaseDir: baseDir, Addr: addr}
}

func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	go music.WarmLibraryCache()
	go music.RefreshLibraryCacheLoop(30 * time.Minute)

	log.Printf("listening on %s", s.Addr)
	return http.ListenAndServe(s.Addr, corsMiddleware(s.logMiddleware(mux)))
}

// corsMiddleware allows Swagger UI and other browser clients to call the API
// (including when host is 127.0.0.1 vs localhost, or a forwarded port).
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		log.Printf("%s - %s %s %d %dms",
			r.RemoteAddr, r.Method, r.URL.Path, rw.status, time.Since(start).Milliseconds())
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	registerSwaggerRoutes(mux)
	mux.HandleFunc("GET /_ping", s.handlePing)
	mux.HandleFunc("GET /events", s.handleEvents)
	mux.HandleFunc("POST /notify", s.handleNotify)
	mux.HandleFunc("PUT /play", s.handlePlay)
	mux.HandleFunc("PUT /pause", s.handlePause)
	mux.HandleFunc("PUT /playpause", s.handlePlayPause)
	mux.HandleFunc("PUT /stop", s.handleStop)
	mux.HandleFunc("PUT /previous", s.handlePrevious)
	mux.HandleFunc("PUT /next", s.handleNext)
	mux.HandleFunc("PUT /volume", s.handleVolume)
	mux.HandleFunc("PUT /mute", s.handleMute)
	mux.HandleFunc("PUT /shuffle", s.handleShuffle)
	mux.HandleFunc("PUT /repeat", s.handleRepeat)
	mux.HandleFunc("PUT /seek", s.handleSeek)
	mux.HandleFunc("GET /now_playing", s.handleNowPlaying)
	mux.HandleFunc("GET /artwork", s.handleArtworkNow)
	mux.Handle("/artwork-cache/", cacheHeaders(http.StripPrefix("/artwork-cache", http.FileServer(http.Dir(music.ArtworkDir(s.BaseDir))))))
	mux.Handle("/custom-artwork/", cacheHeaders(http.StripPrefix("/custom-artwork", http.FileServer(http.Dir(music.CustomArtworkDir(s.BaseDir))))))
	mux.HandleFunc("GET /artwork-static/{file}", s.handleArtworkStatic)
	mux.HandleFunc("GET /artwork/playlist/{name}", s.handleArtworkPlaylist)
	mux.HandleFunc("GET /artwork/artist/{artist}", s.handleArtworkArtist)
	mux.HandleFunc("GET /artwork/{artist}/{album}", s.handleArtworkAlbum)
	mux.HandleFunc("GET /debug/artwork-slugs", s.handleDebugArtworkSlugs)
	mux.HandleFunc("GET /playlists", s.handlePlaylists)
	mux.HandleFunc("PUT /playlists/{id}/play", s.handlePlayPlaylist)
	mux.HandleFunc("GET /library/artists", s.handleLibraryArtists)
	mux.HandleFunc("GET /library/artists/{artist}/albums", s.handleLibraryArtistAlbums)
	mux.HandleFunc("GET /library/artists/{artist}/tracks", s.handleLibraryArtistTracks)
	mux.HandleFunc("GET /library/albums", s.handleLibraryAlbums)
	mux.HandleFunc("GET /library/albums/{artist}/{album}/tracks", s.handleLibraryAlbumTracks)
	mux.HandleFunc("PUT /library/tracks/{id}/play", s.handleLibraryTrackPlay)
	mux.HandleFunc("PUT /library/albums/{artist}/{album}/play", s.handleLibraryAlbumPlay)
	mux.HandleFunc("PUT /library/artists/{artist}/play", s.handleLibraryArtistPlay)
	mux.HandleFunc("GET /library/search", s.handleLibrarySearch)
	mux.HandleFunc("GET /music/search", s.handleMusicSearch)
	mux.HandleFunc("GET /airplay_devices", s.handleAirPlayList)
	mux.HandleFunc("GET /airplay_devices/{id}", s.handleAirPlayGet)
	mux.HandleFunc("PUT /airplay_devices/{id}/on", s.handleAirPlayOn)
	mux.HandleFunc("PUT /airplay_devices/{id}/off", s.handleAirPlayOff)
	mux.HandleFunc("PUT /airplay_devices/{id}/volume", s.handleAirPlayVolume)
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("OK"))
}

func (s *Server) sendResponse(w http.ResponseWriter, err error) {
	if err != nil {
		log.Println(err)
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	state, err := music.GetCurrentState()
	if err != nil {
		if music.IsNotRunning(err) {
			writeJSON(w, music.StoppedState())
			return
		}
		log.Println(err)
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	writeJSON(w, state)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.Encode(v)
}

func parseForm(r *http.Request) error {
	if r.Body != nil {
		defer r.Body.Close()
	}
	return r.ParseForm()
}

func (s *Server) handlePlay(w http.ResponseWriter, r *http.Request) {
	s.sendResponse(w, music.Play())
}

func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	s.sendResponse(w, music.Pause())
}

func (s *Server) handlePlayPause(w http.ResponseWriter, r *http.Request) {
	s.sendResponse(w, music.PlayPause())
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	s.sendResponse(w, music.Stop())
}

func (s *Server) handlePrevious(w http.ResponseWriter, r *http.Request) {
	s.sendResponse(w, music.Previous())
}

func (s *Server) handleNext(w http.ResponseWriter, r *http.Request) {
	s.sendResponse(w, music.Next())
}

func (s *Server) handleVolume(w http.ResponseWriter, r *http.Request) {
	_ = parseForm(r)
	if _, err := music.SetVolume(r.FormValue("level")); err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	s.sendResponse(w, nil)
}

func (s *Server) handleMute(w http.ResponseWriter, r *http.Request) {
	_ = parseForm(r)
	if _, err := music.SetMuted(r.FormValue("muted")); err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	s.sendResponse(w, nil)
}

func (s *Server) handleShuffle(w http.ResponseWriter, r *http.Request) {
	_ = parseForm(r)
	if _, err := music.SetShuffle(r.FormValue("mode")); err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	s.sendResponse(w, nil)
}

func (s *Server) handleRepeat(w http.ResponseWriter, r *http.Request) {
	_ = parseForm(r)
	if _, err := music.SetRepeat(r.FormValue("mode")); err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	s.sendResponse(w, nil)
}

func (s *Server) handleSeek(w http.ResponseWriter, r *http.Request) {
	_ = parseForm(r)
	pos := r.FormValue("position")
	if pos == "" {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]string{"error": "position is required"})
		return
	}
	if err := music.SeekToPosition(pos); err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	s.sendResponse(w, nil)
}

func (s *Server) handleNowPlaying(w http.ResponseWriter, r *http.Request) {
	s.sendResponse(w, nil)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	flusher.Flush()
	s.sse.addClient(w)
	<-r.Context().Done()
	s.sse.removeClient(w)
}

type notification struct {
	PlayerState  string  `json:"player_state"`
	PersistentID string  `json:"persistent_id"`
	Name         string  `json:"name"`
	Artist       string  `json:"artist"`
	Album        string  `json:"album"`
	TotalTime    float64 `json:"total_time"`
}

func (s *Server) handleNotify(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "", http.StatusBadRequest)
		return
	}
	var n notification
	if err := json.Unmarshal(body, &n); err != nil {
		http.Error(w, "", http.StatusBadRequest)
		return
	}
	state := strings.ToLower(n.PlayerState)
	playerState := "stopped"
	switch state {
	case "playing":
		playerState = "playing"
	case "paused":
		playerState = "paused"
	}
	update := music.NotificationUpdate(playerState, n.PersistentID, n.Name, n.Artist, n.Album, n.TotalTime)
	s.sse.setState(update)
	s.sse.broadcast(update)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleArtworkNow(w http.ResponseWriter, r *http.Request) {
	if err := music.FetchNowPlayingArtwork(); err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, music.NowPlayingArtworkPath())
}

func cacheHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleArtworkStatic(w http.ResponseWriter, r *http.Request) {
	file := r.PathValue("file")
	exts := []string{".jpg", ".jpeg", ".png"}
	base := file
	for _, ext := range exts {
		if strings.HasSuffix(strings.ToLower(base), ext) {
			base = strings.TrimSuffix(base, ext)
			break
		}
	}
	slug := strings.TrimPrefix(base, "playlist-")
	slug = strings.TrimPrefix(slug, "artist-")

	customDir := music.CustomArtworkDir(s.BaseDir)
	for _, ext := range exts {
		p := filepath.Join(customDir, slug+ext)
		if _, err := os.Stat(p); err == nil {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			http.ServeFile(w, r, p)
			return
		}
	}
	cached := filepath.Join(music.ArtworkDir(s.BaseDir), base+".jpg")
	if _, err := os.Stat(cached); err == nil {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		http.ServeFile(w, r, cached)
		return
	}
	http.Error(w, "", http.StatusNotFound)
}

func (s *Server) handleArtworkPlaylist(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if p := music.CustomArtworkPath(s.BaseDir, name); p != "" {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		http.ServeFile(w, r, p)
		return
	}
	path, err := music.BuildPlaylistCollage(s.BaseDir, name)
	if err != nil {
		http.Error(w, "", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeFile(w, r, path)
}

func (s *Server) handleArtworkArtist(w http.ResponseWriter, r *http.Request) {
	artist := r.PathValue("artist")
	if p := music.CustomArtworkPath(s.BaseDir, artist); p != "" {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		http.ServeFile(w, r, p)
		return
	}
	cacheFile := filepath.Join(music.ArtworkDir(s.BaseDir), "artist-"+music.Slugify(artist)+".jpg")
	if _, err := os.Stat(cacheFile); err == nil {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		http.ServeFile(w, r, cacheFile)
		return
	}
	tracks, err := music.GetLibraryTracks()
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	for _, t := range tracks {
		a := t.AlbumArtist
		if a == "" {
			a = t.Artist
		}
		album := t.Album
		if a != artist || album == "" {
			continue
		}
		file := music.ArtworkFilePath(s.BaseDir, a, album)
		if _, err := os.Stat(file); err != nil {
			continue
		}
		go music.CopyArtistCache(s.BaseDir, artist, file)
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		http.ServeFile(w, r, file)
		return
	}
	http.Error(w, "", http.StatusNotFound)
}

func (s *Server) handleArtworkAlbum(w http.ResponseWriter, r *http.Request) {
	artist := r.PathValue("artist")
	album := r.PathValue("album")
	if p := music.CustomArtworkPath(s.BaseDir, album); p != "" {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		http.ServeFile(w, r, p)
		return
	}
	filePath := music.ArtworkFilePath(s.BaseDir, artist, album)
	if _, err := os.Stat(filePath); err == nil {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		http.ServeFile(w, r, filePath)
		return
	}
	saved, err := music.FetchAndSaveArtwork(s.BaseDir, artist, album)
	if err != nil {
		http.Error(w, "", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeFile(w, r, saved)
}

func (s *Server) handleDebugArtworkSlugs(w http.ResponseWriter, r *http.Request) {
	playlists, err := music.GetPlaylists()
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	var result []map[string]any
	for _, p := range playlists {
		slug := music.Slugify(p.Name)
		custom := music.CustomArtworkPath(s.BaseDir, p.Name) != ""
		result = append(result, map[string]any{
			"name":         p.Name,
			"slug":         slug,
			"filename":     slug + ".jpg",
			"custom_found": custom,
		})
	}
	writeJSON(w, result)
}

func (s *Server) handlePlaylists(w http.ResponseWriter, r *http.Request) {
	playlists, err := music.GetPlaylists()
	if err != nil {
		log.Printf("get-playlists error: %v", err)
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"playlists": playlists})
}

func (s *Server) handlePlayPlaylist(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := music.PlayPlaylist(id); err != nil {
		if errors.Is(err, music.ErrNotFound) {
			http.Error(w, "", http.StatusNotFound)
			return
		}
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	s.sendResponse(w, nil)
}

func queryInt(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func (s *Server) handleLibraryArtists(w http.ResponseWriter, r *http.Request) {
	offset := queryInt(r, "offset", 0)
	limit := queryInt(r, "limit", 100)
	tracks, err := music.GetLibraryTracks()
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	cached := music.GetCachedArtists()
	if cached == nil {
		a := music.BuildArtists(tracks, 0, 999999)
		music.SetCachedArtists(&a)
		cached = &a
	}
	writeJSON(w, map[string]any{
		"total":   cached.Total,
		"offset":  offset,
		"limit":   limit,
		"artists": sliceArtists(cached.Artists, offset, limit),
	})
}

func sliceArtists(all []music.ArtistEntry, offset, limit int) []music.ArtistEntry {
	end := offset + limit
	if offset > len(all) {
		offset = len(all)
	}
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end]
}

func sliceAlbums(all []music.AlbumEntry, offset, limit int) []music.AlbumEntry {
	end := offset + limit
	if offset > len(all) {
		offset = len(all)
	}
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end]
}

func (s *Server) handleLibraryArtistAlbums(w http.ResponseWriter, r *http.Request) {
	tracks, err := music.GetLibraryTracks()
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	writeJSON(w, music.BuildAlbumsByArtist(tracks, r.PathValue("artist")))
}

func (s *Server) handleLibraryArtistTracks(w http.ResponseWriter, r *http.Request) {
	tracks, err := music.GetLibraryTracks()
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	artist := r.PathValue("artist")
	writeJSON(w, map[string]any{
		"artist": artist,
		"tracks": music.FilterArtistTracks(tracks, artist),
	})
}

func (s *Server) handleLibraryAlbums(w http.ResponseWriter, r *http.Request) {
	offset := queryInt(r, "offset", 0)
	limit := queryInt(r, "limit", 50)
	tracks, err := music.GetLibraryTracks()
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	cached := music.GetCachedAlbums()
	if cached == nil {
		a := music.BuildAlbums(tracks, 0, 999999)
		music.SetCachedAlbums(&a)
		cached = &a
	}
	writeJSON(w, map[string]any{
		"total":  cached.Total,
		"offset": offset,
		"limit":  limit,
		"albums": sliceAlbums(cached.Albums, offset, limit),
	})
}

func (s *Server) handleLibraryAlbumTracks(w http.ResponseWriter, r *http.Request) {
	tracks, err := music.GetLibraryTracks()
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	artist := r.PathValue("artist")
	album := r.PathValue("album")
	writeJSON(w, map[string]any{
		"artist": artist,
		"album":  album,
		"tracks": music.FilterAlbumTracks(tracks, artist, album),
	})
}

func (s *Server) handleLibraryTrackPlay(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	played, err := music.PlayTrackByID(id)
	if err != nil {
		if errors.Is(err, music.ErrCatalogPlayFailed) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			writeJSON(w, map[string]any{
				"error":   "catalog track could not be played",
				"id":      id,
				"hint":    catalogPlayHint(),
			})
			return
		}
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	if !played {
		if music.IsCatalogTrackID(id) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			writeJSON(w, map[string]any{
				"error": "catalog track could not be played",
				"id":    id,
				"hint":  catalogPlayHint(),
			})
			return
		}
		http.Error(w, "", http.StatusNotFound)
		return
	}
	s.sendResponse(w, nil)
}

func catalogPlayHint() string {
	return "Catalog play needs Music.app signed into Apple Music and Accessibility enabled for the process running applemusicgo (Terminal, Cursor, etc.). Keep Music.app visible; first play may take up to ~30s while the track is added to your library."
}

func (s *Server) handleLibraryAlbumPlay(w http.ResponseWriter, r *http.Request) {
	tracks, err := music.GetLibraryTracks()
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	ids := music.CollectAlbumIDs(tracks, r.PathValue("artist"), r.PathValue("album"))
	if len(ids) == 0 {
		http.Error(w, "", http.StatusNotFound)
		return
	}
	if err := music.PlayByIDs("HA_Play_Album", ids); err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	s.sendResponse(w, nil)
}

func (s *Server) handleLibraryArtistPlay(w http.ResponseWriter, r *http.Request) {
	tracks, err := music.GetLibraryTracks()
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	ids := music.CollectArtistIDs(tracks, r.PathValue("artist"))
	if len(ids) == 0 {
		http.Error(w, "", http.StatusNotFound)
		return
	}
	if err := music.PlayByIDs("HA_Play_Artist", ids); err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	s.sendResponse(w, nil)
}

func (s *Server) handleLibrarySearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]string{"error": "q parameter is required"})
		return
	}
	tracks, err := music.GetLibraryTracks()
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"query":  query,
		"tracks": music.SearchTracks(tracks, query),
	})
}

func (s *Server) handleMusicSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]string{"error": "q parameter is required"})
		return
	}
	limit := queryInt(r, "limit", music.DefaultSearchLimit())
	result, err := music.SearchMusic(query, limit)
	if err != nil {
		log.Printf("music search error: %v", err)
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleAirPlayList(w http.ResponseWriter, r *http.Request) {
	devices, err := music.ListAirPlayDevices()
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	if devices == nil {
		devices = []music.AirPlayDevice{}
	}
	writeJSON(w, map[string]any{"airplay_devices": devices})
}

func (s *Server) handleAirPlayGet(w http.ResponseWriter, r *http.Request) {
	devices, err := music.ListAirPlayDevices()
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	for _, d := range devices {
		if d["id"] == r.PathValue("id") {
			writeJSON(w, d)
			return
		}
	}
	http.Error(w, "", http.StatusNotFound)
}

func (s *Server) handleAirPlayOn(w http.ResponseWriter, r *http.Request) {
	if err := music.SetAirPlaySelection(r.PathValue("id"), true); err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	s.sendResponse(w, nil)
}

func (s *Server) handleAirPlayOff(w http.ResponseWriter, r *http.Request) {
	if err := music.SetAirPlaySelection(r.PathValue("id"), false); err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	s.sendResponse(w, nil)
}

func (s *Server) handleAirPlayVolume(w http.ResponseWriter, r *http.Request) {
	_ = parseForm(r)
	level, _ := strconv.Atoi(r.FormValue("level"))
	if err := music.SetAirPlayVolume(r.PathValue("id"), level); err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	s.sendResponse(w, nil)
}
