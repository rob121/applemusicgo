package music

import (
	"encoding/json"
	"fmt"
	"strings"
)

func transport(cmd string) error {
	script := fmt.Sprintf(`
var app;
try { app = Application("Music"); } catch(e) { app = Application("iTunes"); }
app.%s();
`, cmd)
	_, err := RunJXA(script)
	return err
}

func Play() error     { return transport("play") }
func Pause() error    { return transport("pause") }
func PlayPause() error { return transport("playpause") }
func Stop() error     { return transport("stop") }
func Previous() error { return transport("backTrack") }
func Next() error     { return transport("nextTrack") }

func SeekToPosition(position string) error {
	script := fmt.Sprintf(`
var itunes;
try { itunes = Application("Music"); } catch (error) { itunes = Application("iTunes"); }
itunes.playerPosition = parseFloat(%q);
`, position)
	_, err := RunJXA(script)
	return err
}

func SetVolume(level string) (bool, error) {
	if level == "" {
		return false, nil
	}
	script := fmt.Sprintf(`
var itunes;
try { itunes = Application("Music"); } catch (error) { itunes = Application("iTunes"); }
itunes.soundVolume = parseInt(%q);
`, level)
	_, err := RunJXA(script)
	return err == nil, err
}

func SetMuted(muted string) (bool, error) {
	if muted == "" {
		return false, nil
	}
	script := fmt.Sprintf(`
var itunes;
try { itunes = Application("Music"); } catch (error) { itunes = Application("iTunes"); }
itunes.mute = %s;
`, muted)
	_, err := RunJXA(script)
	return err == nil, err
}

func SetShuffle(mode string) (bool, error) {
	if mode == "" {
		mode = "songs"
	}
	var script string
	if mode == "false" || mode == "off" {
		script = `
var itunes;
try { itunes = Application("Music"); } catch (error) { itunes = Application("iTunes"); }
itunes.shuffleEnabled = false;
`
		_, err := RunJXA(script)
		return false, err
	}
	script = fmt.Sprintf(`
var itunes;
try { itunes = Application("Music"); } catch (error) { itunes = Application("iTunes"); }
itunes.shuffleEnabled = true;
itunes.shuffleMode = %q;
`, mode)
	_, err := RunJXA(script)
	return err == nil, err
}

func SetRepeat(mode string) (bool, error) {
	if mode == "" {
		mode = "all"
	}
	var script string
	if mode == "false" || mode == "off" {
		script = `
var itunes;
try { itunes = Application("Music"); } catch (error) { itunes = Application("iTunes"); }
itunes.songRepeat = false;
`
		_, err := RunJXA(script)
		return false, err
	}
	script = fmt.Sprintf(`
var itunes;
try { itunes = Application("Music"); } catch (error) { itunes = Application("iTunes"); }
itunes.songRepeat = %q;
`, mode)
	_, err := RunJXA(script)
	return err == nil, err
}

// TrackIDKind classifies an id from search or library APIs.
type TrackIDKind int

const (
	TrackIDUnknown TrackIDKind = iota
	TrackIDLibrary
	TrackIDCatalog
)

// ResolveTrackIDKind decides how PlayTrackByID should handle an id.
// 16-character hex ids are library persistent IDs; numeric ids are catalog track ids.
func ResolveTrackIDKind(id string) TrackIDKind {
	id = strings.TrimSpace(id)
	switch {
	case IsPersistentTrackID(id):
		return TrackIDLibrary
	case IsCatalogTrackID(id):
		return TrackIDCatalog
	default:
		return TrackIDUnknown
	}
}

// PlayTrackByID plays a track by id only. Library ids (16-char hex) play from the local
// library; catalog ids (numeric, e.g. from GET /music/search) resolve metadata via the
// iTunes Lookup API and play through Apple Music subscription.
func PlayTrackByID(id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, fmt.Errorf("id is required")
	}

	switch ResolveTrackIDKind(id) {
	case TrackIDLibrary:
		return playLibraryTrackByPersistentID(id)
	case TrackIDCatalog:
		played, err := PlayCatalogTrack(id)
		if err != nil {
			return false, err
		}
		return played, nil
	default:
		// Non-standard id: try library first, then catalog lookup.
		if played, err := playLibraryTrackByPersistentID(id); played || err != nil {
			return played, err
		}
		if IsCatalogTrackID(id) {
			played, err := PlayCatalogTrack(id)
			if err != nil {
				return false, err
			}
			return played, nil
		}
		return false, nil
	}
}

func playLibraryTrackByPersistentID(persistentID string) (bool, error) {
	// Play from library playlist 1 by track index (no temp playlist duplicate).
	// PlayByIDs duplicates tracks and can trigger download error 3048 for
	// purchased/cloud items.
	indexScript := fmt.Sprintf(`
var music;
try { music = Application("Music"); } catch(e) { music = Application("iTunes"); }
var id = %q;
var lib = music.libraryPlaylists[0];
var pids = lib.tracks.persistentID();
var result = "0";
for (var i = 0; i < pids.length; i++) {
  if (pids[i] === id) { result = String(i + 1); break; }
}
result;
`, persistentID)
	idxOut, err := RunJXA(indexScript)
	if err != nil {
		return false, err
	}
	idx := strings.TrimSpace(idxOut)
	if idx == "0" || idx == "" {
		return false, nil
	}
	appName := "Music"
	playScript := fmt.Sprintf(`tell application %q to play track %s of library playlist 1`, appName, idx)
	if _, err := RunAppleScript(playScript); err != nil {
		appName = "iTunes"
		playScript = fmt.Sprintf(`tell application %q to play track %s of library playlist 1`, appName, idx)
		if _, err = RunAppleScript(playScript); err != nil {
			return false, err
		}
	}
	return true, nil
}

func PlayByIDs(playlistName string, ids []string) error {
	idsJSON, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	script := fmt.Sprintf(`var music;
try { music = Application("Music"); } catch(e) { music = Application("iTunes"); }
var playlistName = %q;
var persistentIDs = %s;
var playlists = music.userPlaylists();
for (var i = 0; i < playlists.length; i++) {
  if (playlists[i].name() === playlistName) { playlists[i].delete(); break; }
}
var tempPL = music.make({ new: "userPlaylist", withProperties: { name: playlistName } });
for (var j = 0; j < persistentIDs.length; j++) {
  try {
    var matches = music.tracks.whose({ persistentID: { "=": persistentIDs[j] } });
    if (matches.length > 0) { matches[0].duplicate({ to: tempPL }); }
  } catch(e) {}
}
music.play(tempPL);
`, playlistName, string(idsJSON))
	return RunJXAFromTemp(script)
}
