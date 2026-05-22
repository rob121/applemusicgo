package music

import (
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
	script := fmt.Sprintf(`
var se = Application("System Events");
var proc = se.processes.byName("Music");
proc.frontmost = true;
delay(0.3);
var w = proc.windows[0];
function find(el, desc) {
  try {
    if (el.role() === "AXButton" && el.description() === desc) return el;
  } catch(e) {}
  try {
    var kids = el.uiElements();
    for (var i = 0; i < kids.length; i++) {
      var f = find(kids[i], desc);
      if (f) return f;
    }
  } catch(e) {}
  return null;
}
var btn = find(w, %q);
if (!btn) { "0"; } else { btn.click(); "1"; }
`, description)
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
	needle := normalizeTrackTitle(trackName)
	if needle == "" {
		return false, nil
	}
	script := fmt.Sprintf(`
var se = Application("System Events");
var proc = se.processes.byName("Music");
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
		return waitForPlayback(10 * time.Second), nil
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
	sleepMs(8000)
	if meta.TrackNumber > 0 {
		_, _ = doubleClickAlbumTrackByNumber(meta.TrackNumber)
		sleepMs(1200)
		if waitForPlayback(8 * time.Second) {
			return true, nil
		}
	}
	_, _ = clickMusicTrackRow(meta.TrackName)
	sleepMs(800)
	if musicUIAutomationAvailable() {
		if ok, _ := clickMusicUIButton("play"); ok {
			if waitForPlayback(10 * time.Second) {
				return true, nil
			}
		}
	}
	// Global play as fallback (Music menu bar transport).
	if _, err := RunAppleScript(`tell application "Music" to play`); err != nil {
		_, _ = RunAppleScript(`tell application "iTunes" to play`)
	}
	return waitForPlayback(8 * time.Second), nil
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
    if (el.role() === "AXButton" && el.description() === "play") return el;
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
var w = se.processes.byName("Music").windows[0];
var idx = %d;
var groups = [];
function collectGroups(el, depth) {
  if (depth > 14) return;
  try {
    if (el.role() === "AXGroup" || el.role() === "group") groups.push(el);
  } catch(e) {}
  try {
    var kids = el.uiElements();
    for (var i = 0; i < kids.length; i++) collectGroups(kids[i], depth + 1);
  } catch(e) {}
}
collectGroups(w, 0);
if (groups.length >= idx) {
  try {
    groups[idx - 1].performAction("AXPress");
    groups[idx - 1].performAction("AXPress");
    "1";
  } catch (e) { "0"; }
} else { "0"; }
`, trackNumber)
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
