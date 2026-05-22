set outPath to "/tmp/currently-playing.jpg"
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
end try
