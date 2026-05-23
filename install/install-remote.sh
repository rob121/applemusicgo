#!/usr/bin/env bash
# Cross-compile applemusicgo, copy install assets, and deploy via scp/ssh.
set -euo pipefail

usage() {
	cat <<EOF
Usage: $(basename "$0") <user@host> [install-prefix] [options]

Arguments:
  user@host         SSH target (the Mac running Music.app)
  install-prefix    Remote install path (default: ~/applemusicgo)

Options:
  --port PORT       HTTP port on remote (default: 8181)
  --go GO           Go binary to use (default: go)
  -h, --help        Show this help

Examples:
  $(basename "$0") music@192.168.1.50
  $(basename "$0") user@mac-mini.local ~/applemusicgo --port 8181

Uses one SSH connection (ControlMaster) so you are only prompted for a password once.
For passwordless deploys, add your SSH public key to the target Mac.
EOF
}

TARGET=""
PREFIX="~/applemusicgo"
PORT="8181"
GO_BIN="${GO_BIN:-go}"
SSH_CONTROL=""

cleanup() {
	if [[ -n "$SSH_CONTROL" && -n "$TARGET" ]]; then
		ssh -o ControlPath="$SSH_CONTROL" -O exit "$TARGET" 2>/dev/null || true
		rm -f "$SSH_CONTROL"
	fi
	if [[ -n "${STAGING:-}" && -d "${STAGING:-}" ]]; then
		rm -rf "$STAGING"
	fi
}
trap cleanup EXIT

ssh_cmd() {
	ssh -o ControlMaster=auto -o ControlPath="$SSH_CONTROL" -o ControlPersist=120 "$TARGET" "$@"
}

scp_cmd() {
	scp -o ControlMaster=auto -o ControlPath="$SSH_CONTROL" -o ControlPersist=120 "$@"
}

while [[ $# -gt 0 ]]; do
	case "$1" in
		-h | --help)
			usage
			exit 0
			;;
		--port)
			PORT="$2"
			shift 2
			;;
		--go)
			GO_BIN="$2"
			shift 2
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
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
STAGING="$SCRIPT_DIR/staging"
SSH_CONTROL="$(mktemp -u /tmp/applemusicgo-ssh.XXXXXX)"

echo "==> Detecting remote architecture on $TARGET"
REMOTE_ARCH="$(ssh_cmd 'uname -m')"
case "$REMOTE_ARCH" in
	arm64 | aarch64) GOARCH=arm64 ;;
	x86_64) GOARCH=amd64 ;;
	*)
		echo "error: unsupported remote architecture: $REMOTE_ARCH" >&2
		exit 1
		;;
esac
echo "    building darwin/$GOARCH"

mkdir -p "$STAGING"
(
	cd "$REPO_ROOT"
	GOOS=darwin GOARCH="$GOARCH" CGO_ENABLED=0 \
		"$GO_BIN" build -ldflags="-s -w" -o "$STAGING/applemusicgo" ./cmd/applemusicgo
)

cp "$SCRIPT_DIR/com.rob121.applemusicgo.plist.template" "$STAGING/"
cp "$SCRIPT_DIR/install-service.sh" "$STAGING/"
cp "$SCRIPT_DIR/uninstall-service.sh" "$STAGING/"
chmod +x "$STAGING/install-service.sh" "$STAGING/uninstall-service.sh"

REMOTE_STAGING="$(ssh_cmd 'mktemp -d /tmp/applemusicgo-install.XXXXXX')"
echo "==> Copying to $TARGET:$REMOTE_STAGING"
scp_cmd -q "$STAGING/applemusicgo" \
	"$STAGING/com.rob121.applemusicgo.plist.template" \
	"$STAGING/install-service.sh" \
	"$STAGING/uninstall-service.sh" \
	"$TARGET:$REMOTE_STAGING/"

echo "==> Installing launchd service on $TARGET"
ssh_cmd "bash '$REMOTE_STAGING/install-service.sh' --prefix '$PREFIX' --port '$PORT' --source '$REMOTE_STAGING' && rm -rf '$REMOTE_STAGING'"

echo "==> Done"
