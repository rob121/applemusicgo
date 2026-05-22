try
	tell application "Music"
		set output to ""
		repeat with p in user playlists
			set output to output & (id of p as string) & tab & (name of p as string) & linefeed
		end repeat
		return output
	end tell
on error
	tell application "iTunes"
		set output to ""
		repeat with p in user playlists
			set output to output & (id of p as string) & tab & (name of p as string) & linefeed
		end repeat
		return output
	end tell
end try
