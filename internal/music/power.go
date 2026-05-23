package music

import (
	"os/exec"
	"time"
)

const quitMusicJXA = `
var app;
try { app = Application("Music"); } catch(e) { app = Application("iTunes"); }
try { app.quit(); } catch(e) {}
`

var musicProcessNames = []string{"Music", "iTunes"}

// RestartMusicApp quits Music.app (or iTunes), waits for exit, and relaunches it.
func RestartMusicApp() error {
	_, _ = RunJXA(quitMusicJXA)

	waitForMusicExit(8 * time.Second)

	for _, name := range musicProcessNames {
		if processRunning(name) {
			_ = exec.Command("killall", name).Run()
		}
	}
	waitForMusicExit(3 * time.Second)

	if err := launchMusicApp(); err != nil {
		return err
	}
	InvalidateLibraryCache()
	return nil
}

func launchMusicApp() error {
	if err := exec.Command("open", "-a", "Music").Run(); err != nil {
		return exec.Command("open", "-a", "iTunes").Run()
	}
	return nil
}

func processRunning(name string) bool {
	return exec.Command("pgrep", "-x", name).Run() == nil
}

func waitForMusicExit(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		running := false
		for _, name := range musicProcessNames {
			if processRunning(name) {
				running = true
				break
			}
		}
		if !running {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}
