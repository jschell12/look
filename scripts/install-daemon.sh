#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PLIST_LABEL="com.xmuggle.daemon"
PLIST_DEST="$HOME/Library/LaunchAgents/${PLIST_LABEL}.plist"
PLIST_TEMPLATE="$REPO_DIR/launchd/${PLIST_LABEL}.plist"

echo "=== Installing xmuggle daemon ==="
echo ""
echo "This machine will process screenshot tasks pushed into ~/.xmuggle/queue/"
echo "by other Macs on the LAN, plus any received via an encrypted git queue."
echo ""

mkdir -p ~/.xmuggle/{queue,results,logs}

# Make sure the CLI + daemon binaries are installed
bash "$REPO_DIR/scripts/install-skill.sh"

# Locate xmuggled (freshly installed)
DAEMON_BIN="$(command -v xmuggled)"
if [[ -z "$DAEMON_BIN" ]]; then
  echo "Error: xmuggled not found on PATH after install" >&2
  exit 1
fi

# Generate plist
sed \
  -e "s|__DAEMON_BIN__|${DAEMON_BIN}|g" \
  -e "s|__HOME__|${HOME}|g" \
  "$PLIST_TEMPLATE" > "$PLIST_DEST"

echo "Plist installed at $PLIST_DEST"

launchctl unload "$PLIST_DEST" 2>/dev/null || true
launchctl load "$PLIST_DEST"

echo "Daemon started."
echo ""
echo "Verify:  launchctl list | grep com.xmuggle"
echo "Logs:    make daemon-logs"
