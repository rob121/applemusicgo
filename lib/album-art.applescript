on run argv
	set targetArtist to item 1 of argv
	set targetAlbum to item 2 of argv
	set outPath to item 3 of argv
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
	return "fail"
end run
