#!/usr/bin/env bash
# Remove the applemusicgo launchd agent.
set -euo pipefail

LABEL="com.rob121.applemusicgo"
PREFIX="${HOME}/applemusicgo"
REMOVE_DATA=0

usage() {
	cat <<EOF
Usage: $(basename "$0") [options]

Options:
  --prefix PATH     Install directory to remove (default: ~/applemusicgo)
  --remove-data     Delete install directory (binary, logs, artwork cache)
  -h, --help        Show this help
EOF
}

while [[ $# -gt 0 ]]; do
	case "$1" in
		--prefix)
			PREFIX="$2"
			shift 2
			;;
		--remove-data)
			REMOVE_DATA=1
			shift
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

PREFIX="${PREFIX/#\~/$HOME}"
PLIST_DEST="$HOME/Library/LaunchAgents/${LABEL}.plist"
GUI_UID="$(id -u)"
DOMAIN="gui/$GUI_UID"

launchctl bootout "$DOMAIN" "$PLIST_DEST" 2>/dev/null || true
launchctl unload -w "$PLIST_DEST" 2>/dev/null || true
rm -f "$PLIST_DEST"

if [[ "$REMOVE_DATA" -eq 1 && -d "$PREFIX" ]]; then
	rm -rf "$PREFIX"
	echo "Removed $PREFIX"
else
	echo "Launch agent removed. Install files kept at $PREFIX"
	echo "  Re-run with --remove-data to delete the install directory."
fi
