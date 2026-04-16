#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

pick_prefix() {
  # Use INSTALL_PREFIX if provided
  if [[ -n "${INSTALL_PREFIX:-}" ]]; then
    echo "$INSTALL_PREFIX"
    return
  fi
  # Prefer user-writable locations on PATH
  local candidates=(
    "$HOME/.local/bin"
    "$HOME/bin"
    "/opt/homebrew/bin"
    "/usr/local/bin"
  )
  for dir in "${candidates[@]}"; do
    if [[ -d "$(dirname "$dir")" ]] && [[ -w "$dir" || ( ! -e "$dir" && -w "$(dirname "$dir")" ) ]]; then
      mkdir -p "$dir"
      echo "$dir"
      return
    fi
  done
  echo "/usr/local/bin"
}

BIN_DIR="$(pick_prefix)"

echo "=== Installing /look skill + CLI ==="
echo ""
echo "Install dir: $BIN_DIR"

# Build
echo "Building..."
(cd "$REPO_DIR" && make build)

# Install binaries
for bin in look lookd; do
  src="$REPO_DIR/bin/$bin"
  dst="$BIN_DIR/$bin"
  if [[ -w "$BIN_DIR" ]]; then
    install -m 0755 "$src" "$dst"
  else
    echo "Installing $bin (needs sudo)..."
    sudo install -m 0755 "$src" "$dst"
  fi
  echo "  $dst"
done

# PATH hint
case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo "  note: add $BIN_DIR to your PATH" ;;
esac

# Install Claude Code skill
if [[ -d "$HOME/.claude" ]]; then
  mkdir -p "$HOME/.claude/skills/look"
  cp "$REPO_DIR/skills/claude/SKILL.md" "$HOME/.claude/skills/look/SKILL.md"
  echo "  ~/.claude/skills/look/SKILL.md"
fi

# Install Cursor command
if [[ -d "$HOME/.cursor" ]]; then
  mkdir -p "$HOME/.cursor/commands"
  cp "$REPO_DIR/skills/cursor/command.md" "$HOME/.cursor/commands/look.md"
  echo "  ~/.cursor/commands/look.md"
fi

echo ""
echo "Done! Use /look in Claude Code or Cursor."
echo ""
echo "Quick start:"
echo "  look --list"
echo "  look --repo jschell12/my-app"
