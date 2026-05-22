package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/rob121/applemusicgo/internal/music"
	"github.com/rob121/applemusicgo/internal/server"
)

func main() {
	baseDir := flag.String("dir", ".", "base directory (lib, artwork-cache)")
	port := flag.Int("port", 0, "HTTP listen port (default 8181 or PORT env)")
	flag.Parse()

	if flag.NArg() == 0 {
		printUsage()
		os.Exit(1)
	}

	cmd := flag.Arg(0)
	switch cmd {
	case "serve", "server":
		runServer(*baseDir, *port)
	default:
		if err := runCLI(*baseDir, cmd, flag.Args()[1:]); err != nil {
			log.Fatal(err)
		}
	}
}

func runServer(baseDir string, portFlag int) {
	port := portFlag
	if port == 0 {
		if p := os.Getenv("PORT"); p != "" {
			port, _ = strconv.Atoi(p)
		}
	}
	if port == 0 {
		port = 8181
	}
	abs, _ := filepath.Abs(baseDir)
	lib := filepath.Join(abs, "lib")
	_ = os.Setenv("APPLEMUSICGO_LIB", lib)

	addr := fmt.Sprintf(":%d", port)
	srv := server.New(abs, addr)
	log.Fatal(srv.ListenAndServe())
}

func runCLI(baseDir, cmd string, args []string) error {
	abs, _ := filepath.Abs(baseDir)
	_ = os.Setenv("APPLEMUSICGO_LIB", filepath.Join(abs, "lib"))

	var err error
	switch cmd {
	case "play":
		err = music.Play()
	case "pause":
		err = music.Pause()
	case "playpause":
		err = music.PlayPause()
	case "stop":
		err = music.Stop()
	case "previous", "prev":
		err = music.Previous()
	case "next":
		err = music.Next()
	case "now-playing", "now_playing", "state":
		return printState()
	case "volume":
		if len(args) < 1 {
			return fmt.Errorf("usage: applemusicgo volume <level>")
		}
		_, err = music.SetVolume(args[0])
	case "mute":
		if len(args) < 1 {
			return fmt.Errorf("usage: applemusicgo mute <true|false>")
		}
		_, err = music.SetMuted(args[0])
	case "shuffle":
		mode := "songs"
		if len(args) > 0 {
			mode = args[0]
		}
		_, err = music.SetShuffle(mode)
	case "repeat":
		mode := "all"
		if len(args) > 0 {
			mode = args[0]
		}
		_, err = music.SetRepeat(mode)
	case "seek":
		if len(args) < 1 {
			return fmt.Errorf("usage: applemusicgo seek <position>")
		}
		err = music.SeekToPosition(args[0])
	case "play-track":
		if len(args) < 1 {
			return fmt.Errorf("usage: applemusicgo play-track <id>")
		}
		var played bool
		played, err = music.PlayTrackByID(args[0])
		if err == nil && !played {
			return music.ErrNotFound
		}
	default:
		return fmt.Errorf("unknown command: %s", cmd)
	}
	if err != nil {
		return err
	}
	return printState()
}

func printState() error {
	state, err := music.GetCurrentState()
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(state)
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `applemusicgo - control Apple Music on macOS via osascript

Usage:
  applemusicgo serve [--dir .] [--port 8181]
  applemusicgo <command> [args]

Commands:
  serve              Start HTTP API server
  play|pause|playpause|stop|previous|next
  now-playing        Print current player state as JSON
  volume <level>     Set volume 0-100
  mute <true|false>
  shuffle [mode]     songs|albums|off
  repeat [mode]      all|one|off
  seek <seconds>
  play-track <id>    Play by library (hex) or catalog (numeric) id

Environment:
  PORT               HTTP port (default 8181)
  APPLEMUSICGO_LIB   Path to lib/*.applescript (default: <dir>/lib)

`)
}
