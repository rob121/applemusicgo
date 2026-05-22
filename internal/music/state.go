package music

import (
	"encoding/json"
	"fmt"
	"time"
)

type PlayerState map[string]any

const getCurrentStateJXA = `
var itunes;
try { itunes = Application("Music"); } catch (error) { itunes = Application("iTunes"); }
var currentState = {};
try { currentState["player_state"] = itunes.playerState(); } catch (e) { currentState["player_state"] = "stopped"; }
var playerState = currentState["player_state"];
if (playerState != "stopped") {
  try {
    var currentTrack = itunes.currentTrack;
    try { currentState["id"] = currentTrack.persistentID(); } catch (e) {}
    try { currentState["name"] = currentTrack.name(); } catch (e) {}
    try { currentState["artist"] = currentTrack.artist(); } catch (e) {}
    try { currentState["album"] = currentTrack.album(); } catch (e) {}
    try { currentState["volume"] = itunes.soundVolume(); } catch (e) {}
    try { currentState["muted"] = itunes.mute(); } catch (e) {}
    try { currentState["repeat"] = itunes.songRepeat(); } catch (e) {}
    try { currentState["shuffle"] = itunes.shuffleEnabled() && itunes.shuffleMode(); } catch (e) {}
    try { currentState["player_position"] = itunes.playerPosition(); } catch (e) {}
    try { currentState["player_duration"] = currentTrack.duration(); } catch (e) {}
    currentState["position_timestamp"] = Date.now();
    try {
      var year = currentTrack.year();
      if (year && currentState["album"]) {
        currentState["album"] += " (" + year + ")";
      }
    } catch (e) {}
    try {
      currentState["playlist"] = itunes.currentPlaylist.name();
    } catch (e) {
      currentState["playlist"] = "";
    }
    if (!currentState["name"]) {
      currentState["player_state"] = "stopped";
    }
  } catch (e) {
    currentState["player_state"] = "stopped";
  }
}
JSON.stringify(currentState);
`

func GetCurrentState() (PlayerState, error) {
	out, err := RunJXA(getCurrentStateJXA)
	if err != nil {
		if IsNotRunning(err) || IsObjectError(err) {
			return PlayerState{"player_state": "stopped"}, nil
		}
		return nil, err
	}
	var state PlayerState
	if err := json.Unmarshal([]byte(out), &state); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	return state, nil
}

func StoppedState() PlayerState {
	return PlayerState{"player_state": "stopped"}
}

func NotificationUpdate(playerState, id, name, artist, album string, totalTimeMs float64) PlayerState {
	return PlayerState{
		"player_state":       playerState,
		"id":                 id,
		"name":               name,
		"artist":             artist,
		"album":              album,
		"player_duration":    totalTimeMs / 1000,
		"position_timestamp": time.Now().UnixMilli(),
		"_from_notification": true,
	}
}
