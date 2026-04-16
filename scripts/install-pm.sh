#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PLIST_LABEL="com.screenshot-agent.daemon"
PLIST_DEST="$HOME/Library/LaunchAgents/${PLIST_LABEL}.plist"
PLIST_TEMPLATE="$REPO_DIR/launchd/${PLIST_LABEL}.plist"

echo "=== Installing screenshot-agent (personal machine) ==="
echo ""

# Create directories
echo "Creating directories..."
mkdir -p ~/.screenshot-agent/{queue,results,logs}

# Build
echo "Building..."
(cd "$REPO_DIR" && pnpm build)

# Link globally
echo "Linking globally..."
(cd "$REPO_DIR" && pnpm link --global 2>/dev/null || npm link 2>/dev/null || true)

# Install launchd plist
echo "Installing daemon..."

NODE_PATH="$(which node)"
DAEMON_JS="$REPO_DIR/dist/daemon.js"

if [[ ! -f "$DAEMON_JS" ]]; then
  echo "Error: dist/daemon.js not found. Build failed?"
  exit 1
fi

# Generate plist from template
sed \
  -e "s|__NODE_PATH__|${NODE_PATH}|g" \
  -e "s|__DAEMON_JS__|${DAEMON_JS}|g" \
  -e "s|__HOME__|${HOME}|g" \
  "$PLIST_TEMPLATE" > "$PLIST_DEST"

echo "Plist installed at $PLIST_DEST"

# Unload if already loaded, then load
launchctl unload "$PLIST_DEST" 2>/dev/null || true
launchctl load "$PLIST_DEST"

echo "Daemon started."
echo ""
echo "Verify with: launchctl list | grep screenshot-agent"
echo "Logs at: ~/.screenshot-agent/logs/"
echo ""
echo "Control:"
echo "  make daemon-start   # start daemon"
echo "  make daemon-stop    # stop daemon"
