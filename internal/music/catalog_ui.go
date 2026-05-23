package music

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var albumPathIDRe = regexp.MustCompile(`/album/[^/]+/(\d+)`)

func catalogITMSURL(collectionID, trackID int64) string {
	if collectionID <= 0 {
		return ""
	}
	if trackID > 0 {
		return fmt.Sprintf(
			"itmss://geo.music.apple.com/us/album/%d?i=%d&ls=1&app=music",
			collectionID, trackID,
		)
	}
	return fmt.Sprintf("itmss://geo.music.apple.com/us/album/%d?ls=1&app=music", collectionID)
}

func collectionIDFromTrackURL(trackViewURL string) int64 {
	m := albumPathIDRe.FindStringSubmatch(trackViewURL)
	if len(m) < 2 {
		return 0
	}
	id, _ := strconv.ParseInt(m[1], 10, 64)
	return id
}

// clickMusicUIButton clicks a Music.app control by accessibility description (requires Accessibility permission).
func clickMusicUIButton(description string) (bool, error) {
	return clickMusicUIButtonAny(description)
}

func clickMusicUIButtonAny(descriptions ...string) (bool, error) {
	if len(descriptions) == 0 {
		return false, nil
	}
	descJSON, err := json.Marshal(descriptions)
	if err != nil {
		return false, err
	}
	script := fmt.Sprintf(`
var se = Application("System Events");
var proc = se.processes.byName("Music");
proc.frontmost = true;
delay(0.3);
var w = proc.windows[0];
var descs = %s;
function matches(desc) {
  desc = String(desc || "").toLowerCase();
  for (var i = 0; i < descs.length; i++) {
    if (desc === String(descs[i]).toLowerCase()) return true;
  }
  return false;
}
function find(el) {
  try {
    if (el.role() === "AXButton" && matches(el.description())) return el;
  } catch(e) {}
  try {
    var kids = el.uiElements();
    for (var i = 0; i < kids.length; i++) {
      var f = find(kids[i]);
      if (f) return f;
    }
  } catch(e) {}
  return null;
}
var btn = find(w);
if (!btn) { "0"; } else { btn.click(); "1"; }
`, string(descJSON))
	out, err := RunJXA(script)
	if err != nil {
		if strings.Contains(err.Error(), "1002") || strings.Contains(err.Error(), "not allowed") {
			return false, nil
		}
		return false, err
	}
	return strings.TrimSpace(out) == "1", nil
}

func clickMusicTrackRow(trackName string) (bool, error) {
	return selectMusicTrackRow(trackName)
}

func selectMusicTrackRow(trackName string) (bool, error) {
	needle := normalizeTrackTitle(trackName)
	if needle == "" {
		return false, nil
	}
	script := fmt.Sprintf(`
var se = Application("System Events");
var proc = se.processes.byName("Music");
proc.frontmost = true;
delay(0.15);
var w = proc.windows[0];
var needle = %q;
function walk(el) {
  try {
    var v = el.value();
    if (v && String(v).toLowerCase().indexOf(needle) >= 0) {
      el.click();
      return true;
    }
  } catch(e) {}
  try {
    var kids = el.uiElements();
    for (var i = 0; i < kids.length; i++) {
      if (walk(kids[i])) return true;
    }
  } catch(e) {}
  return false;
}
walk(w) ? "1" : "0";
`, needle)
	out, err := RunJXA(script)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "1", nil
}

func doubleClickMusicTrackRow(trackName string) (bool, error) {
	needle := normalizeTrackTitle(trackName)
	if needle == "" {
		return false, nil
	}
	script := fmt.Sprintf(`
var se = Application("System Events");
var proc = se.processes.byName("Music");
proc.frontmost = true;
delay(0.15);
var w = proc.windows[0];
var needle = %q;
function pressTwice(el) {
  el.performAction("AXPress");
  delay(0.08);
  el.performAction("AXPress");
}
function walk(el) {
  try {
    var v = el.value();
    if (v && String(v).toLowerCase().indexOf(needle) >= 0) {
      pressTwice(el);
      return true;
    }
  } catch(e) {}
  try {
    var kids = el.uiElements();
    for (var i = 0; i < kids.length; i++) {
      if (walk(kids[i])) return true;
    }
  } catch(e) {}
  return false;
}
walk(w) ? "1" : "0";
`, needle)
	out, err := RunJXA(script)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "1", nil
}

func pressMusicKey(keyCode int) (bool, error) {
	script := fmt.Sprintf(`
var se = Application("System Events");
var proc = se.processes.byName("Music");
proc.frontmost = true;
delay(0.15);
se.keyCode(%d);
"1";
`, keyCode)
	out, err := RunJXA(script)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "1", nil
}

func playerIsActive() bool {
	state, err := GetCurrentState()
	if err != nil {
		return false
	}
	ps, _ := state["player_state"].(string)
	return ps == "playing" || ps == "paused"
}

// ensureCatalogPlayback tries several ways to start playback after a track is selected in Music.app.
func ensureCatalogPlayback(timeout time.Duration) bool {
	if playerIsActive() {
		return true
	}
	deadline := time.Now().Add(timeout)
	step := 0
	for time.Now().Before(deadline) {
		if playerIsActive() {
			return true
		}
		switch step % 6 {
		case 0:
			_, _ = RunAppleScript(`tell application "Music" to play`)
		case 1:
			_, _ = pressMusicKey(36) // Return — plays selected track in album lists
		case 2:
			_, _ = clickMusicUIButtonAny("Play", "play", "Resume")
		case 3:
			_, _ = pressMusicKey(49) // Space
		case 4:
			_, _ = RunAppleScript(`tell application "iTunes" to play`)
		case 5:
			_, _ = clickMusicUIButtonAny("Play", "play")
		}
		step++
		sleepMs(700)
	}
	return playerIsActive()
}

func openCatalogInMusic(itmsURL string) error {
	itmsURL = strings.TrimSpace(itmsURL)
	if itmsURL == "" {
		return fmt.Errorf("empty catalog URL")
	}
	activate := `tell application "Music" to activate`
	if _, err := RunAppleScript(activate); err != nil {
		activate = `tell application "iTunes" to activate`
		if _, err = RunAppleScript(activate); err != nil {
			return err
		}
	}
	_ = exec.Command("open", "-g", "-a", "Music", itmsURL).Run()
	openScript := fmt.Sprintf(`tell application "Music" to open location %q`, itmsURL)
	if _, err := RunAppleScript(openScript); err != nil {
		openScript = strings.Replace(openScript, `"Music"`, `"iTunes"`, 1)
		if _, err = RunAppleScript(openScript); err != nil {
			return err
		}
	}
	return nil
}

func playCatalogViaStoreSearch(meta *ITunesTrackMeta) (bool, error) {
	if !musicUIAutomationAvailable() {
		return false, nil
	}
	q := url.QueryEscape(strings.TrimSpace(meta.TrackName + " " + meta.ArtistName))
	searchURL := fmt.Sprintf("https://music.apple.com/us/search?term=%s", q)
	if err := openCatalogInMusic(searchURL); err != nil {
		return false, err
	}
	sleepMs(6000)
	if ok, _ := clickMusicSearchResultPlay(meta.TrackName, meta.ArtistName); ok {
		if ensureCatalogPlayback(10 * time.Second) {
			return true, nil
		}
	}
	return false, nil
}

func playCatalogViaITMSPage(meta *ITunesTrackMeta) (bool, error) {
	itms := catalogITMSURL(meta.CollectionID, meta.TrackID)
	if itms == "" {
		itms = strings.Replace(meta.TrackViewURL, "https://music.apple.com", "itmss://geo.music.apple.com", 1)
		itms = strings.Replace(itms, "http://music.apple.com", "itmss://geo.music.apple.com", 1)
	}
	if err := openCatalogInMusic(itms); err != nil {
		return false, err
	}
	sleepMs(5000)

	// ?i= in the URL often selects the track; try starting playback before clicking the row.
	if ensureCatalogPlayback(8 * time.Second) {
		return true, nil
	}

	if meta.TrackNumber > 0 {
		if ok, _ := doubleClickAlbumTrackByNumber(meta.TrackNumber); ok {
			if ensureCatalogPlayback(10 * time.Second) {
				return true, nil
			}
		}
	}

	if ok, _ := doubleClickMusicTrackRow(meta.TrackName); ok {
		if ensureCatalogPlayback(10 * time.Second) {
			return true, nil
		}
	}

	// Single click only selects (highlights) the row — follow with explicit play attempts.
	if ok, _ := selectMusicTrackRow(meta.TrackName); ok {
		if ensureCatalogPlayback(12 * time.Second) {
			return true, nil
		}
	}

	return false, nil
}

func playCatalogViaAddToLibrary(meta *ITunesTrackMeta) (bool, error) {
	if !musicUIAutomationAvailable() {
		return false, nil
	}
	itms := catalogITMSURL(meta.CollectionID, meta.TrackID)
	if err := openCatalogInMusic(itms); err != nil {
		return false, err
	}
	sleepMs(6000)
	if ok, _ := clickMusicUIButton("Add to Library"); !ok {
		if ensureCatalogPlayback(12 * time.Second) {
			return true, nil
		}
		return false, nil
	}
	if id, ok := waitForLibraryTrack(meta.TrackName, meta.ArtistName, 30*time.Second); ok {
		return playLibraryTrackByPersistentID(id)
	}
	return false, nil
}

func waitForLibraryTrack(name, artist string, timeout time.Duration) (string, bool) {
	deadline := time.Now().Add(timeout)
	nameOnlyAfter := time.Now().Add(timeout / 2)
	for time.Now().Before(deadline) {
		InvalidateLibraryCache()
		tracks, err := GetLibraryTracks()
		if err == nil {
			if id := matchLibraryTrack(tracks, name, artist); id != "" {
				return id, true
			}
			if time.Now().After(nameOnlyAfter) {
				if id := matchLibraryTrackByName(tracks, name); id != "" {
					return id, true
				}
			}
		}
		sleepMs(2000)
	}
	return "", false
}

func matchLibraryTrack(tracks []LibraryTrack, name, artist string) string {
	nameKey := foldAccents(normalizeTrackTitle(name))
	artistKey := foldAccents(artist)
	for _, t := range tracks {
		tName := foldAccents(normalizeTrackTitle(t.Name))
		if nameKey == "" {
			continue
		}
		if !strings.Contains(tName, nameKey) && !strings.Contains(nameKey, tName) {
			continue
		}
		if artistKey != "" {
			ta := foldAccents(t.Artist)
			taa := foldAccents(t.AlbumArtist)
			if !strings.Contains(ta, artistKey) && !strings.Contains(taa, artistKey) {
				continue
			}
		}
		return t.ID
	}
	return ""
}

func matchLibraryTrackByName(tracks []LibraryTrack, name string) string {
	nameKey := foldAccents(normalizeTrackTitle(name))
	if nameKey == "" {
		return ""
	}
	for _, t := range tracks {
		tName := foldAccents(normalizeTrackTitle(t.Name))
		if strings.Contains(tName, nameKey) || strings.Contains(nameKey, tName) {
			return t.ID
		}
	}
	return ""
}

func foldAccents(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	repl := strings.NewReplacer(
		"à", "a", "á", "a", "â", "a", "ä", "a", "ã", "a", "å", "a",
		"é", "e", "è", "e", "ê", "e", "ë", "e",
		"í", "i", "ì", "i", "î", "i", "ï", "i",
		"ó", "o", "ò", "o", "ô", "o", "ö", "o", "õ", "o",
		"ú", "u", "ù", "u", "û", "u", "ü", "u",
		"ç", "c", "ñ", "n",
	)
	return repl.Replace(s)
}

func musicUIAutomationAvailable() bool {
	script := `
var se = Application("System Events");
try {
  var proc = se.processes.byName("Music");
  proc.frontmost = true;
  proc.windows.length >= 0 ? "1" : "0";
} catch (e) { "0"; }
`
	out, err := RunJXA(script)
	return err == nil && strings.TrimSpace(out) == "1"
}

func clickMusicSearchResultPlay(trackName, artist string) (bool, error) {
	trackNeedle := foldAccents(normalizeTrackTitle(trackName))
	artistNeedle := foldAccents(artist)
	if trackNeedle == "" {
		return false, nil
	}
	script := fmt.Sprintf(`
var se = Application("System Events");
var w = se.processes.byName("Music").windows[0];
var trackNeedle = %q;
var artistNeedle = %q;
function findPlayIn(el, depth) {
  if (depth > 8) return null;
  try {
    if (el.role() === "AXButton") {
      var d = String(el.description() || "").toLowerCase();
      if (d === "play") return el;
    }
  } catch(e) {}
  try {
    var kids = el.uiElements();
    for (var i = 0; i < kids.length; i++) {
      var f = findPlayIn(kids[i], depth + 1);
      if (f) return f;
    }
  } catch(e) {}
  return null;
}
function blob(el, depth) {
  if (depth > 4) return "";
  var parts = [];
  try { if (el.value()) parts.push(String(el.value())); } catch(e) {}
  try { if (el.description()) parts.push(String(el.description())); } catch(e) {}
  try {
    var kids = el.uiElements();
    for (var i = 0; i < kids.length; i++) parts.push(blob(kids[i], depth + 1));
  } catch(e) {}
  return parts.join(" ").toLowerCase();
}
function walk(el, depth) {
  if (depth > 12) return null;
  var text = blob(el, 0);
  if (text.indexOf(trackNeedle) >= 0 && (artistNeedle === "" || text.indexOf(artistNeedle) >= 0)) {
    var btn = findPlayIn(el, depth);
    if (btn) return btn;
  }
  try {
    var kids = el.uiElements();
    for (var i = 0; i < kids.length; i++) {
      var f = walk(kids[i], depth + 1);
      if (f) return f;
    }
  } catch(e) {}
  return null;
}
var btn = walk(w, 0);
if (!btn) { "0"; } else { btn.click(); "1"; }
`, trackNeedle, artistNeedle)
	out, err := RunJXA(script)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "1", nil
}

func doubleClickAlbumTrackByNumber(trackNumber int) (bool, error) {
	if trackNumber <= 0 {
		return false, nil
	}
	script := fmt.Sprintf(`
var se = Application("System Events");
var proc = se.processes.byName("Music");
proc.frontmost = true;
delay(0.2);
var w = proc.windows[0];
var numStr = %q;
function firstValue(el) {
  try {
    var v = el.value();
    if (v) return String(v).trim();
  } catch(e) {}
  return "";
}
function rowLead(el) {
  try {
    var kids = el.uiElements();
    if (kids.length === 0) return "";
    return firstValue(kids[0]);
  } catch(e) { return ""; }
}
function pressTwice(el) {
  el.performAction("AXPress");
  delay(0.08);
  el.performAction("AXPress");
}
function rowMatches(el) {
  var lead = rowLead(el);
  if (lead === numStr) return true;
  try {
    var parts = [];
    var kids = el.uiElements();
    for (var i = 0; i < Math.min(kids.length, 4); i++) parts.push(firstValue(kids[i]));
    var text = parts.join(" ");
    if (text.indexOf(numStr + " ") === 0) return true;
  } catch(e) {}
  return false;
}
function walk(el, depth) {
  if (depth > 18) return false;
  try {
    var role = el.role();
    if (role === "AXRow" || role === "AXGroup" || role === "group" || role === "row") {
      if (rowMatches(el)) {
        pressTwice(el);
        return true;
      }
    }
  } catch(e) {}
  try {
    var kids = el.uiElements();
    for (var i = 0; i < kids.length; i++) {
      if (walk(kids[i], depth + 1)) return true;
    }
  } catch(e) {}
  return false;
}
walk(w, 0) ? "1" : "0";
`, strconv.Itoa(trackNumber))
	out, err := RunJXA(script)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "1", nil
}

func normalizeTrackTitle(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if i := strings.Index(name, " ("); i > 0 {
		name = name[:i]
	}
	return strings.ToLower(name)
}

func sleepMs(ms int) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
}
