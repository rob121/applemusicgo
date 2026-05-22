on run argv
	set playlistId to item 1 of argv
	try
		tell application "Music"
			repeat with p in user playlists
				if (id of p as string) is playlistId then
					play p
					return "ok"
				end if
			end repeat
		end tell
	on error
		tell application "iTunes"
			repeat with p in user playlists
				if (id of p as string) is playlistId then
					play p
					return "ok"
				end if
			end repeat
		end tell
	end try
	return "notfound"
end run
