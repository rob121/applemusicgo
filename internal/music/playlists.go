package music

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const listPlaylistsJXA = `
var music;
try { music = Application("Music"); } catch(e) { music = Application("iTunes"); }
var playlists = music.userPlaylists();
var lines = [];
for (var i = 0; i < playlists.length; i++) {
  var p = playlists[i];
  lines.push(String(p.id()) + "\t" + p.name());
}
lines.join("\n");
`

type Playlist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func GetPlaylists() ([]Playlist, error) {
	out, err := RunJXA(listPlaylistsJXA)
	if err != nil {
		return nil, err
	}
	return parsePlaylistLines(out), nil
}

func parsePlaylistLines(stdout string) []Playlist {
	var playlists []Playlist
	lines := splitLines(stdout)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		tab := strings.Index(line, "\t")
		if tab == -1 {
			continue
		}
		id := strings.TrimSpace(line[:tab])
		name := strings.TrimSpace(line[tab+1:])
		if id != "" && name != "" {
			playlists = append(playlists, Playlist{ID: id, Name: name})
		}
	}
	return playlists
}

func splitLines(s string) []string {
	re := regexp.MustCompile(`\r\n|\r|\n`)
	return re.Split(s, -1)
}

func PlayPlaylist(idOrSlug string) error {
	if matched, _ := regexp.MatchString(`^\d+$`, idOrSlug); matched {
		return playPlaylistByID(idOrSlug)
	}
	playlists, err := GetPlaylists()
	if err != nil {
		return err
	}
	var matchID string
	for _, p := range playlists {
		if Slugify(p.Name) == idOrSlug || p.Name == idOrSlug {
			matchID = p.ID
			break
		}
	}
	if matchID == "" {
		return ErrNotFound
	}
	return playPlaylistByID(matchID)
}

var ErrNotFound = fmt.Errorf("not found")

func playPlaylistByID(id string) error {
	script := fmt.Sprintf(`
var music;
try { music = Application("Music"); } catch(e) { music = Application("iTunes"); }
var targetID = %q;
var playlists = music.userPlaylists();
var result = "notfound";
for (var i = 0; i < playlists.length; i++) {
  if (String(playlists[i].id()) === targetID) {
    music.play(playlists[i]);
    result = "ok";
    break;
  }
}
result;
`, id)
	out, err := RunJXA(script)
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) == "notfound" {
		return ErrNotFound
	}
	return nil
}

type playlistTrackRef struct {
	Artist string `json:"artist"`
	Album  string `json:"album"`
}

func GetPlaylistTracks(playlistName string) ([]playlistTrackRef, error) {
	script := fmt.Sprintf(`try { var itunes = Application("Music"); } catch(e) { var itunes = Application("iTunes"); } var name = %q; var playlists = itunes.playlists(); var results = []; for (var i = 0; i < playlists.length; i++) { var p = playlists[i]; if (p.name() === name) { var tracks = p.tracks(); for (var j = 0; j < tracks.length; j++) { var t = tracks[j]; results.push({ artist: t.albumArtist() || t.artist() || "", album: t.album() || "" }); } break; } } JSON.stringify(results);`, playlistName)
	out, err := RunJXAWithBuffer(script, 10*1024*1024)
	if err != nil {
		return nil, err
	}
	var refs []playlistTrackRef
	if err := json.Unmarshal([]byte(out), &refs); err != nil {
		return nil, err
	}
	return refs, nil
}
