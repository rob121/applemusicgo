#!/usr/bin/env bash
# Install or upgrade applemusicgo as a per-user launchd agent (starts at login).
set -euo pipefail

LABEL="com.rob121.applemusicgo"
PORT="${PORT:-8181}"
PREFIX="${HOME}/applemusicgo"
SOURCE=""

usage() {
	cat <<EOF
Usage: $(basename "$0") [options]

Options:
  --prefix PATH   Install directory (default: ~/applemusicgo)
  --port PORT     HTTP port (default: 8181)
  --source DIR    Directory containing applemusicgo binary and plist template
                  (default: directory of this script)
  -h, --help      Show this help

Run on the Mac where Music.app lives. For remote deploy, use install-remote.sh.
EOF
}

while [[ $# -gt 0 ]]; do
	case "$1" in
		--prefix)
			PREFIX="$2"
			shift 2
			;;
		--port)
			PORT="$2"
			shift 2
			;;
		--source)
			SOURCE="$2"
			shift 2
			;;
		-h | --help)
			usage
			exit 0
			;;
		*)
			echo "Unknown option: $1" >&2
			usage >&2
			exit 1
			;;
	esac
done

if [[ "$(uname -s)" != "Darwin" ]]; then
	echo "error: applemusicgo must run on macOS (Music.app required)" >&2
	exit 1
fi

PREFIX="${PREFIX/#\~/$HOME}"
PREFIX="$(cd "$PREFIX" 2>/dev/null && pwd || echo "$PREFIX")"

if [[ -z "$SOURCE" ]]; then
	SOURCE="$(cd "$(dirname "$0")" && pwd)"
fi
SOURCE="$(cd "$SOURCE" && pwd)"

BIN_SRC="$SOURCE/applemusicgo"
PLIST_TEMPLATE="$SOURCE/com.rob121.applemusicgo.plist.template"

if [[ ! -x "$BIN_SRC" && ! -f "$BIN_SRC" ]]; then
	echo "error: binary not found: $BIN_SRC" >&2
	exit 1
fi
if [[ ! -f "$PLIST_TEMPLATE" ]]; then
	echo "error: plist template not found: $PLIST_TEMPLATE" >&2
	exit 1
fi

mkdir -p "$PREFIX/log" "$PREFIX/data/custom-artwork" "$HOME/Library/LaunchAgents"
install -m 755 "$BIN_SRC" "$PREFIX/applemusicgo"

PLIST_DEST="$HOME/Library/LaunchAgents/${LABEL}.plist"
sed \
	-e "s|@INSTALL_DIR@|$PREFIX|g" \
	-e "s|@PORT@|$PORT|g" \
	"$PLIST_TEMPLATE" >"$PLIST_DEST"

GUI_UID="$(id -u)"
DOMAIN="gui/$GUI_UID"

# Stop existing agent if loaded.
launchctl bootout "$DOMAIN" "$PLIST_DEST" 2>/dev/null || true
launchctl unload -w "$PLIST_DEST" 2>/dev/null || true

if launchctl bootstrap "$DOMAIN" "$PLIST_DEST" 2>/dev/null; then
	launchctl kickstart -k "$DOMAIN/$LABEL" 2>/dev/null || true
else
	launchctl load -w "$PLIST_DEST"
fi

sleep 1
if curl -sf "http://127.0.0.1:${PORT}/_ping" >/dev/null; then
	echo "applemusicgo installed and responding on http://127.0.0.1:${PORT}"
else
	echo "applemusicgo installed at $PREFIX (service may still be starting)"
	echo "  logs: $PREFIX/log/"
	echo "  check: curl http://127.0.0.1:${PORT}/_ping"
fi

echo "  plist: $PLIST_DEST"
