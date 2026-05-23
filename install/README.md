# Install as a macOS service

Run **applemusicgo** as a per-user **launchd** agent so it starts at login and restarts on failure.

## Remote deploy (build + scp + install)

From the repo root on your dev machine:

```bash
chmod +x install/*.sh
./install/install-remote.sh user@mac-host
```

Optional install path and port:

```bash
./install/install-remote.sh music@192.168.1.50 ~/applemusicgo --port 8181
```

This will:

1. Detect the remote CPU (`arm64` / `x86_64`) and cross-compile the Go binary
2. Copy the binary, plist template, and install scripts via **scp**
3. **ssh** into the target and run `install-service.sh`
4. Register `~/Library/LaunchAgents/com.rob121.applemusicgo.plist`

SSH uses a single multiplexed connection (one password prompt). For passwordless deploys:

```bash
ssh-copy-id dashboard@192.168.20.228
```

## Local install (same Mac)

After building:

```bash
go build -o install/staging/applemusicgo ./cmd/applemusicgo
./install/install-service.sh --source ./install/staging
```

Or copy a release binary into `install/staging/` first.

## Uninstall

**Remote:**

```bash
./install/uninstall-remote.sh user@mac-host
./install/uninstall-remote.sh user@mac-host ~/applemusicgo --remove-data
```

**Local:**

```bash
./install/uninstall-service.sh
./install/uninstall-service.sh --remove-data
```

## Install layout on the Mac

```
~/applemusicgo/
  applemusicgo          # binary
  data/                 # --dir (artwork-cache, custom-artwork)
  log/
    applemusicgo.log
    applemusicgo.error.log
~/Library/LaunchAgents/com.rob121.applemusicgo.plist
```

## Permissions

The launchd agent runs in your **GUI login session**, which is required for Music.app / osascript control. After install, grant **Automation** (Music) and **Accessibility** (if using catalog play) to the process context — see the main [README](../README.md).

For instant SSE push updates (like [apple-music-custom](https://github.com/Hackashaq666/apple-music-custom)), run a separate `notify.py` listener or use polling; this install bundle covers the **applemusicgo** HTTP server only.

## Verify

```bash
curl http://127.0.0.1:8181/_ping
curl http://127.0.0.1:8181/now_playing
launchctl print "gui/$(id -u)/com.rob121.applemusicgo"
```
