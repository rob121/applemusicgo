#!/usr/bin/env bash
# Remove applemusicgo launchd service on a remote Mac via ssh.
set -euo pipefail

usage() {
	cat <<EOF
Usage: $(basename "$0") <user@host> [install-prefix] [--remove-data]

Examples:
  $(basename "$0") music@192.168.1.50
  $(basename "$0") user@mac-mini.local ~/applemusicgo --remove-data
EOF
}

TARGET=""
PREFIX="~/applemusicgo"
REMOVE_DATA=""
SSH_CONTROL=""

cleanup() {
	if [[ -n "$SSH_CONTROL" && -n "$TARGET" ]]; then
		ssh -o ControlPath="$SSH_CONTROL" -O exit "$TARGET" 2>/dev/null || true
		rm -f "$SSH_CONTROL"
	fi
}
trap cleanup EXIT

while [[ $# -gt 0 ]]; do
	case "$1" in
		-h | --help)
			usage
			exit 0
			;;
		--remove-data)
			REMOVE_DATA="--remove-data"
			shift
			;;
		-*)
			echo "Unknown option: $1" >&2
			usage >&2
			exit 1
			;;
		*)
			if [[ -z "$TARGET" ]]; then
				TARGET="$1"
			elif [[ "$PREFIX" == "~/applemusicgo" ]]; then
				PREFIX="$1"
			else
				echo "Unexpected argument: $1" >&2
				usage >&2
				exit 1
			fi
			shift
			;;
	esac
done

if [[ -z "$TARGET" ]]; then
	usage >&2
	exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SSH_CONTROL="$(mktemp -u /tmp/applemusicgo-ssh.XXXXXX)"

scp -o ControlMaster=auto -o ControlPath="$SSH_CONTROL" -o ControlPersist=60 \
	-q "$SCRIPT_DIR/uninstall-service.sh" "$TARGET:/tmp/applemusicgo-uninstall.sh"
ssh -o ControlMaster=auto -o ControlPath="$SSH_CONTROL" -o ControlPersist=60 \
	"$TARGET" "bash /tmp/applemusicgo-uninstall.sh --prefix '$PREFIX' $REMOVE_DATA; rm -f /tmp/applemusicgo-uninstall.sh"
