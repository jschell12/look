#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PLIST_LABEL="com.xmuggle.daemon"
PLIST_DEST="$HOME/Library/LaunchAgents/${PLIST_LABEL}.plist"
PLIST_TEMPLATE="$REPO_DIR/launchd/${PLIST_LABEL}.plist"
SLEEP_MODE="${SLEEP_MODE:-default}"

echo "=== Installing xmuggle daemon ==="
echo ""
echo "This machine will process screenshot tasks pushed into ~/.xmuggle/queue/"
echo "by other Macs on the LAN, plus any received via an encrypted git queue."
echo ""
echo "Sleep mode: ${SLEEP_MODE}"
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

# Build the ProgramArguments <array> body based on sleep mode.
#   default — daemon sleeps when the Mac sleeps (normal launchd behavior)
#   awake   — wrap in `caffeinate -i` so idle-sleep is prevented while running
case "$SLEEP_MODE" in
  default)
    PROGRAM_ARGS="    <string>${DAEMON_BIN}</string>"
    ;;
  awake)
    PROGRAM_ARGS="    <string>/usr/bin/caffeinate</string>
    <string>-i</string>
    <string>${DAEMON_BIN}</string>"
    ;;
  *)
    echo "Error: unknown SLEEP_MODE '${SLEEP_MODE}' (expected: default, awake)" >&2
    exit 1
    ;;
esac

# Generate plist. __HOME__ is substituted via sed; __PROGRAM_ARGS__ is replaced
# via awk so the multi-line block doesn't need escaping.
sed -e "s|__HOME__|${HOME}|g" "$PLIST_TEMPLATE" | \
  awk -v args="$PROGRAM_ARGS" '/__PROGRAM_ARGS__/ { print args; next } { print }' \
  > "$PLIST_DEST"

echo "Plist installed at $PLIST_DEST"

launchctl unload "$PLIST_DEST" 2>/dev/null || true
launchctl load "$PLIST_DEST"

echo "Daemon started."
echo ""
echo "Verify:  launchctl list | grep com.xmuggle"
echo "Logs:    make daemon-logs"
