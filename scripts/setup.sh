#!/usr/bin/env bash
set -euo pipefail

CONFIG_DIR="$HOME/.screenshot-agent"
CONFIG_FILE="$CONFIG_DIR/config.json"

echo "=== Screenshot Agent Setup ==="
echo ""

# Prompt for SSH host
read -rp "SSH host (e.g., macbook.local or 192.168.1.100): " SSH_HOST
if [[ -z "$SSH_HOST" ]]; then
  echo "Error: SSH host is required"
  exit 1
fi

# Prompt for SSH user (optional)
read -rp "SSH user (leave blank for current user): " SSH_USER

# Prompt for SSH port
read -rp "SSH port (default 22): " SSH_PORT
SSH_PORT="${SSH_PORT:-22}"

# Build the SSH target
TARGET=""
if [[ -n "$SSH_USER" ]]; then
  TARGET="${SSH_USER}@${SSH_HOST}"
else
  TARGET="$SSH_HOST"
fi

echo ""
echo "Testing SSH connection to $TARGET..."

if ssh -p "$SSH_PORT" -o ConnectTimeout=10 "$TARGET" "echo ok" 2>/dev/null | grep -q "ok"; then
  echo "SSH connection successful!"
else
  echo "SSH connection failed."
  echo ""
  read -rp "Would you like to copy your SSH key? (y/n): " COPY_KEY
  if [[ "$COPY_KEY" == "y" ]]; then
    # Generate key if needed
    if [[ ! -f "$HOME/.ssh/id_ed25519" ]] && [[ ! -f "$HOME/.ssh/id_rsa" ]]; then
      echo "No SSH key found. Generating one..."
      ssh-keygen -t ed25519 -f "$HOME/.ssh/id_ed25519" -N ""
    fi
    echo "Copying SSH key to $TARGET..."
    ssh-copy-id -p "$SSH_PORT" "$TARGET"

    echo "Retesting connection..."
    if ssh -p "$SSH_PORT" -o ConnectTimeout=10 "$TARGET" "echo ok" 2>/dev/null | grep -q "ok"; then
      echo "SSH connection successful!"
    else
      echo "Still failing. Check your network and SSH settings."
      exit 1
    fi
  else
    echo "Skipping key copy. You may need to set up SSH manually."
  fi
fi

# Ensure remote dirs exist
echo ""
echo "Creating remote directories..."
ssh -p "$SSH_PORT" "$TARGET" "mkdir -p ~/.screenshot-agent/queue ~/.screenshot-agent/results ~/.screenshot-agent/logs"
echo "Remote directories created."

# Write config
mkdir -p "$CONFIG_DIR"
cat > "$CONFIG_FILE" <<EOF
{
  "sshHost": "$SSH_HOST",
$(if [[ -n "$SSH_USER" ]]; then echo "  \"sshUser\": \"$SSH_USER\","; fi)
  "sshPort": $SSH_PORT
}
EOF

echo ""
echo "Config saved to $CONFIG_FILE"
echo ""
echo "Next steps:"
echo "  Work machine:    make i-wm"
echo "  Personal machine: make i-pm"
