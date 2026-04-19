#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

is_on_path() {
  case ":$PATH:" in
    *":$1:"*) return 0 ;;
    *) return 1 ;;
  esac
}

BIN_DIR="$HOME/.local/bin"
mkdir -p "$BIN_DIR"

# Clean up old installs in other locations
for old_dir in /opt/homebrew/bin /usr/local/bin "$HOME/bin"; do
  for bin in xmuggle xmuggled; do
    if [[ -f "$old_dir/$bin" ]]; then
      rm -f "$old_dir/$bin" 2>/dev/null || sudo rm -f "$old_dir/$bin" 2>/dev/null || true
      echo "Removed old $old_dir/$bin"
    fi
  done
done

echo "=== Installing /xmuggle skill + CLI ==="
echo "Install dir: $BIN_DIR"
echo ""

# Build
echo "Building..."
(cd "$REPO_DIR" && make build)

# Install binaries
for bin in xmuggle xmuggled; do
  install -m 0755 "$REPO_DIR/bin/$bin" "$BIN_DIR/$bin"
  echo "  $BIN_DIR/$bin"
done

# Ensure ~/.local/bin is on PATH in shell profile
if ! is_on_path "$BIN_DIR"; then
  PATH_LINE='export PATH="$HOME/.local/bin:$PATH"'
  # Add to all relevant rc files so it works regardless of shell
  RC_FILES=()
  [[ -f "$HOME/.zshrc" ]]       && RC_FILES+=("$HOME/.zshrc")
  [[ -f "$HOME/.bashrc" ]]      && RC_FILES+=("$HOME/.bashrc")
  [[ -f "$HOME/.bash_profile" ]] && RC_FILES+=("$HOME/.bash_profile")
  # If none exist, create for the login shell
  if [[ ${#RC_FILES[@]} -eq 0 ]]; then
    if [[ "$(basename "$SHELL")" == "zsh" ]]; then
      RC_FILES+=("$HOME/.zshrc")
    else
      RC_FILES+=("$HOME/.bash_profile")
    fi
  fi

  for RC_FILE in "${RC_FILES[@]}"; do
    if ! grep -qF '.local/bin' "$RC_FILE" 2>/dev/null; then
      echo "" >> "$RC_FILE"
      echo '# Added by xmuggle install' >> "$RC_FILE"
      echo "$PATH_LINE" >> "$RC_FILE"
      echo "Added ~/.local/bin to PATH in $RC_FILE"
    fi
  done

  # Also export for the current session
  export PATH="$BIN_DIR:$PATH"
fi

# Claude Code skill
if [[ -d "$HOME/.claude" ]]; then
  mkdir -p "$HOME/.claude/skills/xmuggle"
  cp "$REPO_DIR/skills/claude/SKILL.md" "$HOME/.claude/skills/xmuggle/SKILL.md"
  echo "  ~/.claude/skills/xmuggle/SKILL.md"
fi

# Cursor command
if [[ -d "$HOME/.cursor" ]]; then
  mkdir -p "$HOME/.cursor/commands"
  cp "$REPO_DIR/skills/cursor/command.md" "$HOME/.cursor/commands/xmuggle.md"
  echo "  ~/.cursor/commands/xmuggle.md"
fi

echo ""
echo "Done! Use /xmuggle in Claude Code or Cursor."
echo ""
echo "Quick start:"
echo "  cd ~/dev/my-app && xmuggle init"
echo "  xmuggle send --screenshots"
