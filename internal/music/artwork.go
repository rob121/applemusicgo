package music

import (
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func ArtworkDir(base string) string {
	return filepath.Join(base, "artwork-cache")
}

func CustomArtworkDir(base string) string {
	return filepath.Join(base, "custom-artwork")
}

func EnsureArtworkDirs(base string) {
	for _, d := range []string{ArtworkDir(base), CustomArtworkDir(base)} {
		_ = os.MkdirAll(d, 0755)
	}
}

func CustomArtworkPath(base, name string) string {
	slug := Slugify(name)
	for _, ext := range []string{".jpg", ".jpeg", ".png"} {
		p := filepath.Join(CustomArtworkDir(base), slug+ext)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func ArtworkFilePath(base, artist, album string) string {
	return filepath.Join(ArtworkDir(base), Slugify(artist+"||"+album)+".jpg")
}

const nowPlayingArtScript = `set outPath to "/tmp/currently-playing.jpg"
try
	tell application "Music"
		if player state is not stopped then
			set artData to raw data of artwork 1 of current track
			set f to open for access POSIX file outPath with write permission
			set eof of f to 0
			write artData to f
			close access f
		end if
	end tell
on error
	try
		tell application "iTunes"
			if player state is not stopped then
				set artData to raw data of artwork 1 of current track
				set f to open for access POSIX file outPath with write permission
				set eof of f to 0
				write artData to f
				close access f
			end if
		end tell
	end try
end try`

func FetchAndSaveArtwork(base, artist, album string) (string, error) {
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("album-art-%d.jpg", time.Now().UnixNano()))
	script := fmt.Sprintf(`set targetArtist to %q
set targetAlbum to %q
set outPath to %q
try
	tell application "Music"
		repeat with t in (tracks of library playlist 1)
			if (artist of t as string) is targetArtist and (album of t as string) is targetAlbum then
				try
					set artData to raw data of artwork 1 of t
					set f to open for access POSIX file outPath with write permission
					set eof of f to 0
					write artData to f
					close access f
					return "ok"
				end try
			end if
		end repeat
	end tell
on error
	tell application "iTunes"
		repeat with t in (tracks of library playlist 1)
			if (artist of t as string) is targetArtist and (album of t as string) is targetAlbum then
				try
					set artData to raw data of artwork 1 of t
					set f to open for access POSIX file outPath with write permission
					set eof of f to 0
					write artData to f
					close access f
					return "ok"
				end try
			end if
		end repeat
	end tell
end try
return "fail"`, artist, album, tmpFile)
	out, err := RunAppleScript(script)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(out) != "ok" {
		return "", fmt.Errorf("no artwork")
	}
	dest := ArtworkFilePath(base, artist, album)
	if err := os.Rename(tmpFile, dest); err != nil {
		_ = os.Remove(tmpFile)
		return "", err
	}
	return dest, nil
}

func FetchNowPlayingArtwork() error {
	_, err := RunAppleScript(nowPlayingArtScript)
	return err
}

func NowPlayingArtworkPath() string {
	return "/tmp/currently-playing.jpg"
}

func BuildCollageFromCovers(covers []string, cacheFile string) error {
	for len(covers) < 4 && len(covers) > 0 {
		covers = append(covers, covers[len(covers)-1])
	}
	if len(covers) == 0 {
		return fmt.Errorf("no covers found")
	}

	const size = 300
	half := size / 2
	dst := image.NewRGBA(image.Rect(0, 0, size, size))

	for i, cover := range covers[:4] {
		f, err := os.Open(cover)
		if err != nil {
			return err
		}
		img, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			return err
		}
		b := img.Bounds()
		scaled := resizeNearest(img, b.Dx(), b.Dy(), half, half)
		x0 := 0
		y0 := 0
		if i == 1 {
			x0 = half
		}
		if i == 2 {
			y0 = half
		}
		if i == 3 {
			x0, y0 = half, half
		}
		blit(dst, scaled, x0, y0)
	}

	out, err := os.Create(cacheFile)
	if err != nil {
		return err
	}
	defer out.Close()
	return jpeg.Encode(out, dst, &jpeg.Options{Quality: 90})
}

func resizeNearest(src image.Image, sw, sh, dw, dh int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			sx := x * sw / dw
			sy := y * sh / dh
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

func blit(dst *image.RGBA, src *image.RGBA, x0, y0 int) {
	sb := src.Bounds()
	for y := 0; y < sb.Dy(); y++ {
		for x := 0; x < sb.Dx(); x++ {
			dst.Set(x0+x, y0+y, src.At(sb.Min.X+x, sb.Min.Y+y))
		}
	}
}

func BuildPlaylistCollage(base, playlistName string) (string, error) {
	cacheFile := filepath.Join(ArtworkDir(base), "playlist-"+Slugify(playlistName)+".jpg")
	if _, err := os.Stat(cacheFile); err == nil {
		return cacheFile, nil
	}

	tracks, _ := GetPlaylistTracks(playlistName)
	seen := map[string]bool{}
	var covers []string
	for _, t := range tracks {
		if len(covers) >= 4 {
			break
		}
		artist, album := t.Artist, t.Album
		if artist == "" || album == "" {
			continue
		}
		key := artist + "||" + album
		if seen[key] {
			continue
		}
		file := ArtworkFilePath(base, artist, album)
		if _, err := os.Stat(file); err != nil {
			continue
		}
		seen[key] = true
		covers = append(covers, file)
	}

	if len(covers) == 0 {
		libTracks, err := GetLibraryTracks()
		if err != nil {
			return "", err
		}
		seen2 := map[string]bool{}
		for _, t := range libTracks {
			if len(covers) >= 4 {
				break
			}
			artist := t.AlbumArtist
			if artist == "" {
				artist = t.Artist
			}
			album := t.Album
			if artist == "" || album == "" {
				continue
			}
			key := artist + "||" + album
			if seen2[key] {
				continue
			}
			file := ArtworkFilePath(base, artist, album)
			if _, err := os.Stat(file); err != nil {
				continue
			}
			seen2[key] = true
			covers = append(covers, file)
		}
	}

	if len(covers) == 0 {
		return "", fmt.Errorf("no covers found")
	}
	if err := BuildCollageFromCovers(covers, cacheFile); err != nil {
		return "", err
	}
	return cacheFile, nil
}

func CopyArtistCache(base, artist, src string) {
	cacheFile := filepath.Join(ArtworkDir(base), "artist-"+Slugify(artist)+".jpg")
	_ = copyFile(src, cacheFile)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
